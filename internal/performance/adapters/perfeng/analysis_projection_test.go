package perfeng

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

func TestProjectAnalysisDecisionKeepsIndependentStatusReasonsAndLateEvidence(t *testing.T) {
	f := conformanceFixture(t)
	deadline, err := time.Parse(time.RFC3339Nano, f.publication.Data.Context.Deadline)
	if err != nil {
		t.Fatal(err)
	}
	f.publication.Time = deadline.Add(time.Microsecond).Format(time.RFC3339Nano)
	admitted, review, raw := conformanceInputs(t, f)
	checked, err := VerifyReviewedAnalysis(t.Context(), admitted, review, raw)
	if err != nil {
		t.Fatal(err)
	}
	event, err := AnalysisObservation(t.Context(), checked)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := ProjectAnalysisDecision(t.Context(), event)
	if err != nil || !decision.Late || decision.ReportSHA256 != event.ReportSHA256() || len(decision.Rules) != len(f.report.Evaluations) {
		t.Fatal("projection pins lost", err)
	}
	for i, rule := range decision.Rules {
		expected := f.report.Evaluations[i]
		if rule.Quality.Status != expected.Quality.Status || rule.SLO.Status != expected.SLO.Status || rule.Regression.Status != expected.Regression.Status {
			t.Fatal("independent status changed")
		}
		if !reflect.DeepEqual([][]string{rule.Quality.Reasons, rule.SLO.Reasons, rule.Regression.Reasons}, [][]string{expected.Quality.Reasons, expected.SLO.Reasons, expected.Regression.Reasons}) {
			t.Fatal("reported reasons changed")
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := ProjectAnalysisDecision(ctx, event); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := ProjectAnalysisDecision(t.Context(), requestintent.AnalysisEvidence{}); err == nil {
		t.Fatal("zero evidence accepted")
	}
}
