package browsercli

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
	"github.com/stanimirivanov/argus/internal/execution/adapters/cli/planningcli"
)

func stagePromotionArgs(t *testing.T, dir, schedule string, repo contracts.RepositoryReference, family string) []string {
	t.Helper()
	p := contracts.PredictivePromotionPolicyV1{APIVersion: "argus.dev/predictive-promotion-policy/v1", Repository: repo, Family: family, ReviewID: "owner-review", ReviewedBy: "maintainer", ReviewedAt: "2026-10-21T00:00:00Z", ExpiresAt: "2026-10-23T00:00:00Z", ScheduleSHA256: schedule, ModelSHA256: strings.Repeat("d", 64), CalibrationSHA256: strings.Repeat("e", 64), MaxEvidenceAgeSeconds: 86400, ValidForSeconds: 60, MinReplayEpisodes: 1, MinShadowRuns: 1, MinFailures: 1, MaxMissRate: 0, MaxECE: 1, MaxECEDrift: 1, MaxStageDurationMS: 10000, MaxStageResourceUnitMS: 10000}
	policy := stageJSON(t, dir, "promotion-policy.json", p)
	rate := 0.0
	m := contracts.PredictivePromotionMeasurement{Samples: 1, Failures: 1, Misses: 0, MissRate: &rate}
	d := contracts.PredictivePromotionDecisionV1{APIVersion: "argus.dev/predictive-promotion-decision/v1", PolicySHA256: promotionDigest(policy), ReplaySHA256: strings.Repeat("1", 64), ShadowIndexSHA256: strings.Repeat("2", 64), ScheduleSHA256: schedule, ModelSHA256: p.ModelSHA256, CalibrationSHA256: p.CalibrationSHA256, Repository: repo, Family: family, EvaluatedAt: "2026-10-22T10:01:00Z", ExpiresAt: "2026-10-22T10:02:00Z", Mode: "staged", RemainingRequired: true, Reasons: []string{}, Replay: m, Shadow: m}
	decision := stageJSON(t, dir, "promotion-decision.json", d)

	return []string{"-promotion-policy", filepath.Join(dir, "promotion-policy.json"), "-promotion-decision", filepath.Join(dir, "promotion-decision.json"), "-promotion-sha256", promotionDigest(decision), "-promotion-as-of", d.EvaluatedAt}
}

func promotionDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func browserPromotionArgs(t *testing.T, f browserStageFixture) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.directory, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := contracts.DecodeBrowserSelectionManifestV1(data)
	if err != nil {
		t.Fatal(err)
	}

	return stagePromotionArgs(t, f.directory, f.sha, manifest.Decisions[0].TestRepository, "functional-ui")
}

func TestBrowserStagedPlanningFallsBackWithoutCurrentAuthority(t *testing.T) {
	for _, configured := range []bool{false, true} {
		f := browserStageInputs(t)
		args := f.args("staged")
		if configured {
			authority := browserPromotionArgs(t, f)
			authority[7] = "2026-10-22T10:02:00Z"
			args = append(args, authority...)
		}
		var output bytes.Buffer
		if err := planningcli.RunBrowser(t.Context(), args, nil, &output); err != nil {
			t.Fatal(err)
		}
		var plan contracts.BrowserStagePlanV2
		if err := json.Unmarshal(output.Bytes(), &plan); err != nil {
			t.Fatal(err)
		}
		if plan.Mode != "shadow" || len(plan.Groups) != 2 || len(plan.Groups[0].RemainingLater) != 1 {
			t.Fatalf("fallback: %+v", plan)
		}
	}
}
