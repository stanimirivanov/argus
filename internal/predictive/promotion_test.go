package predictive_test

import (
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/predictive"
)

func promotionFixture(t *testing.T, family catalog.TestFamily) (predictive.PromotionPolicy, predictive.PromotionReplay, predictive.SafetyReport, time.Time) {
	t.Helper()
	w, input := monitorFixture("shadow", family)
	report := monitorReport(t, w, input)
	p := predictive.PromotionPolicy{Repository: input.Scope.Repository.Identity, RepositoryKey: strings.Repeat("a", 64), Family: string(family), ReviewID: "owner-review", ReviewedBy: "maintainer", ReviewedAt: w.Start.Add(-24 * time.Hour), ExpiresAt: w.End, ScheduleSHA256: input.Schedule.SHA256, ModelSHA256: *input.ModelSHA256, CalibrationSHA256: input.Calibration.SHA256, MaxEvidenceAge: 24 * time.Hour, ValidFor: time.Minute, MinReplayEpisodes: 1, MinShadowRuns: 1, MinFailures: 1, MaxMissRate: 0.5, MaxECE: 0.5, MaxECEDrift: 0.5, MaxStageDurationMS: 1000, MaxStageResourceUnitMS: 2000}
	probability := 0.8
	r := predictive.PromotionReplay{EvidenceSHA256: strings.Repeat("1", 64), ScheduleSHA256: p.ScheduleSHA256, ModelSHA256: p.ModelSHA256, CalibrationSHA256: p.CalibrationSHA256, HoldoutEnd: w.Start.Add(-time.Hour), LastCompletedAt: w.Start.Add(-time.Minute), ControlIDs: []string{"historical-control"}, Samples: []predictive.PromotionSample{{EpisodeID: "historical", RepositoryKey: p.RepositoryKey, Family: p.Family, Selected: true, Failed: true, Eligible: true, Probability: &probability, EstimatedMS: 100, ResourceUnitMS: 200}}}

	return p, r, report, w.AsOf
}

func TestPromotionRequiresReplayAndIndependentShadowForEachFamily(t *testing.T) {
	for _, family := range []catalog.TestFamily{catalog.TestFamilyFunctionalAPI, catalog.TestFamilyFunctionalUI} {
		p, r, shadow, now := promotionFixture(t, family)
		d, err := predictive.EvaluatePromotion(p, r, shadow, now)
		if err != nil || d.Mode != "staged" || len(d.Reasons) != 0 || !d.ExpiresAt.Equal(now.Add(time.Minute)) || d.Replay.Failures != 1 || d.Shadow.Failures != 2 {
			t.Fatalf("promotion: %+v %v", d, err)
		}
		// There is no sticky state: the next evaluation immediately restores shadow.
		p.MaxMissRate = 0
		d, err = predictive.EvaluatePromotion(p, r, shadow, now)
		if err != nil || d.Mode != "shadow" || !slices.Contains(d.Reasons, "shadow-miss-breach") {
			t.Fatalf("breach did not demote: %+v %v", d, err)
		}
	}
}

func TestPromotionDeniesInsufficientStaleMismatchedOrBreachedEvidence(t *testing.T) {
	type inputs struct {
		p   predictive.PromotionPolicy
		r   predictive.PromotionReplay
		s   predictive.SafetyReport
		now time.Time
	}
	cases := map[string]struct {
		reason string
		mutate func(*inputs)
	}{
		"replay-count":          {"insufficient-replay-episodes", func(i *inputs) { i.p.MinReplayEpisodes = 2 }},
		"shadow-count":          {"insufficient-shadow-runs", func(i *inputs) { i.p.MinShadowRuns = 2 }},
		"no-failures":           {"replay-insufficient-failing-controls", func(i *inputs) { i.r.Samples[0].Failed = false }},
		"no-shadow-failures":    {"shadow-insufficient-failing-controls", func(i *inputs) { i.s.Groups[0].Failures = new(int); i.s.Groups[0].Misses = nil }},
		"policy-expired":        {"policy-not-current", func(i *inputs) { i.now = i.p.ExpiresAt }},
		"before-review":         {"policy-not-current", func(i *inputs) { i.p.ReviewedAt = i.now.Add(time.Second) }},
		"evidence-expired":      {"evidence-not-current", func(i *inputs) { i.p.MaxEvidenceAge = time.Second }},
		"future-evidence":       {"evidence-not-current", func(i *inputs) { i.s.Window.AsOf = i.now.Add(time.Second) }},
		"temporal-overlap":      {"non-prospective-shadow", func(i *inputs) { i.r.LastCompletedAt = i.s.Window.Start.Add(time.Second) }},
		"missing-replay":        {"non-prospective-shadow", func(i *inputs) { i.r.LastCompletedAt = time.Time{} }},
		"wrong-family":          {"insufficient-shadow-runs", func(i *inputs) { i.s.Groups[0].Input.Scope.Family = catalog.TestFamilyFunctionalUI }},
		"wrong-repository":      {"insufficient-shadow-runs", func(i *inputs) { i.s.Groups[0].Input.Scope.Repository.Identity.ProviderRepositoryID = "other" }},
		"wrong-model":           {"replay-provenance-mismatch", func(i *inputs) { i.r.ModelSHA256 = strings.Repeat("0", 64) }},
		"wrong-live-model":      {"shadow-provenance-mismatch", func(i *inputs) { v := strings.Repeat("0", 64); i.s.Groups[0].Input.ModelSHA256 = &v }},
		"wrong-calibration":     {"shadow-provenance-mismatch", func(i *inputs) { i.s.Groups[0].Input.Calibration = nil }},
		"wrong-portfolio":       {"shadow-provenance-mismatch", func(i *inputs) { i.s.Groups[0].Input.Schedule.SHA256 = strings.Repeat("0", 64) }},
		"pending":               {"shadow-incomplete", func(i *inputs) { i.s.Groups[0].State = "pending" }},
		"staged-not-shadow":     {"shadow-incomplete", func(i *inputs) { i.s.Groups[0].Input.Mode = "staged" }},
		"reused-control":        {"reused-replay-attempt", func(i *inputs) { i.r.ControlIDs = []string{*i.s.Groups[0].Phases[1].AttemptID} }},
		"replay-miss":           {"replay-miss-breach", func(i *inputs) { i.r.Samples[0].Selected = false }},
		"replay-abstention":     {"replay-abstention", func(i *inputs) { i.r.Samples[0].Eligible = false }},
		"replay-no-probability": {"replay-calibration-unavailable", func(i *inputs) { i.r.Samples[0].Probability = nil }},
		"replay-ece":            {"replay-calibration-breach", func(i *inputs) { v := 0.0; i.r.Samples[0].Probability = &v }},
		"drift":                 {"shadow-calibration-breach", func(i *inputs) { i.p.MaxECEDrift = 0.1 }},
		"missing-drift":         {"shadow-calibration-breach", func(i *inputs) { i.s.Groups[0].ECEDrift = nil }},
		"replay-cost":           {"replay-cost-breach", func(i *inputs) { i.r.Samples[0].ResourceUnitMS = 2001 }},
		"shadow-cost":           {"shadow-cost-breach", func(i *inputs) { i.p.MaxStageDurationMS = 100 }},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			p, r, s, now := promotionFixture(t, catalog.TestFamilyFunctionalAPI)
			i := inputs{p, r, s, now}
			c.mutate(&i)
			d, err := predictive.EvaluatePromotion(i.p, i.r, i.s, i.now)
			if err != nil || d.Mode != "shadow" || !slices.Contains(d.Reasons, c.reason) {
				t.Fatalf("decision %+v error %v, want %s", d, err, c.reason)
			}
		})
	}
}

func TestPromotionRejectsInvalidOwnerPolicy(t *testing.T) {
	for _, change := range []func(*predictive.PromotionPolicy){
		func(p *predictive.PromotionPolicy) { p.MinFailures = 0 },
		func(p *predictive.PromotionPolicy) { p.MaxMissRate = math.NaN() },
		func(p *predictive.PromotionPolicy) { p.MaxECE = math.Inf(1) },
		func(p *predictive.PromotionPolicy) { p.ValidFor = 25 * time.Hour },
		func(p *predictive.PromotionPolicy) { p.ReviewedBy = "" },
	} {
		p, r, s, now := promotionFixture(t, catalog.TestFamilyFunctionalAPI)
		change(&p)
		if _, err := predictive.EvaluatePromotion(p, r, s, now); err == nil {
			t.Fatal("invalid policy accepted")
		}
	}
}
