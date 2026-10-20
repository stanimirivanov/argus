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

func rankingArchive(t *testing.T, directory string) contracts.PredictiveDatasetIndexV1 {
	t.Helper()
	index := baselineArchive(t, directory)
	second := observation(t, 44, "tests-a", "full-c", "2026-10-20T09:30:00Z")
	second.Observation.Tests = append(second.Observation.Tests, contracts.PredictiveTestV1{
		SuiteKey: "api", TestKey: "billing", Decision: "SKIP_FOR_NOW", Reasons: []string{"NOT_AFFECTED_IN_EARLY_STAGE"},
		Capabilities: []string{}, MappingKeys: []string{}, Outcome: "failed", DurationMS: 700})
	var err error
	second, err = contracts.NewPredictiveObservationEnvelopeV1(second.Observation)
	if err != nil {
		t.Fatal(err)
	}
	save(t, directory, "train-second.json", second)
	index.Episodes = append(index.Episodes, contracts.PredictiveIndexEpisode{ID: "train-second", Observations: []string{"train-second.json"}})

	return index
}

func rankingReport(t *testing.T, directory string, index contracts.PredictiveDatasetIndexV1) (contracts.PredictiveRankingV1, []byte) {
	t.Helper()
	save(t, directory, "index.json", index)
	var output bytes.Buffer
	if err := replaycli.RunRanking([]string{"-index", filepath.Join(directory, "index.json"), "-budget-ms", "1000", "-seed", "42"}, &output); err != nil {
		t.Fatal(err)
	}
	var result contracts.PredictiveRankingV1
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if err := contracts.ValidatePredictiveRankingV1(result); err != nil {
		t.Fatal(err)
	}

	return result, output.Bytes()
}

func TestRankingReportIsPortableAndOrderIndependent(t *testing.T) {
	directory := t.TempDir()
	index := rankingArchive(t, directory)
	first, firstBytes := rankingReport(t, directory, index)
	index.Episodes[0], index.Episodes[2] = index.Episodes[2], index.Episodes[0]
	second, secondBytes := rankingReport(t, directory, index)
	if !bytes.Equal(firstBytes, secondBytes) || !reflect.DeepEqual(first, second) ||
		len(first.Episodes) != 1 || len(first.Episodes[0].Ranked) != 2 ||
		len(first.Baselines.Episodes[0].Comparisons) != 5 || first.Model.TrainingExamples != 4 {
		t.Fatalf("ranking report differs with index order: %+v", first)
	}
}

func TestRankingReportDoesNotUseHeldOutLabelsForSelection(t *testing.T) {
	directory := t.TempDir()
	index := rankingArchive(t, directory)
	first, _ := rankingReport(t, directory, index)
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
	second, _ := rankingReport(t, directory, index)
	if !reflect.DeepEqual(first.Model, second.Model) || !reflect.DeepEqual(first.Episodes[0].Ranked, second.Episodes[0].Ranked) ||
		reflect.DeepEqual(first.Episodes[0].Comparison, second.Episodes[0].Comparison) {
		t.Fatal("held-out labels leaked into scores or did not affect metrics")
	}
}

func TestRankingRejectsInsufficientTrainingWithoutOutput(t *testing.T) {
	directory := t.TempDir()
	index := baselineArchive(t, directory)
	save(t, directory, "index.json", index)
	var output bytes.Buffer
	err := replaycli.RunRanking([]string{"-index", filepath.Join(directory, "index.json"), "-budget-ms", "1000", "-seed", "42"}, &output)
	if err == nil || output.Len() != 0 {
		t.Fatalf("insufficient training emitted output: err=%v bytes=%d", err, output.Len())
	}
}
