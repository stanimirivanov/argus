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
	"time"

	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/predictive"
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
	data, err := readBounded(*indexPath, maxIndexBytes)
	if err != nil {
		return fmt.Errorf("read dataset index: %w", err)
	}
	index, err := contracts.DecodePredictiveDatasetIndexV1(data)
	if err != nil {
		return err
	}
	start, err := time.Parse(time.RFC3339Nano, index.HoldoutStart)
	if err != nil {
		return err
	}
	end, err := time.Parse(time.RFC3339Nano, index.HoldoutEnd)
	if err != nil {
		return err
	}
	root, err := filepath.EvalSymlinks(filepath.Dir(*indexPath))
	if err != nil {
		return err
	}
	episodes := make([]predictive.Episode, 0, len(index.Episodes))
	count := 0
	for _, declaration := range index.Episodes {
		count += len(declaration.Observations)
		if count > maxObservations {
			return fmt.Errorf("dataset exceeds %d observations", maxObservations)
		}
		episode, err := loadEpisode(root, declaration)
		if err != nil {
			return fmt.Errorf("episode %s: %w", declaration.ID, err)
		}
		episodes = append(episodes, episode)
	}
	replay, err := predictive.BuildReplay(episodes, start, end)
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

func loadEpisode(root string, declaration contracts.PredictiveIndexEpisode) (predictive.Episode, error) {
	episode := predictive.Episode{ID: declaration.ID, Groups: make([]predictive.GroupReference, 0, len(declaration.Observations))}
	for _, name := range declaration.Observations {
		path, err := containedPath(root, name)
		if err != nil {
			return predictive.Episode{}, err
		}
		data, err := readBounded(path, maxObservationBytes)
		if err != nil {
			return predictive.Episode{}, err
		}
		envelope, err := contracts.DecodePredictiveObservationEnvelopeV1(data)
		if err != nil {
			return predictive.Episode{}, err
		}
		current, group, err := importObservation(envelope)
		if err != nil {
			return predictive.Episode{}, err
		}
		if len(episode.Groups) == 0 {
			episode.ChangeKey, episode.SourcePullKey, episode.ManifestKey = current.ChangeKey, current.SourcePullKey, current.ManifestKey
			episode.FilesKey, episode.Family, episode.FeatureCutoffAt = current.FilesKey, current.Family, current.FeatureCutoffAt
		} else if episode.ChangeKey != current.ChangeKey || episode.ManifestKey != current.ManifestKey ||
			episode.FilesKey != current.FilesKey || episode.Family != current.Family || !episode.FeatureCutoffAt.Equal(current.FeatureCutoffAt) {
			return predictive.Episode{}, fmt.Errorf("%w: inconsistent decision groups", predictive.ErrInvalid)
		}
		episode.Groups = append(episode.Groups, group)
	}

	return episode, nil
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
