package perfeng

import (
	"context"
	"slices"
	"time"

	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

// ProjectAnalysisDecision maps already revalidated evidence to independent owned
// outcomes. The caller must authenticate/revalidate first; decoding alone grants
// no authority. Measurements/effects remain accessible by exact event/report pins.
// It performs no I/O or numerical/aggregate verdict calculation.
func ProjectAnalysisDecision(ctx context.Context, event requestintent.AnalysisEvidence) (requestintent.AnalysisDecision, error) {
	if err := ctx.Err(); err != nil {
		return requestintent.AnalysisDecision{}, err
	}
	if event.Fingerprint() == "" {
		return requestintent.AnalysisDecision{}, ErrAnalysisConformance
	}
	data := event.Data()
	report, err := DecodeAnalysis(data.Report)
	if err != nil {
		return requestintent.AnalysisDecision{}, ErrAnalysisConformance
	}
	publication, err := DecodePublication(data.Publication)
	if err != nil {
		return requestintent.AnalysisDecision{}, ErrAnalysisConformance
	}
	deadline, err := time.Parse(time.RFC3339Nano, publication.Data.Context.Deadline)
	if err != nil {
		return requestintent.AnalysisDecision{}, ErrAnalysisConformance
	}
	decision := requestintent.AnalysisDecision{EventID: data.EventID, EvidenceFingerprint: event.Fingerprint(), ReportSHA256: event.ReportSHA256(), PublishedAt: data.PublishedAt, Late: !data.PublishedAt.Before(deadline)}
	for _, rule := range report.Evaluations {
		decision.Rules = append(decision.Rules, requestintent.AnalysisRuleDecision{RuleID: rule.RuleID, Metric: rule.Metric.Name, Statistic: rule.Metric.Statistic, Unit: rule.Metric.Unit,
			Quality:    requestintent.AnalysisOutcome{Status: rule.Quality.Status, Reasons: slices.Clone(rule.Quality.Reasons)},
			SLO:        requestintent.AnalysisOutcome{Status: rule.SLO.Status, Reasons: slices.Clone(rule.SLO.Reasons)},
			Regression: requestintent.AnalysisOutcome{Status: rule.Regression.Status, Reasons: slices.Clone(rule.Regression.Reasons)}})
	}
	if err := ctx.Err(); err != nil {
		return requestintent.AnalysisDecision{}, err
	}

	return decision, nil
}
