package predictive

import (
	"math"
	"slices"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
)

// PromotionPolicy is authenticated owner input. Limits measure observed risk,
// not a statistical guarantee about future failures. All later work is required.
type PromotionPolicy struct {
	Repository                                     catalog.RepositoryIdentity
	RepositoryKey, Family, ReviewID, ReviewedBy    string
	ScheduleSHA256, ModelSHA256, CalibrationSHA256 string
	ReviewedAt, ExpiresAt                          time.Time
	MaxEvidenceAge, ValidFor                       time.Duration
	MinReplayEpisodes, MinShadowRuns, MinFailures  int
	MaxMissRate, MaxECE, MaxECEDrift               float64
	MaxStageDurationMS, MaxStageResourceUnitMS     int64
}

// PromotionSample is one evaluated held-out candidate. Its selection was frozen
// before its outcome; missing probabilities or abstention deny promotion.
type PromotionSample struct {
	EpisodeID, RepositoryKey, Family string
	Selected, Failed, Eligible       bool
	Probability                      *float64
	EstimatedMS, ResourceUnitMS      int64
}

// PromotionReplay is rederived from the chronological archive, not scalar input.
type PromotionReplay struct {
	EvidenceSHA256, ScheduleSHA256, ModelSHA256, CalibrationSHA256 string
	HoldoutEnd, LastCompletedAt                                    time.Time
	ControlIDs                                                     []string
	Samples                                                        []PromotionSample
}

// PromotionMeasurement is scoped evidence retained even when limits are breached.
type PromotionMeasurement struct {
	Samples, Failures, Misses int
	MissRate                  *float64
}

// PromotionDecision is an expiring observation-based choice, never release
// authority. Missing/expired/breached evidence always returns shadow mode.
type PromotionDecision struct {
	Mode                   string
	Reasons                []string
	EvaluatedAt, ExpiresAt time.Time
	Replay, Shadow         PromotionMeasurement
}

// EvaluatePromotion re-evaluates current evidence without persisted promotion
// state, clocks, I/O, or authority to omit the remaining-later phase.
func EvaluatePromotion(p PromotionPolicy, replay PromotionReplay, shadow SafetyReport, now time.Time) (PromotionDecision, error) {
	if err := ValidatePromotionPolicy(p); err != nil {
		return PromotionDecision{}, err
	}
	d := PromotionDecision{Mode: "shadow", Reasons: []string{}, EvaluatedAt: now, ExpiresAt: now.Add(p.ValidFor)}
	d.ExpiresAt = earliestPromotionTime(d.ExpiresAt, p.ExpiresAt, shadow.Window.AsOf.Add(p.MaxEvidenceAge), replay.LastCompletedAt.Add(p.MaxEvidenceAge))
	if now.IsZero() || now.Before(p.ReviewedAt) || !now.Before(p.ExpiresAt) {
		d.deny("policy-not-current")
	}
	if replay.HoldoutEnd.IsZero() || replay.LastCompletedAt.IsZero() || shadow.Window.Start.Before(replay.HoldoutEnd) || shadow.Window.Start.Before(replay.LastCompletedAt) {
		d.deny("non-prospective-shadow")
	}
	if shadow.Window.AsOf.After(now) || !now.Before(d.ExpiresAt) {
		d.deny("evidence-not-current")
	}
	if replay.ScheduleSHA256 != p.ScheduleSHA256 || replay.ModelSHA256 != p.ModelSHA256 || replay.CalibrationSHA256 != p.CalibrationSHA256 || !safetyDigest(replay.EvidenceSHA256) {
		d.deny("replay-provenance-mismatch")
	}
	evaluatePromotionReplay(p, replay, &d)
	evaluatePromotionShadow(p, replay, shadow, &d)
	measurePromotionRisk(p, &d.Replay, "replay", &d)
	measurePromotionRisk(p, &d.Shadow, "shadow", &d)
	if len(d.Reasons) == 0 {
		d.Mode = "staged"
	}
	slices.Sort(d.Reasons)

	return d, nil
}

// ValidatePromotionPolicy rejects malformed policy rather than silently choosing
// defaults or replacing an owner's risk/cost limits with implementation values.
func ValidatePromotionPolicy(p PromotionPolicy) error {
	if p.Repository.Provider == "" || p.Repository.Host == "" || p.Repository.ProviderRepositoryID == "" || !safetyDigest(p.RepositoryKey) ||
		(p.Family != "functional-api" && p.Family != "functional-ui") || p.ReviewID == "" || p.ReviewedBy == "" ||
		!safetyDigest(p.ScheduleSHA256) || !safetyDigest(p.ModelSHA256) || !safetyDigest(p.CalibrationSHA256) ||
		p.ReviewedAt.IsZero() || !p.ExpiresAt.After(p.ReviewedAt) || p.ValidFor <= 0 || p.ValidFor > 24*time.Hour ||
		p.MaxEvidenceAge <= 0 || p.MaxEvidenceAge > 365*24*time.Hour || p.MinReplayEpisodes < 1 || p.MinShadowRuns < 1 || p.MinFailures < 1 ||
		!validProbability(p.MaxMissRate) || !validProbability(p.MaxECE) || !validProbability(p.MaxECEDrift) ||
		p.MaxStageDurationMS < 1 || p.MaxStageResourceUnitMS < 1 {
		return ErrInvalid
	}

	return nil
}

func evaluatePromotionReplay(p PromotionPolicy, r PromotionReplay, d *PromotionDecision) {
	episodes := map[string]bool{}
	predictions := []CalibratedTest{}
	for _, s := range r.Samples {
		if s.RepositoryKey != p.RepositoryKey || s.Family != p.Family {
			continue
		}
		episodes[s.EpisodeID] = true
		validatePromotionSample(p, s, d)
		if s.Probability == nil || !validProbability(*s.Probability) {
			d.deny("replay-calibration-unavailable")
		} else {
			predictions = append(predictions, CalibratedTest{Probability: *s.Probability, Failed: s.Failed})
		}
		if s.Failed {
			d.Replay.Failures++
			if !s.Selected {
				d.Replay.Misses++
			}
		}
	}
	d.Replay.Samples = len(episodes)
	if len(episodes) < p.MinReplayEpisodes {
		d.deny("insufficient-replay-episodes")
	}
	if len(predictions) == 0 || measureCalibration(cohortKey{}, predictions).ExpectedError > p.MaxECE {
		d.deny("replay-calibration-breach")
	}
}

func validatePromotionSample(p PromotionPolicy, s PromotionSample, d *PromotionDecision) {
	if !s.Eligible {
		d.deny("replay-abstention")
	}
	if s.EstimatedMS > p.MaxStageDurationMS || s.ResourceUnitMS > p.MaxStageResourceUnitMS {
		d.deny("replay-cost-breach")
	}
}

func evaluatePromotionShadow(p PromotionPolicy, r PromotionReplay, report SafetyReport, d *PromotionDecision) {
	runs, replayControls := map[string]bool{}, map[string]bool{}
	for _, id := range r.ControlIDs {
		replayControls[id] = true
	}
	for _, g := range report.Groups {
		i := g.Input
		if i.Scope.Repository.Identity != p.Repository || string(i.Scope.Family) != p.Family {
			continue
		}
		runs[i.RunID] = true
		validatePromotionShadowGroup(p, g, replayControls, d)
		if g.Failures != nil {
			d.Shadow.Failures += *g.Failures
		}
		d.Shadow.Misses += len(g.Misses)
	}
	d.Shadow.Samples = len(runs)
	if len(runs) < p.MinShadowRuns {
		d.deny("insufficient-shadow-runs")
	}
}

func validatePromotionShadowGroup(p PromotionPolicy, g SafetyGroup, replayControls map[string]bool, d *PromotionDecision) {
	i := g.Input
	if i.Mode != "shadow" || g.State != "complete" || g.UnknownLabels != 0 {
		d.deny("shadow-incomplete")
	}
	if i.Schedule.SHA256 != p.ScheduleSHA256 || i.ModelSHA256 == nil || *i.ModelSHA256 != p.ModelSHA256 || i.Calibration == nil || i.Calibration.SHA256 != p.CalibrationSHA256 {
		d.deny("shadow-provenance-mismatch")
	}
	if g.ECE == nil || !validProbability(*g.ECE) || *g.ECE > p.MaxECE || g.ECEDrift == nil || math.IsNaN(*g.ECEDrift) || math.Abs(*g.ECEDrift) > p.MaxECEDrift {
		d.deny("shadow-calibration-breach")
	}
	for _, phase := range g.Phases {
		if phase.EstimatedDurationMS > p.MaxStageDurationMS || phase.EstimatedResourceUnitMS > p.MaxStageResourceUnitMS {
			d.deny("shadow-cost-breach")
		}
		if phase.AttemptID != nil && replayControls[*phase.AttemptID] {
			d.deny("reused-replay-attempt")
		}
	}
}

func measurePromotionRisk(p PromotionPolicy, m *PromotionMeasurement, name string, d *PromotionDecision) {
	if m.Failures < p.MinFailures {
		d.deny(name + "-insufficient-failing-controls")
	}
	if m.Failures > 0 {
		rate := float64(m.Misses) / float64(m.Failures)
		m.MissRate = &rate
		if rate > p.MaxMissRate {
			d.deny(name + "-miss-breach")
		}
	}
}

func (d *PromotionDecision) deny(reason string) {
	if !slices.Contains(d.Reasons, reason) {
		d.Reasons = append(d.Reasons, reason)
	}
}

func earliestPromotionTime(first time.Time, rest ...time.Time) time.Time {
	for _, t := range rest {
		if t.Before(first) {
			first = t
		}
	}

	return first
}
