package replaycli_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/predictive/adapters/replaycli"
)

func TestPromotionReplayRecomputesExactPortfolioAndOriginalControlEvidence(t *testing.T) {
	dir := t.TempDir()
	index := calibrationArchive(t, dir)
	save(t, dir, "index.json", index)
	var calibrated bytes.Buffer
	if err := replaycli.RunCalibration([]string{"-index", filepath.Join(dir, "index.json")}, &calibrated); err != nil {
		t.Fatal(err)
	}
	var c contracts.PredictiveCalibrationV1
	if err := json.Unmarshal(calibrated.Bytes(), &c); err != nil {
		t.Fatal(err)
	}
	constraints := contracts.PredictiveStageConstraintsV1{APIVersion: "argus.dev/predictive-stage-constraints/v1", Episodes: []contracts.PredictiveStageReview{{ID: "holdout", ReviewID: "review", ReviewedBy: "maintainer", ReviewedAt: "2026-10-20T10:30:00Z", FeatureCutoffAt: "2026-10-20T11:00:00Z", ManifestSHA256: strings.Repeat("c", 64), Environments: []contracts.PredictiveStageEnvironment{{Key: "ci", CapacityUnits: 2, SetupDurationMS: 100}}, Tests: []contracts.PredictiveStageTestConstraint{}}}}
	for _, test := range c.Episodes[0].Predictions {
		constraints.Episodes[0].Tests = append(constraints.Episodes[0].Tests, contracts.PredictiveStageTestConstraint{ID: test.ID, Environment: "ci", ResourceUnits: 1, ColdStartDurationMS: 5000, Dependencies: []string{}})
	}
	save(t, dir, "constraints.json", constraints)
	indexPath, constraintsPath := filepath.Join(dir, "index.json"), filepath.Join(dir, "constraints.json")
	var scheduled bytes.Buffer
	if err := replaycli.RunPortfolio([]string{"-index", indexPath, "-constraints", constraintsPath, "-budget-ms", "100"}, &scheduled); err != nil {
		t.Fatal(err)
	}
	r, err := replaycli.LoadPromotionReplay(indexPath, constraintsPath, 100)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(scheduled.Bytes())
	model, err := json.Marshal(c.Model)
	if err != nil {
		t.Fatal(err)
	}
	modelSum := sha256.Sum256(model)
	calibrationSum := sha256.Sum256(calibrated.Bytes())
	if r.ScheduleSHA256 != hex.EncodeToString(sum[:]) || r.ModelSHA256 != hex.EncodeToString(modelSum[:]) || r.CalibrationSHA256 != hex.EncodeToString(calibrationSum[:]) || len(r.Samples) != 2 || len(r.ControlIDs) != 4 || r.LastCompletedAt.IsZero() || len(r.EvidenceSHA256) != 64 {
		t.Fatalf("missing provenance: %+v", r)
	}
	for _, sample := range r.Samples {
		if sample.Probability == nil || sample.ResourceUnitMS < 1 || sample.EstimatedMS < 1 {
			t.Fatalf("missing measurements: %+v", sample)
		}
	}
	// An altered archived observation must not become a valid scalar report.
	bad := observation(t, 1, "tests-a", "full-a", "2026-10-20T09:00:00Z")
	bad.SHA256 = strings.Repeat("0", 64)
	save(t, dir, "a.json", bad)
	index.Episodes[0].Observations = []string{"a.json"}
	save(t, dir, "index.json", index)
	if _, err := replaycli.LoadPromotionReplay(indexPath, constraintsPath, 100); err == nil {
		t.Fatal("tampered archive accepted")
	}
}
