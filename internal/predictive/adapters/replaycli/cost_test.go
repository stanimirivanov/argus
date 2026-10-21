package replaycli_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/predictive/adapters/replaycli"
)

func TestCostReportBindsReviewAndIgnoresHoldoutLabels(t *testing.T) {
	directory := t.TempDir()
	index := rankingArchive(t, directory)
	ranking, _ := rankingReport(t, directory, index)
	constraints := contracts.PredictiveStageConstraintsV1{APIVersion: "argus.dev/predictive-stage-constraints/v1",
		Episodes: []contracts.PredictiveStageReview{{ID: "holdout", ReviewID: "review-1", ReviewedBy: "maintainer",
			ReviewedAt: "2026-10-20T10:30:00Z", FeatureCutoffAt: "2026-10-20T11:00:00Z", ManifestSHA256: strings.Repeat("c", 64),
			Environments: []contracts.PredictiveStageEnvironment{{
				Key: "ci", CapacityUnits: 2, SetupDurationMS: 100}}, Tests: []contracts.PredictiveStageTestConstraint{}}}}
	for _, test := range ranking.Episodes[0].Ranked {
		constraints.Episodes[0].Tests = append(constraints.Episodes[0].Tests, contracts.PredictiveStageTestConstraint{
			ID: test.ID, Environment: "ci", ResourceUnits: 1, ColdStartDurationMS: 5000, Dependencies: []string{}})
	}
	save(t, directory, "constraints.json", constraints)
	run := func() ([]byte, contracts.PredictiveStageCostV1) {
		t.Helper()
		save(t, directory, "index.json", index)
		var output bytes.Buffer
		if err := replaycli.RunCost([]string{"-index", filepath.Join(directory, "index.json"),
			"-constraints", filepath.Join(directory, "constraints.json")}, &output); err != nil {
			t.Fatal(err)
		}
		var report contracts.PredictiveStageCostV1
		if err := json.Unmarshal(output.Bytes(), &report); err != nil {
			t.Fatal(err)
		}
		if err := contracts.ValidatePredictiveStageCostV1(report); err != nil {
			t.Fatal(err)
		}

		return output.Bytes(), report
	}
	first, report := run()
	data, err := os.ReadFile(filepath.Join(directory, "constraints.json"))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	if report.ConstraintsSHA256 != hex.EncodeToString(digest[:]) || len(report.Episodes) != 1 ||
		report.Episodes[0].SerialDurationMS != 1800 || report.Episodes[0].LowerBoundMS != 1100 {
		t.Fatalf("incomplete stage cost report: %+v", report)
	}
	index.Episodes[0], index.Episodes[2] = index.Episodes[2], index.Episodes[0]
	second, _ := run()
	if !bytes.Equal(first, second) {
		t.Fatal("archive declaration order changed stage cost")
	}
	holdout := observation(t, 43, "tests-a", "full-b", "2026-10-20T11:00:00Z")
	holdout.Observation.Tests[0].Outcome, holdout.Observation.Tests[0].DurationMS = "passed", 90000
	holdout.Observation.Tests = append(holdout.Observation.Tests, contracts.PredictiveTestV1{
		SuiteKey: "api", TestKey: "billing", Decision: "SKIP_FOR_NOW", Reasons: []string{"NOT_AFFECTED_IN_EARLY_STAGE"},
		Capabilities: []string{}, MappingKeys: []string{}, Outcome: "failed", DurationMS: 90000})
	holdout, err = contracts.NewPredictiveObservationEnvelopeV1(holdout.Observation)
	if err != nil {
		t.Fatal(err)
	}
	save(t, directory, "holdout.json", holdout)
	third, _ := run()
	if !bytes.Equal(first, third) {
		t.Fatal("held-out labels affected duration estimates")
	}
	constraints.Episodes[0].Tests[0].Dependencies = []string{constraints.Episodes[0].Tests[1].ID}
	constraints.Episodes[0].Tests[1].Dependencies = []string{constraints.Episodes[0].Tests[0].ID}
	save(t, directory, "constraints.json", constraints)
	var rejected bytes.Buffer
	if err := replaycli.RunCost([]string{"-index", filepath.Join(directory, "index.json"),
		"-constraints", filepath.Join(directory, "constraints.json")}, &rejected); err == nil || rejected.Len() != 0 {
		t.Fatalf("cyclic constraints accepted or partial output written: %v", err)
	}
}

func TestCostRejectsMalformedReviewWithoutOutput(t *testing.T) {
	directory := t.TempDir()
	save(t, directory, "constraints.json", map[string]any{"apiVersion": "argus.dev/predictive-stage-constraints/v1", "episodes": []any{}})
	var output bytes.Buffer
	if err := replaycli.RunCost([]string{"-index", "missing", "-constraints", filepath.Join(directory, "constraints.json")}, &output); err == nil || output.Len() != 0 {
		t.Fatal("malformed review accepted or partial output emitted")
	}
}
