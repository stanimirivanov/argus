package requestintent

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func projectionDecision(id string) AnalysisDecision {
	return AnalysisDecision{EventID: id, EvidenceFingerprint: sum([]byte(id)), ReportSHA256: sum([]byte("report")), PublishedAt: time.Date(2026, 10, 31, 0, 0, 0, 123456789, time.UTC), Late: true,
		Rules: []AnalysisRuleDecision{{RuleID: "latency", Metric: "http_req_duration", Statistic: "p95", Unit: "ms", Quality: AnalysisOutcome{Status: "PASS"}, SLO: AnalysisOutcome{Status: "PASS"}, Regression: AnalysisOutcome{Status: "FAIL", Reasons: []string{"regression_detected"}}}}}
}

func TestAnalysisCollectorPreservesIndependentOwnedOrderedEvidence(t *testing.T) {
	var collector AnalysisCollector
	if result := collector.Result(); result.Decisions == nil || len(result.Decisions) != 0 || result.Conflict {
		t.Fatal("missing evidence fabricated a decision")
	}
	for _, id := range []string{"b", "a"} {
		if err := collector.Add(projectionDecision(id)); err != nil {
			t.Fatal(err)
		}
	}
	if err := collector.Add(projectionDecision("a")); err != nil {
		t.Fatal(err)
	}
	result := collector.Result()
	if len(result.Decisions) != 2 || result.Decisions[0].EventID != "a" || result.Conflict || result.Decisions[0].Rules[0].SLO.Status != "PASS" || result.Decisions[0].Rules[0].Regression.Status != "FAIL" || !result.Decisions[0].Late {
		t.Fatal("outcomes/order changed")
	}
	result.Decisions[0].Rules[0].Regression.Reasons[0] = "changed"
	if collector.Result().Decisions[0].Rules[0].Regression.Reasons[0] != "regression_detected" {
		t.Fatal("aliased projection")
	}
	different := projectionDecision("c")
	different.ReportSHA256 = sum([]byte("another report"))
	different.PublishedAt = different.PublishedAt.Add(-time.Nanosecond)
	if err := collector.Add(different); err != nil {
		t.Fatal(err)
	}
	if result := collector.Result(); !result.Conflict || result.Decisions[0].EventID != "c" || len(result.Decisions) != 3 {
		t.Fatal("conflicting evidence hidden")
	}
}

func TestAnalysisCollectorRejectsChangedReuseAndBounds(t *testing.T) {
	var collector AnalysisCollector
	first := projectionDecision("event")
	if err := collector.Add(first); err != nil {
		t.Fatal(err)
	}
	first.Rules[0].Regression.Reasons[0] = "caller mutation"
	changed := projectionDecision("event")
	changed.Rules[0].Regression.Status = "PASS"
	if err := collector.Add(changed); !errors.Is(err, ErrConflict) {
		t.Fatal("changed reuse accepted", err)
	}
	invalid := projectionDecision("bad")
	invalid.Rules[0].Quality.Status = "FAIL"
	if err := collector.Add(invalid); !errors.Is(err, ErrInvalid) {
		t.Fatal("quality collapsed to FAIL")
	}
	huge := projectionDecision("huge")
	huge.Rules[0].Regression.Reasons = []string{strings.Repeat("x", AnalysisProjectionMaximumBytes)}
	if err := collector.Add(huge); !errors.Is(err, ErrInvalid) {
		t.Fatal("unbounded output accepted")
	}
	if result := collector.Result(); len(result.Decisions) != 1 || result.Decisions[0].Rules[0].Regression.Reasons[0] != "regression_detected" {
		t.Fatal("rejection mutated collector")
	}
	collector.decisions = make([]AnalysisDecision, 1000)
	if err := collector.Add(projectionDecision("overflow")); !errors.Is(err, ErrInvalid) {
		t.Fatal("history capacity bypassed")
	}
}
