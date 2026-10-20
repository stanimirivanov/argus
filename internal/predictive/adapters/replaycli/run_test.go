package replaycli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/predictive/adapters/replaycli"
)

func observation(t *testing.T, pull int, repository, attempt, cutoff string) contracts.PredictiveObservationEnvelopeV1 {
	t.Helper()
	change := contracts.ManifestChangeReference{
		SourceRepository: contracts.RepositoryReference{Provider: "github", Host: "github.com", ProviderRepositoryID: "source-1", Owner: "example", Name: "source"},
		PullRequest:      contracts.PullRequestReference{Number: pull},
		BaseRevision:     contracts.RevisionReference{Algorithm: "git-sha1", Digest: strings.Repeat("a", 40)},
		HeadRevision:     contracts.RevisionReference{Algorithm: "git-sha1", Digest: strings.Repeat("b", 40)},
		ObservedAt:       cutoff,
		Trigger:          contracts.ChangeTrigger{Provider: "github", DeliveryID: "delivery-" + strconv.Itoa(pull), Event: "pull_request", Action: "synchronize"},
	}
	doc := contracts.PredictiveObservationV1{APIVersion: "argus.dev/predictive-observation/v1", Change: change,
		Files:           []contracts.PredictiveFileV1{{Path: "api/openapi.yaml", Kind: "modified", Additions: 1, Deletions: 0, PatchStatus: "complete"}},
		Manifest:        contracts.ManifestDigestReference{APIVersion: "argus.dev/execution-manifest/v1", SHA256: strings.Repeat("c", 64)},
		FeatureCutoffAt: cutoff, Family: "functional-api",
		TestRepository: contracts.RepositoryReference{Provider: "github", Host: "github.com", ProviderRepositoryID: repository, Owner: "example", Name: repository},
		TestRevision:   contracts.RevisionReference{Algorithm: "git-sha1", Digest: strings.Repeat("d", 40)},
		Adapter:        "playwright", ControlAttemptID: attempt,
		ControlStartedAt:   strings.Replace(cutoff, ":00Z", ":01Z", 1),
		ControlCompletedAt: strings.Replace(cutoff, ":00Z", ":02Z", 1),
		Tests: []contracts.PredictiveTestV1{{SuiteKey: "api", TestKey: "orders", Decision: "RUN_REQUIRED",
			Reasons: []string{"DIRECT_CAPABILITY_IMPACT"}, Capabilities: []string{"orders"}, MappingKeys: []string{}, Outcome: "failed", DurationMS: 1000}}}
	envelope, err := contracts.NewPredictiveObservationEnvelopeV1(doc)
	if err != nil {
		t.Fatal(err)
	}

	return envelope
}

func save(t *testing.T, directory, name string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, name), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func run(t *testing.T, directory string, index contracts.PredictiveDatasetIndexV1) (contracts.PredictiveReplayV1, []byte, error) {
	t.Helper()
	save(t, directory, "index.json", index)
	var output bytes.Buffer
	err := replaycli.Run([]string{"-index", filepath.Join(directory, "index.json")}, &output)
	if err != nil {
		return contracts.PredictiveReplayV1{}, output.Bytes(), err
	}
	var replay contracts.PredictiveReplayV1
	if err := json.Unmarshal(output.Bytes(), &replay); err != nil {
		t.Fatal(err)
	}
	if err := contracts.ValidatePredictiveReplayV1(replay); err != nil {
		t.Fatal(err)
	}

	return replay, output.Bytes(), nil
}

func TestReplayLoadsExplicitGroupsAndIsOrderIndependent(t *testing.T) {
	directory := t.TempDir()
	save(t, directory, "train-a.json", observation(t, 42, "tests-a", "full-a", "2026-10-20T09:00:00Z"))
	save(t, directory, "train-b.json", observation(t, 42, "tests-b", "full-b", "2026-10-20T09:00:00Z"))
	save(t, directory, "holdout.json", observation(t, 43, "tests-a", "full-c", "2026-10-20T11:00:00Z"))
	index := contracts.PredictiveDatasetIndexV1{APIVersion: "argus.dev/predictive-dataset-index/v1",
		HoldoutStart: "2026-10-20T10:00:00Z", HoldoutEnd: "2026-10-20T12:00:00Z",
		Episodes: []contracts.PredictiveIndexEpisode{{ID: "holdout", Observations: []string{"holdout.json"}},
			{ID: "train", Observations: []string{"train-b.json", "train-a.json"}}}}
	first, firstBytes, err := run(t, directory, index)
	if err != nil {
		t.Fatal(err)
	}
	index.Episodes[0], index.Episodes[1] = index.Episodes[1], index.Episodes[0]
	index.Episodes[0].Observations[0], index.Episodes[0].Observations[1] = index.Episodes[0].Observations[1], index.Episodes[0].Observations[0]
	second, secondBytes, err := run(t, directory, index)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstBytes, secondBytes) || !reflect.DeepEqual(first, second) ||
		first.Episodes[0].Split != "train" || len(first.Episodes[0].ObservationSHA256) != 2 ||
		first.Episodes[1].Split != "holdout" || !reflect.DeepEqual(first.Episodes[1].History, []string{"train"}) {
		t.Fatalf("invalid replay: %+v", first)
	}
}

func TestReplayRejectsTamperingMissingAndConflictingGroups(t *testing.T) {
	directory := t.TempDir()
	envelope := observation(t, 42, "tests-a", "full-a", "2026-10-20T09:00:00Z")
	save(t, directory, "one.json", envelope)
	index := contracts.PredictiveDatasetIndexV1{APIVersion: "argus.dev/predictive-dataset-index/v1",
		HoldoutStart: "2026-10-20T10:00:00Z", HoldoutEnd: "2026-10-20T12:00:00Z",
		Episodes: []contracts.PredictiveIndexEpisode{{ID: "train", Observations: []string{"one.json", "missing.json"}}}}
	if _, output, err := run(t, directory, index); err == nil || len(output) != 0 {
		t.Fatal("missing declared group accepted or partial output written")
	}
	index.Episodes[0].Observations[1] = "one.json"
	if _, output, err := run(t, directory, index); err == nil || len(output) != 0 {
		t.Fatal("duplicate group accepted or partial output written")
	}
	envelope.SHA256 = strings.Repeat("0", 64)
	save(t, directory, "tampered.json", envelope)
	index.Episodes[0].Observations = []string{"tampered.json"}
	if _, output, err := run(t, directory, index); err == nil || len(output) != 0 {
		t.Fatal("tampered digest accepted or partial output written")
	}
	index.Episodes[0].Observations = []string{"../one.json"}
	if _, output, err := run(t, directory, index); err == nil || len(output) != 0 {
		t.Fatal("escaped path accepted or partial output written")
	}
}

func TestReplayRejectsRenamedSourcePullAcrossSplit(t *testing.T) {
	directory := t.TempDir()
	save(t, directory, "train.json", observation(t, 42, "tests-a", "full-a", "2026-10-20T09:00:00Z"))
	holdout := observation(t, 42, "tests-a", "full-b", "2026-10-20T11:00:00Z")
	holdout.Observation.Change.SourceRepository.Owner = "renamed-owner"
	var err error
	holdout, err = contracts.NewPredictiveObservationEnvelopeV1(holdout.Observation)
	if err != nil {
		t.Fatal(err)
	}
	save(t, directory, "holdout.json", holdout)
	index := contracts.PredictiveDatasetIndexV1{APIVersion: "argus.dev/predictive-dataset-index/v1",
		HoldoutStart: "2026-10-20T10:00:00Z", HoldoutEnd: "2026-10-20T12:00:00Z",
		Episodes: []contracts.PredictiveIndexEpisode{{ID: "train", Observations: []string{"train.json"}},
			{ID: "holdout", Observations: []string{"holdout.json"}}}}
	if _, output, err := run(t, directory, index); err == nil || len(output) != 0 {
		t.Fatal("renamed source pull crossed temporal split")
	}
}
