package executioncli_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/execution/adapters/cli/planningcli"
	"github.com/stanimirivanov/argus/internal/predictive"
	predictivecli "github.com/stanimirivanov/argus/internal/predictive/adapters/cli"
)

func stagePromotionArgs(t *testing.T, dir, schedule string, repo contracts.RepositoryReference, family string) []string {
	t.Helper()
	p := contracts.PredictivePromotionPolicyV1{APIVersion: "argus.dev/predictive-promotion-policy/v1", Repository: repo, Family: family, ReviewID: "owner-review", ReviewedBy: "maintainer", ReviewedAt: "2026-10-21T00:00:00Z", ExpiresAt: "2026-10-23T00:00:00Z", ScheduleSHA256: schedule, ModelSHA256: strings.Repeat("d", 64), CalibrationSHA256: strings.Repeat("e", 64), MaxEvidenceAgeSeconds: 86400, ValidForSeconds: 60, MinReplayEpisodes: 1, MinShadowRuns: 1, MinFailures: 1, MaxMissRate: 0, MaxECE: 1, MaxECEDrift: 1, MaxStageDurationMS: 10000, MaxStageResourceUnitMS: 10000}
	policy := saveStageJSON(t, dir, "promotion-policy.json", p)
	rate := 0.0
	m := contracts.PredictivePromotionMeasurement{Samples: 1, Failures: 1, Misses: 0, MissRate: &rate}
	d := contracts.PredictivePromotionDecisionV1{APIVersion: "argus.dev/predictive-promotion-decision/v1", PolicySHA256: promotionDigest(policy), ReplaySHA256: strings.Repeat("1", 64), ShadowIndexSHA256: strings.Repeat("2", 64), ScheduleSHA256: schedule, ModelSHA256: p.ModelSHA256, CalibrationSHA256: p.CalibrationSHA256, Repository: repo, Family: family, EvaluatedAt: "2026-10-22T10:01:00Z", ExpiresAt: "2026-10-22T10:02:00Z", Mode: "staged", RemainingRequired: true, Reasons: []string{}, Replay: m, Shadow: m}
	decision := saveStageJSON(t, dir, "promotion-decision.json", d)

	return []string{"-promotion-policy", filepath.Join(dir, "promotion-policy.json"), "-promotion-decision", filepath.Join(dir, "promotion-decision.json"), "-promotion-sha256", promotionDigest(decision), "-promotion-as-of", d.EvaluatedAt}
}

func promotionDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func apiPromotionArgs(t *testing.T, f stageFixture) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.directory, "bindings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var bindings contracts.FunctionalAPIStageBindingsV2
	if err := json.Unmarshal(data, &bindings); err != nil {
		t.Fatal(err)
	}

	return stagePromotionArgs(t, f.directory, f.sha, bindings.Groups[0].TestRepository, "functional-api")
}

func TestAPIStagedPlanningFallsBackWithoutCurrentAuthority(t *testing.T) {
	for _, name := range []string{"absent", "expired", "wrong-digest", "wrong-family", "denied"} {
		t.Run(name, func(t *testing.T) {
			f := stageInputs(t)
			args := f.planArgs("staged")
			if name != "absent" {
				authority := apiPromotionArgs(t, f)
				alterPromotionAuthority(t, f, name, authority)
				args = append(args, authority...)
			}
			var output bytes.Buffer
			if err := planningcli.Run(t.Context(), args, nil, &output); err != nil {
				t.Fatal(err)
			}
			var plan contracts.FunctionalAPIStagePlanV2
			if err := json.Unmarshal(output.Bytes(), &plan); err != nil {
				t.Fatal(err)
			}
			if plan.Mode != "shadow" || len(plan.Groups[0].RemainingLater) != 1 {
				t.Fatalf("fallback lost full obligation: %+v", plan)
			}
		})
	}
}

func alterPromotionAuthority(t *testing.T, f stageFixture, name string, authority []string) {
	t.Helper()
	switch name {
	case "expired":
		authority[7] = "2026-10-22T10:02:00Z"
	case "wrong-digest":
		authority[5] = strings.Repeat("0", 64)
	case "wrong-family", "denied":
		path := filepath.Join(f.directory, "promotion-decision.json")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		d, err := contracts.DecodePredictivePromotionDecisionV1(data)
		if err != nil {
			t.Fatal(err)
		}
		if name == "wrong-family" {
			d.Family = "functional-ui"
		} else {
			d.Mode = "shadow"
			d.Reasons = []string{"shadow-miss-breach"}
		}
		authority[5] = promotionDigest(saveStageJSON(t, f.directory, "promotion-decision.json", d))
	}
}

func TestPromotionCLIRevalidatesOriginalShadowWithoutTrustingScalarReports(t *testing.T) {
	f, index := apiSafetyFixture(t)
	authority := apiPromotionArgs(t, f)
	saveStageJSON(t, f.directory, "safety-index.json", index)
	args := []string{"-index", "replay-index", "-constraints", "constraints", "-budget-ms", "1000", "-shadow-index", filepath.Join(f.directory, "safety-index.json"), "-policy", authority[1], "-as-of", "2026-10-22T10:01:00Z"}
	loader := func(indexPath, constraints string, budget int64) (predictive.PromotionReplay, error) {
		if indexPath != "replay-index" || constraints != "constraints" || budget != 1000 {
			t.Fatal("composition inputs lost")
		}
		return predictive.PromotionReplay{EvidenceSHA256: strings.Repeat("1", 64), ScheduleSHA256: f.sha, ModelSHA256: strings.Repeat("d", 64), CalibrationSHA256: strings.Repeat("e", 64), HoldoutEnd: time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC), LastCompletedAt: time.Date(2026, 10, 20, 1, 0, 0, 0, time.UTC), ControlIDs: []string{"historical"}}, nil
	}
	var output bytes.Buffer
	if err := predictivecli.RunPromotion(args, &output, loader); err != nil {
		t.Fatal(err)
	}
	d, err := contracts.DecodePredictivePromotionDecisionV1(output.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if d.Mode != "shadow" || !d.RemainingRequired || len(d.Reasons) == 0 || d.ShadowIndexSHA256 != safetyReference(t, f.directory, "safety-index.json").SHA256 {
		t.Fatalf("sparse evidence authorized: %+v", d)
	}
	// Tamper with original evidence, not an output scalar: the entire CLI fails.
	index.Runs[0].Attempts[0].SHA256 = strings.Repeat("0", 64)
	saveStageJSON(t, f.directory, "safety-index.json", index)
	output.Reset()
	if err := predictivecli.RunPromotion(args, &output, loader); err == nil || output.Len() != 0 {
		t.Fatalf("tampered archive emitted a decision: %v", err)
	}
}
