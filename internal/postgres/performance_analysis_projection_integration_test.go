//go:build integration

package postgres

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

func projectionFixture(ctx context.Context, event requestintent.AnalysisEvidence) (requestintent.AnalysisDecision, error) {
	if err := ctx.Err(); err != nil {
		return requestintent.AnalysisDecision{}, err
	}
	data := event.Data()

	return requestintent.AnalysisDecision{EventID: data.EventID, EvidenceFingerprint: event.Fingerprint(), ReportSHA256: event.ReportSHA256(), PublishedAt: data.PublishedAt, Late: true,
		Rules: []requestintent.AnalysisRuleDecision{{RuleID: "latency", Metric: "duration", Statistic: "p95", Unit: "ms", Quality: requestintent.AnalysisOutcome{Status: "PASS"}, SLO: requestintent.AnalysisOutcome{Status: "PASS"}, Regression: requestintent.AnalysisOutcome{Status: "FAIL"}}}}, nil
}

func TestAnalysisProjectionUsesOneScopedSnapshotAndKeepsConcurrentEvidence(t *testing.T) {
	f, event, verify := analysisStorageFixture(t)
	empty, err := f.store.AnalysisProjection(t.Context(), f.scope, f.claim.Pending.After, event.Data().Admission, verify, projectionFixture)
	if err != nil || len(empty.Decisions) != 0 {
		t.Fatal("known empty history", err)
	}
	if _, err := f.store.AppendAnalysis(t.Context(), f.scope, f.claim.Pending.After, event, verify); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	project := func(ctx context.Context, event requestintent.AnalysisEvidence) (requestintent.AnalysisDecision, error) {
		once.Do(func() {
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
			}
		})

		return projectionFixture(ctx, event)
	}
	type result struct {
		projection requestintent.AnalysisProjection
		err        error
	}
	done := make(chan result, 1)
	go func() {
		projection, err := f.store.AnalysisProjection(ctx, f.scope, f.claim.Pending.After, event.Data().Admission, verify, project)
		done <- result{projection, err}
	}()
	select {
	case <-entered:
	case got := <-done:
		t.Fatal("projection failed before snapshot", got.err)
	case <-t.Context().Done():
		t.Fatal("snapshot did not start")
	}
	data := event.Data()
	data.EventID = "another"
	data.PublishedAt = data.PublishedAt.Add(time.Nanosecond)
	data.Report = []byte(`{"different":true}`)
	other, err := requestintent.NewAnalysisEvidence(data)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.AppendAnalysis(t.Context(), f.scope, f.claim.Pending.After, other, verify); err != nil {
		t.Fatal(err)
	}
	close(release)
	got := <-done
	if got.err != nil || len(got.projection.Decisions) != 1 || got.projection.Conflict {
		t.Fatal("snapshot changed during traversal", got.err)
	}
	latest, err := f.store.AnalysisProjection(t.Context(), f.scope, f.claim.Pending.After, event.Data().Admission, verify, projectionFixture)
	if err != nil || len(latest.Decisions) != 2 || !latest.Conflict || latest.Decisions[0].Rules[0].Regression.Status != "FAIL" || latest.Decisions[0].Rules[0].SLO.Status != "PASS" {
		t.Fatal("conflicting outcomes hidden", err)
	}
}

func TestAnalysisProjectionRejectsScopeCorruptionAndPartialOutput(t *testing.T) {
	f, event, verify := analysisStorageFixture(t)
	if _, err := f.store.AppendAnalysis(t.Context(), f.scope, f.claim.Pending.After, event, verify); err != nil {
		t.Fatal(err)
	}
	wrong := f.scope
	wrong.PrincipalID += "-other"
	if _, err := f.store.AnalysisProjection(t.Context(), wrong, f.claim.Pending.After, event.Data().Admission, verify, projectionFixture); !errors.Is(err, requestintent.ErrNotFound) {
		t.Fatal("wrong scope", err)
	}
	invalid := func(ctx context.Context, event requestintent.AnalysisEvidence) (requestintent.AnalysisDecision, error) {
		d, err := projectionFixture(ctx, event)
		d.ReportSHA256 = "wrong"
		return d, err
	}
	if got, err := f.store.AnalysisProjection(t.Context(), f.scope, f.claim.Pending.After, event.Data().Admission, verify, invalid); !errors.Is(err, requestintent.ErrUnavailable) || got.Decisions != nil {
		t.Fatal("unbound projection accepted", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := f.store.AnalysisProjection(ctx, f.scope, f.claim.Pending.After, event.Data().Admission, verify, projectionFixture); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	data := event.Data()
	data.EventID = "z-corrupt"
	later, err := requestintent.NewAnalysisEvidence(data)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.AppendAnalysis(t.Context(), f.scope, f.claim.Pending.After, later, verify); err != nil {
		t.Fatal(err)
	}
	_, err = f.store.runtime.pool.Exec(t.Context(), `ALTER TABLE argus_catalog.performance_analysis_events DISABLE TRIGGER trg_performance_analysis_immutable;
 UPDATE argus_catalog.performance_analysis_events SET fingerprint=repeat('0',64) WHERE event_id='z-corrupt';
 ALTER TABLE argus_catalog.performance_analysis_events ENABLE TRIGGER trg_performance_analysis_immutable`)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	project := func(ctx context.Context, event requestintent.AnalysisEvidence) (requestintent.AnalysisDecision, error) {
		calls++
		return projectionFixture(ctx, event)
	}
	if got, err := f.store.AnalysisProjection(t.Context(), f.scope, f.claim.Pending.After, event.Data().Admission, verify, project); !errors.Is(err, requestintent.ErrUnavailable) || got.Decisions != nil || calls != 1 {
		t.Fatal("corruption produced output", err)
	}
}
