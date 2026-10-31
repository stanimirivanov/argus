package requestintent

import (
	"cmp"
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"
)

// AnalysisOutcome preserves one reported dimension, never an aggregate verdict.
type AnalysisOutcome struct {
	Status  string
	Reasons []string
}

// AnalysisRuleDecision identifies a reviewed rule and its original selector.
// Numeric measurements/effects remain in the exact report referenced by its pin.
type AnalysisRuleDecision struct {
	RuleID, Metric, Statistic, Unit string
	Quality, SLO, Regression        AnalysisOutcome
}

// AnalysisDecision is an owned compact projection of one verified publication.
// Late refers to provider publication time versus the frozen deadline, not receipt.
type AnalysisDecision struct {
	EventID, EvidenceFingerprint, ReportSHA256 string
	PublishedAt                                time.Time
	Late                                       bool
	Rules                                      []AnalysisRuleDecision
}

// AnalysisProjection preserves every event. Conflict means distinct report-byte
// identities, conservatively including metadata differences; no winner is selected.
type AnalysisProjection struct {
	Decisions []AnalysisDecision
	Conflict  bool
}

// AnalysisProjector must be pure, bounded and context-aware. It maps already
// verified original report bytes without I/O, aggregation or numerical analysis.
type AnalysisProjector func(context.Context, AnalysisEvidence) (AnalysisDecision, error)

// AnalysisProjectionMaximumBytes bounds the compact serialized response.
const AnalysisProjectionMaximumBytes = 8 << 20

// AnalysisCollector incrementally bounds owned compact evidence. Its zero value
// is ready; one caller must serialize Add/Result. Errors leave state unchanged.
// Result returns independent copies. No authentication or approval is granted.
type AnalysisCollector struct {
	decisions []AnalysisDecision
	seen      map[string]string
	report    string
	conflict  bool
	bytes     int
}

// Add accepts at most 1000 distinct events and an 8 MiB compact response. Exact
// duplicates are ignored; changed reuse conflicts. Status families stay separate.
func (collector *AnalysisCollector) Add(decision AnalysisDecision) error {
	if !text(decision.EventID, 256) || !digest(decision.EvidenceFingerprint) || !digest(decision.ReportSHA256) || decision.PublishedAt.IsZero() || len(decision.Rules) == 0 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, rule := range decision.Rules {
		if strings.TrimSpace(rule.RuleID) == "" || seen[rule.RuleID] || strings.TrimSpace(rule.Metric) == "" || strings.TrimSpace(rule.Statistic) == "" || strings.TrimSpace(rule.Unit) == "" || !validAnalysisOutcome(rule.Quality, true) || !validAnalysisOutcome(rule.SLO, false) || !validAnalysisOutcome(rule.Regression, false) {
			return ErrInvalid
		}
		seen[rule.RuleID] = true
	}
	decision.PublishedAt = decision.PublishedAt.UTC()
	raw, err := json.Marshal(decision)
	if err != nil {
		return ErrInvalid
	}
	fingerprint := sum(raw)
	if prior, exists := collector.seen[decision.EventID]; exists {
		if prior != fingerprint {
			return ErrConflict
		}

		return nil
	}
	if len(collector.decisions) >= 1000 || collector.bytes+len(raw)+1 > AnalysisProjectionMaximumBytes-128 {
		return ErrInvalid
	}
	if collector.seen == nil {
		collector.seen = map[string]string{}
	}
	collector.seen[decision.EventID] = fingerprint
	collector.bytes += len(raw) + 1
	collector.conflict = collector.conflict || (collector.report != "" && collector.report != decision.ReportSHA256)
	collector.report = decision.ReportSHA256
	collector.decisions = append(collector.decisions, cloneAnalysisDecision(decision))

	return nil
}

func validAnalysisOutcome(outcome AnalysisOutcome, quality bool) bool {
	valid := outcome.Status == "PASS" || outcome.Status == "INCONCLUSIVE" || outcome.Status == "NOT_EVALUATED" || (!quality && outcome.Status == "FAIL") || (quality && (outcome.Status == "INVALID" || outcome.Status == "UNSTABLE"))
	for _, reason := range outcome.Reasons {
		if strings.TrimSpace(reason) == "" {
			return false
		}
	}

	return valid
}

// Result returns owned chronological/event-ID ordered evidence. Known empty
// history returns an empty list; missing evidence never fabricates a PASS.
func (collector *AnalysisCollector) Result() AnalysisProjection {
	result := AnalysisProjection{Decisions: make([]AnalysisDecision, len(collector.decisions)), Conflict: collector.conflict}
	for i, decision := range collector.decisions {
		result.Decisions[i] = cloneAnalysisDecision(decision)
	}
	slices.SortFunc(result.Decisions, func(a, b AnalysisDecision) int {
		if order := a.PublishedAt.Compare(b.PublishedAt); order != 0 {
			return order
		}

		return cmp.Compare(a.EventID, b.EventID)
	})

	return result
}

func cloneAnalysisDecision(decision AnalysisDecision) AnalysisDecision {
	decision.Rules = slices.Clone(decision.Rules)
	for i := range decision.Rules {
		rule := &decision.Rules[i]
		rule.Quality.Reasons = slices.Clone(rule.Quality.Reasons)
		rule.SLO.Reasons = slices.Clone(rule.SLO.Reasons)
		rule.Regression.Reasons = slices.Clone(rule.Regression.Reasons)
	}

	return decision
}
