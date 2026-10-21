package replaycli_test

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/predictive/adapters/replaycli"
)

func TestAbstentionReportIsPortableCompleteAndOrderIndependent(t *testing.T) {
	directory := t.TempDir()
	index := calibrationArchive(t, directory)
	holdout := observation(t, 43, "tests-a", "full-b", "2026-10-20T11:00:00Z")
	holdout.Observation.Tests = append(holdout.Observation.Tests, contracts.PredictiveTestV1{
		SuiteKey: "api", TestKey: "new", Decision: "SKIP_FOR_NOW",
		Reasons: []string{"NOT_AFFECTED_IN_EARLY_STAGE"}, Capabilities: []string{}, MappingKeys: []string{},
		Outcome: "passed", DurationMS: 200})
	var err error
	holdout, err = contracts.NewPredictiveObservationEnvelopeV1(holdout.Observation)
	if err != nil {
		t.Fatal(err)
	}
	save(t, directory, "holdout.json", holdout)
	run := func() ([]byte, contracts.PredictiveAbstentionV1) {
		t.Helper()
		save(t, directory, "index.json", index)
		var output bytes.Buffer
		if err := replaycli.RunAbstention([]string{"-index", filepath.Join(directory, "index.json")}, &output); err != nil {
			t.Fatal(err)
		}
		var report contracts.PredictiveAbstentionV1
		if err := json.Unmarshal(output.Bytes(), &report); err != nil {
			t.Fatal(err)
		}
		if err := contracts.ValidatePredictiveAbstentionV1(report); err != nil {
			t.Fatal(err)
		}

		return output.Bytes(), report
	}
	first, report := run()
	if len(report.Episodes) != 1 || report.Episodes[0].Mode != "full-control" ||
		report.Episodes[0].EvidenceState != "calibrated" ||
		len(report.Episodes[0].Tests) != 2 || len(report.Episodes[0].RequiredTestIDs) != 2 {
		t.Fatalf("fallback is incomplete: %+v", report)
	}
	ids := []string{report.Episodes[0].Tests[0].ID, report.Episodes[0].Tests[1].ID}
	slices.Sort(ids)
	if !slices.Equal(ids, report.Episodes[0].RequiredTestIDs) {
		t.Fatalf("fallback omitted a control: %+v", report.Episodes[0])
	}
	coldStart := false
	for _, test := range report.Episodes[0].Tests {
		if test.PriorControls == 0 && slices.Contains(test.Reasons, "cold-start") {
			coldStart = true
		}
	}
	if !coldStart {
		t.Fatal("new held-out test did not trigger cold-start fallback")
	}
	missing := report
	missing.Episodes = append([]contracts.PredictiveAbstentionEpisode(nil), report.Episodes...)
	missing.Episodes[0].RequiredTestIDs = report.Episodes[0].RequiredTestIDs[:1]
	if err := contracts.ValidatePredictiveAbstentionV1(missing); err == nil {
		t.Fatal("portable report accepted incomplete full-control test set")
	}
	index.Episodes[0], index.Episodes[3] = index.Episodes[3], index.Episodes[0]
	second, _ := run()
	if !bytes.Equal(first, second) {
		t.Fatal("archive declaration order changed abstention report")
	}
	holdout.Observation.Tests[0].Outcome = "passed"
	holdout.Observation.Tests[0].DurationMS = 9000
	holdout, err = contracts.NewPredictiveObservationEnvelopeV1(holdout.Observation)
	if err != nil {
		t.Fatal(err)
	}
	save(t, directory, "holdout.json", holdout)
	third, _ := run()
	if !bytes.Equal(first, third) {
		t.Fatal("held-out labels changed abstention report")
	}
}

func TestAbstentionFallsBackWithoutCalibrationAndRejectsMalformedArchive(t *testing.T) {
	directory := t.TempDir()
	index := rankingArchive(t, directory)
	save(t, directory, "index.json", index)
	var output bytes.Buffer
	if err := replaycli.RunAbstention([]string{"-index", filepath.Join(directory, "index.json")}, &output); err != nil {
		t.Fatal(err)
	}
	var report contracts.PredictiveAbstentionV1
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if err := contracts.ValidatePredictiveAbstentionV1(report); err != nil ||
		len(report.Episodes) != 1 || report.Episodes[0].EvidenceState != "unavailable" ||
		len(report.Episodes[0].RequiredTestIDs) != len(report.Episodes[0].CandidateTestIDs) {
		t.Fatalf("missing calibration did not produce portable full-control fallback: %v %+v", err, report)
	}
	index.Episodes[0].Observations = []string{"missing.json"}
	save(t, directory, "index.json", index)
	output.Reset()
	err := replaycli.RunAbstention([]string{"-index", filepath.Join(directory, "index.json")}, &output)
	if err == nil || output.Len() != 0 {
		t.Fatalf("malformed archive emitted output: err=%v bytes=%d", err, output.Len())
	}
}
