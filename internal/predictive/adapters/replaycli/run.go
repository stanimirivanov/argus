// Package replaycli loads explicit offline evidence and emits a temporal replay.
package replaycli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/predictive"
	"github.com/stanimirivanov/argus/internal/selection"
)

const maxIndexBytes = 1 << 20
const maxObservationBytes = 16 << 20
const maxObservations = 10000

// Run reads a complete declared archive and writes one replay or no output.
func Run(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("predictive-replay", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	indexPath := flags.String("index", "", "dataset index JSON")
	if err := commandline.Parse(flags, args); err != nil {
		return err
	}
	if *indexPath == "" || flags.NArg() != 0 {
		return commandline.UsageText("usage: predictive-replay -index <path>")
	}
	replay, _, err := loadDataset(*indexPath, false)
	if err != nil {
		return err
	}
	output := exportReplay(replay)
	if err := contracts.ValidatePredictiveReplayV1(output); err != nil {
		return err
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return err
	}
	_, err = stdout.Write(append(encoded, '\n'))

	return err
}

func loadDataset(indexPath string, includeTests bool) (predictive.Replay, map[string][]predictive.EvaluationTest, error) {
	data, err := readBounded(indexPath, maxIndexBytes)
	if err != nil {
		return predictive.Replay{}, nil, fmt.Errorf("read dataset index: %w", err)
	}
	index, err := contracts.DecodePredictiveDatasetIndexV1(data)
	if err != nil {
		return predictive.Replay{}, nil, err
	}
	start, err := time.Parse(time.RFC3339Nano, index.HoldoutStart)
	if err != nil {
		return predictive.Replay{}, nil, err
	}
	end, err := time.Parse(time.RFC3339Nano, index.HoldoutEnd)
	if err != nil {
		return predictive.Replay{}, nil, err
	}
	root, err := filepath.EvalSymlinks(filepath.Dir(indexPath))
	if err != nil {
		return predictive.Replay{}, nil, err
	}
	episodes := make([]predictive.Episode, 0, len(index.Episodes))
	archive := make(map[string][]predictive.EvaluationTest, len(index.Episodes))
	count := 0
	testCount := 0
	for _, declaration := range index.Episodes {
		count += len(declaration.Observations)
		if count > maxObservations {
			return predictive.Replay{}, nil, fmt.Errorf("dataset exceeds %d observations", maxObservations)
		}
		episode, tests, err := loadEpisode(root, declaration, includeTests)
		if err != nil {
			return predictive.Replay{}, nil, fmt.Errorf("episode %s: %w", declaration.ID, err)
		}
		testCount += len(tests)
		if includeTests && (testCount > 100000 || len(tests) > 10000) {
			return predictive.Replay{}, nil, fmt.Errorf("dataset exceeds baseline test limit")
		}
		episodes = append(episodes, episode)
		archive[declaration.ID] = tests
	}
	replay, err := predictive.BuildReplay(episodes, start, end)
	if err != nil {
		return predictive.Replay{}, nil, err
	}

	return replay, archive, nil
}

func loadEpisode(root string, declaration contracts.PredictiveIndexEpisode, includeTests bool) (predictive.Episode, []predictive.EvaluationTest, error) {
	episode := predictive.Episode{ID: declaration.ID, Groups: make([]predictive.GroupReference, 0, len(declaration.Observations))}
	var tests []predictive.EvaluationTest
	for _, name := range declaration.Observations {
		envelope, err := readObservation(root, name)
		if err != nil {
			return predictive.Episode{}, nil, err
		}
		current, group, err := importObservation(envelope)
		if err != nil {
			return predictive.Episode{}, nil, err
		}
		if len(episode.Groups) == 0 {
			episode.ChangeKey, episode.SourcePullKey, episode.ManifestKey = current.ChangeKey, current.SourcePullKey, current.ManifestKey
			episode.FilesKey, episode.Family, episode.FeatureCutoffAt = current.FilesKey, current.Family, current.FeatureCutoffAt
		} else if episode.ChangeKey != current.ChangeKey || episode.ManifestKey != current.ManifestKey ||
			episode.FilesKey != current.FilesKey || episode.Family != current.Family || !episode.FeatureCutoffAt.Equal(current.FeatureCutoffAt) {
			return predictive.Episode{}, nil, fmt.Errorf("%w: inconsistent decision groups", predictive.ErrInvalid)
		}
		episode.Groups = append(episode.Groups, group)
		if includeTests {
			tests, err = appendEvaluationTests(tests, envelope.Observation, group.ControlCompletedAt)
			if err != nil {
				return predictive.Episode{}, nil, err
			}
		}
	}

	return episode, tests, nil
}

func readObservation(root, name string) (contracts.PredictiveObservationEnvelopeV1, error) {
	path, err := containedPath(root, name)
	if err != nil {
		return contracts.PredictiveObservationEnvelopeV1{}, err
	}
	data, err := readBounded(path, maxObservationBytes)
	if err != nil {
		return contracts.PredictiveObservationEnvelopeV1{}, err
	}

	return contracts.DecodePredictiveObservationEnvelopeV1(data)
}

func appendEvaluationTests(tests []predictive.EvaluationTest, observation contracts.PredictiveObservationV1, completedAt time.Time) ([]predictive.EvaluationTest, error) {
	environment := ""
	if observation.Environment != nil {
		environment = *observation.Environment
	}
	repositoryKey, err := digest(struct {
		Provider             string
		Host                 string
		ProviderRepositoryID string
	}{observation.TestRepository.Provider, observation.TestRepository.Host, observation.TestRepository.ProviderRepositoryID})
	if err != nil {
		return nil, err
	}
	for _, test := range observation.Tests {
		if len(tests) >= 10000 {
			return nil, fmt.Errorf("baseline episode exceeds 10000 tests")
		}
		identity, err := testIdentity(observation, test)
		if err != nil {
			return nil, err
		}
		tests = append(tests, predictive.EvaluationTest{ID: identity, RepositoryKey: repositoryKey, Family: observation.Family, Environment: environment,
			ManifestSHA256: observation.Manifest.SHA256,
			Capabilities:   slices.Clone(test.Capabilities),
			Decision:       selection.Outcome(test.Decision),
			Outcome:        execution.TestOutcome(test.Outcome), DurationMS: test.DurationMS, ControlCompletedAt: completedAt})
	}

	return tests, nil
}

func testIdentity(observation contracts.PredictiveObservationV1, test contracts.PredictiveTestV1) (string, error) {
	return digest(struct {
		Provider             string
		Host                 string
		ProviderRepositoryID string
		Family               string
		Adapter              string
		Project              *string
		Environment          *string
		SuiteKey             string
		TestKey              string
	}{observation.TestRepository.Provider, observation.TestRepository.Host, observation.TestRepository.ProviderRepositoryID,
		observation.Family, observation.Adapter, observation.Project, observation.Environment, test.SuiteKey, test.TestKey})
}

func importObservation(envelope contracts.PredictiveObservationEnvelopeV1) (predictive.Episode, predictive.GroupReference, error) {
	doc := envelope.Observation
	cutoff, err := time.Parse(time.RFC3339Nano, doc.FeatureCutoffAt)
	if err != nil {
		return predictive.Episode{}, predictive.GroupReference{}, err
	}
	observed, err := time.Parse(time.RFC3339Nano, doc.Change.ObservedAt)
	if err != nil || cutoff.Before(observed) {
		return predictive.Episode{}, predictive.GroupReference{}, predictive.ErrInvalid
	}
	started, err := time.Parse(time.RFC3339Nano, doc.ControlStartedAt)
	if err != nil {
		return predictive.Episode{}, predictive.GroupReference{}, err
	}
	completed, err := time.Parse(time.RFC3339Nano, doc.ControlCompletedAt)
	if err != nil {
		return predictive.Episode{}, predictive.GroupReference{}, err
	}
	changeKey, err := digest(doc.Change)
	if err != nil {
		return predictive.Episode{}, predictive.GroupReference{}, err
	}
	sourcePullKey, err := digest(struct {
		Provider             string
		Host                 string
		ProviderRepositoryID string
		Number               int
	}{doc.Change.SourceRepository.Provider, doc.Change.SourceRepository.Host,
		doc.Change.SourceRepository.ProviderRepositoryID, doc.Change.PullRequest.Number})
	if err != nil {
		return predictive.Episode{}, predictive.GroupReference{}, err
	}
	filesKey, err := digest(struct {
		Files     []contracts.PredictiveFileV1
		Truncated bool
	}{doc.Files, doc.FilesTruncated})
	if err != nil {
		return predictive.Episode{}, predictive.GroupReference{}, err
	}
	coordinates, err := digest(struct {
		Provider             string
		Host                 string
		ProviderRepositoryID string
		Adapter              string
		Project              *string
		Environment          *string
	}{doc.TestRepository.Provider, doc.TestRepository.Host, doc.TestRepository.ProviderRepositoryID,
		doc.Adapter, doc.Project, doc.Environment})
	if err != nil {
		return predictive.Episode{}, predictive.GroupReference{}, err
	}
	episode := predictive.Episode{ChangeKey: changeKey, SourcePullKey: sourcePullKey,
		ManifestKey: doc.Manifest.APIVersion + ":" + doc.Manifest.SHA256, FilesKey: filesKey,
		Family: doc.Family, FeatureCutoffAt: cutoff}
	group := predictive.GroupReference{ObservationSHA256: envelope.SHA256, Coordinates: coordinates,
		ControlAttemptID: doc.ControlAttemptID, ControlStartedAt: started, ControlCompletedAt: completed}

	return episode, group, nil
}

func digest(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:]), nil
}

func containedPath(root, name string) (string, error) {
	if !filepath.IsLocal(name) {
		return "", fmt.Errorf("%w: non-local observation path", predictive.ErrInvalid)
	}
	path, err := filepath.EvalSymlinks(filepath.Join(root, name))
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || !filepath.IsLocal(relative) {
		return "", fmt.Errorf("%w: observation escaped index directory", predictive.ErrInvalid)
	}

	return path, nil
}

func readBounded(path string, limit int) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }() //nolint:errcheck // Read-only archive input.
	data, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > limit {
		return nil, fmt.Errorf("archive input exceeds %d bytes", limit)
	}

	return data, nil
}

func exportReplay(replay predictive.Replay) contracts.PredictiveReplayV1 {
	output := contracts.PredictiveReplayV1{APIVersion: replay.APIVersion,
		HoldoutStart: replay.HoldoutStart.Format(time.RFC3339Nano), HoldoutEnd: replay.HoldoutEnd.Format(time.RFC3339Nano),
		TrainCount: replay.TrainCount, HoldoutCount: replay.HoldoutCount, ExcludedCount: replay.ExcludedCount,
		Episodes: make([]contracts.PredictiveReplayEpisode, 0, len(replay.Episodes))}
	for _, episode := range replay.Episodes {
		var reason *string
		if episode.ExclusionReason != "" {
			reason = &episode.ExclusionReason
		}
		output.Episodes = append(output.Episodes, contracts.PredictiveReplayEpisode{ID: episode.ID,
			FeatureCutoffAt: episode.FeatureCutoffAt.Format(time.RFC3339Nano), ObservationSHA256: episode.ObservationSHA256,
			Split: episode.Split, ExclusionReason: reason, History: episode.History})
	}

	return output
}
