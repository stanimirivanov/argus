package replaycli_test

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/predictive/adapters/replaycli"
)

func report(t *testing.T, directory string, index contracts.PredictiveDatasetIndexV1) (contracts.PredictiveBaselinesV1, []byte) {
	t.Helper()
	save(t, directory, "index.json", index)
	var output bytes.Buffer
	if err := replaycli.RunBaselines([]string{"-index", filepath.Join(directory, "index.json"), "-budget-ms", "1000", "-seed", "42"}, &output); err != nil {
		t.Fatal(err)
	}
	var result contracts.PredictiveBaselinesV1
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if err := contracts.ValidatePredictiveBaselinesV1(result); err != nil {
		t.Fatal(err)
	}

	return result, output.Bytes()
}

func baselineArchive(t *testing.T, directory string) contracts.PredictiveDatasetIndexV1 {
	t.Helper()
	train := observation(t, 42, "tests-a", "full-a", "2026-10-20T09:00:00Z")
	train.Observation.Tests = append(train.Observation.Tests, contracts.PredictiveTestV1{
		SuiteKey: "api", TestKey: "billing", Decision: "RUN_REQUIRED", Reasons: []string{"DIRECT_CAPABILITY_IMPACT"},
		Capabilities: []string{}, MappingKeys: []string{}, Outcome: "passed", DurationMS: 500})
	var err error
	train, err = contracts.NewPredictiveObservationEnvelopeV1(train.Observation)
	if err != nil {
		t.Fatal(err)
	}
	holdout := observation(t, 43, "tests-a", "full-b", "2026-10-20T11:00:00Z")
	holdout.Observation.Tests = append(holdout.Observation.Tests, contracts.PredictiveTestV1{
		SuiteKey: "api", TestKey: "billing", Decision: "SKIP_FOR_NOW", Reasons: []string{"NOT_AFFECTED_IN_EARLY_STAGE"},
		Capabilities: []string{}, MappingKeys: []string{}, Outcome: "passed", DurationMS: 500})
	holdout, err = contracts.NewPredictiveObservationEnvelopeV1(holdout.Observation)
	if err != nil {
		t.Fatal(err)
	}
	save(t, directory, "train.json", train)
	save(t, directory, "holdout.json", holdout)

	return contracts.PredictiveDatasetIndexV1{APIVersion: "argus.dev/predictive-dataset-index/v1",
		HoldoutStart: "2026-10-20T10:00:00Z", HoldoutEnd: "2026-10-20T12:00:00Z",
		Episodes: []contracts.PredictiveIndexEpisode{{ID: "holdout", Observations: []string{"holdout.json"}},
			{ID: "train", Observations: []string{"train.json"}}}}
}

func TestBaselinesArePortableAndOrderIndependent(t *testing.T) {
	directory := t.TempDir()
	index := baselineArchive(t, directory)
	first, firstBytes := report(t, directory, index)
	index.Episodes[0], index.Episodes[1] = index.Episodes[1], index.Episodes[0]
	second, secondBytes := report(t, directory, index)
	if !bytes.Equal(firstBytes, secondBytes) || !reflect.DeepEqual(first, second) || len(first.Episodes) != 1 ||
		len(first.Episodes[0].Comparisons) != 5 || first.Episodes[0].Comparisons[0].FullTestCount != 2 ||
		first.Episodes[0].Comparisons[0].FullFailureCount != 1 || first.Episodes[0].Comparisons[0].MissedFailureCount != 0 {
		t.Fatalf("baseline report is not deterministic or complete: %+v", first)
	}
}

func TestBaselinesCannotUseHeldOutLabelsForSelection(t *testing.T) {
	directory := t.TempDir()
	index := baselineArchive(t, directory)
	first, _ := report(t, directory, index)
	holdout := observation(t, 43, "tests-a", "full-b", "2026-10-20T11:00:00Z")
	holdout.Observation.Tests = append(holdout.Observation.Tests, contracts.PredictiveTestV1{
		SuiteKey: "api", TestKey: "billing", Decision: "SKIP_FOR_NOW", Reasons: []string{"NOT_AFFECTED_IN_EARLY_STAGE"},
		Capabilities: []string{}, MappingKeys: []string{}, Outcome: "failed", DurationMS: 9000})
	holdout.Observation.Tests[0].Outcome = "passed"
	holdout.Observation.Tests[0].DurationMS = 9000
	var err error
	holdout, err = contracts.NewPredictiveObservationEnvelopeV1(holdout.Observation)
	if err != nil {
		t.Fatal(err)
	}
	save(t, directory, "holdout.json", holdout)
	second, _ := report(t, directory, index)
	for position, comparison := range first.Episodes[0].Comparisons {
		if !reflect.DeepEqual(comparison.SelectedTestIDs, second.Episodes[0].Comparisons[position].SelectedTestIDs) {
			t.Fatalf("holdout labels changed %s selection", comparison.Name)
		}
	}
	if reflect.DeepEqual(first, second) {
		t.Fatal("held-out outcomes did not change measured metrics")
	}
}
