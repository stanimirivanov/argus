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

func portfolioReport(t *testing.T) contracts.PredictivePortfolioV1 {
	t.Helper()
	directory := t.TempDir()
	index := rankingArchive(t, directory)
	ranking, _ := rankingReport(t, directory, index)
	constraints := contracts.PredictiveStageConstraintsV1{APIVersion: "argus.dev/predictive-stage-constraints/v1",
		Episodes: []contracts.PredictiveStageReview{{ID: "holdout", ReviewID: "review-1", ReviewedBy: "maintainer",
			ReviewedAt: "2026-10-20T10:30:00Z", FeatureCutoffAt: "2026-10-20T11:00:00Z", ManifestSHA256: strings.Repeat("c", 64),
			Environments: []contracts.PredictiveStageEnvironment{{Key: "ci", CapacityUnits: 2, SetupDurationMS: 100}},
			Tests:        []contracts.PredictiveStageTestConstraint{}}}}
	for _, test := range ranking.Episodes[0].Ranked {
		constraints.Episodes[0].Tests = append(constraints.Episodes[0].Tests, contracts.PredictiveStageTestConstraint{
			ID: test.ID, Environment: "ci", ResourceUnits: 1, ColdStartDurationMS: 5000, Dependencies: []string{}})
	}
	constraints.Episodes[0].Tests[0].Dependencies = []string{constraints.Episodes[0].Tests[1].ID}
	save(t, directory, "constraints.json", constraints)
	save(t, directory, "index.json", index)
	args := []string{"-index", filepath.Join(directory, "index.json"), "-constraints", filepath.Join(directory, "constraints.json"), "-budget-ms", "100"}
	var output bytes.Buffer
	if err := replaycli.RunPortfolio(args, &output); err != nil {
		t.Fatal(err)
	}
	var report contracts.PredictivePortfolioV1
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if err := contracts.ValidatePredictivePortfolioV1(report); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(directory, "constraints.json"))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	e := report.Episodes[0]
	if report.Costs.ConstraintsSHA256 != hex.EncodeToString(digest[:]) || e.Mode != "full-control" ||
		!e.OverBudget || e.EstimatedDurationMS != 1800 || len(e.Selected) != 2 || len(e.RequiredTestIDs) != 2 || len(e.OmittedTestIDs) != 0 {
		t.Fatalf("incomplete portable fallback: %+v", report)
	}
	constraints.Episodes[0].Tests[1].Dependencies = []string{constraints.Episodes[0].Tests[0].ID}
	save(t, directory, "constraints.json", constraints)
	output.Reset()
	if err := replaycli.RunPortfolio(args, &output); err == nil || output.Len() != 0 {
		t.Fatal("cycle accepted or partial report emitted")
	}

	return report
}

func TestPortfolioPortableReport(t *testing.T) { portfolioReport(t) }

func TestPortfolioRejectsTamperedReport(t *testing.T) {
	original := portfolioReport(t)
	cases := map[string]func(*contracts.PredictivePortfolioV1){
		"missing-candidate": func(r *contracts.PredictivePortfolioV1) { r.Episodes[0].Selected = r.Episodes[0].Selected[:1] },
		"duplicate":         func(r *contracts.PredictivePortfolioV1) { r.Episodes[0].Selected[1] = r.Episodes[0].Selected[0] },
		"timing":            func(r *contracts.PredictivePortfolioV1) { r.Episodes[0].Selected[0].StartMS++ },
		"capacity":          func(r *contracts.PredictivePortfolioV1) { r.Costs.Episodes[0].Tests[0].ResourceUnits = 3 },
		"required": func(r *contracts.PredictivePortfolioV1) {
			r.Episodes[0].RequiredTestIDs = r.Episodes[0].RequiredTestIDs[:1]
		},
		"mode": func(r *contracts.PredictivePortfolioV1) {
			r.Episodes[0].Mode = "portfolio"
			r.Episodes[0].FallbackReason = ""
		},
		"overrun": func(r *contracts.PredictivePortfolioV1) { r.Episodes[0].OverBudget = false },
		"unknown-omission": func(r *contracts.PredictivePortfolioV1) {
			r.Episodes[0].OmittedTestIDs = []string{strings.Repeat("f", 64)}
		},
		"dependency": func(r *contracts.PredictivePortfolioV1) {
			r.Episodes[0].Selected[0], r.Episodes[0].Selected[1] = r.Episodes[0].Selected[1], r.Episodes[0].Selected[0]
		},
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			var report contracts.PredictivePortfolioV1
			if err := json.Unmarshal(data, &report); err != nil {
				t.Fatal(err)
			}
			change(&report)
			if err := contracts.ValidatePredictivePortfolioV1(report); err == nil {
				t.Fatal("invalid report accepted")
			}
		})
	}
}

func TestPortfolioRejectsInvalidInputWithoutOutput(t *testing.T) {
	for _, args := range [][]string{
		{}, {"-index", "missing", "-constraints", "missing", "-budget-ms", "0"},
		{"-index", "missing", "-constraints", "missing", "-budget-ms", "86400001"},
		{"-index", "missing", "-constraints", "missing", "-budget-ms", "100"},
	} {
		var output bytes.Buffer
		if err := replaycli.RunPortfolio(args, &output); err == nil || output.Len() != 0 {
			t.Fatal("invalid inputs accepted")
		}
	}
	directory := t.TempDir()
	save(t, directory, "bad.json", map[string]any{"apiVersion": "argus.dev/predictive-stage-constraints/v1", "episodes": []any{}})
	var output bytes.Buffer
	if err := replaycli.RunPortfolio([]string{"-index", "missing", "-constraints", filepath.Join(directory, "bad.json"), "-budget-ms", "100"}, &output); err == nil || output.Len() != 0 {
		t.Fatal("invalid review accepted")
	}
}
func TestPortfolioEligibleReportRetainsDecisionCapabilities(t *testing.T) {
	directory := t.TempDir()
	index := calibrationArchive(t, directory)
	holdout := observation(t, 43, "tests-a", "full-b", "2026-10-20T11:00:00Z")
	holdout.Observation.Tests[0].Capabilities = []string{"orders"}
	var err error
	holdout, err = contracts.NewPredictiveObservationEnvelopeV1(holdout.Observation)
	if err != nil {
		t.Fatal(err)
	}
	save(t, directory, "holdout.json", holdout)
	ranking, _ := rankingReport(t, directory, index)
	constraints := contracts.PredictiveStageConstraintsV1{APIVersion: "argus.dev/predictive-stage-constraints/v1",
		Episodes: []contracts.PredictiveStageReview{{ID: "holdout", ReviewID: "review-1", ReviewedBy: "maintainer",
			ReviewedAt: "2026-10-20T10:30:00Z", FeatureCutoffAt: "2026-10-20T11:00:00Z", ManifestSHA256: strings.Repeat("c", 64),
			Environments: []contracts.PredictiveStageEnvironment{{Key: "ci", CapacityUnits: 1, SetupDurationMS: 100}},
			Tests:        []contracts.PredictiveStageTestConstraint{{ID: ranking.Episodes[0].Ranked[0].ID, Environment: "ci", ResourceUnits: 1, ColdStartDurationMS: 5000, Dependencies: []string{}}}}}}
	save(t, directory, "constraints.json", constraints)
	save(t, directory, "index.json", index)
	var output bytes.Buffer
	if err := replaycli.RunPortfolio([]string{"-index", filepath.Join(directory, "index.json"), "-constraints", filepath.Join(directory, "constraints.json"), "-budget-ms", "1100"}, &output); err != nil {
		t.Fatal(err)
	}
	var report contracts.PredictivePortfolioV1
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	e := report.Episodes[0]
	if e.Mode != "portfolio" || e.EstimatedDurationMS != 1100 || len(e.Selected) != 1 ||
		len(e.CoveredCapabilities) != 1 || e.CoveredCapabilities[0] != "orders" {
		t.Fatalf("eligible report lost decision metadata: %+v", e)
	}
}
