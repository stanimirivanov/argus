//go:build integration

package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

func TestDecisionSnapshotEmptyScopeAndConcurrentHistory(t *testing.T) {
	f, event, verify := analysisStorageFixture(t)
	admit := func(child requestintent.Child, outcome requestintent.Outcome) (requestintent.LifecycleAdmission, error) {
		if child.SHA256 != f.claim.Pending.Child.SHA256 || outcome.Fingerprint() != f.admission.Fingerprint() {
			return requestintent.LifecycleAdmission{}, requestintent.ErrInvalid
		}
		return event.Data().Admission, nil
	}
	read := func(scope requestintent.Scope, project requestintent.AnalysisProjector) (requestintent.DecisionSnapshot, error) {
		return f.store.DecisionSnapshot(t.Context(), scope, f.claim.Pending.After, admit, verify, project)
	}
	empty, err := read(f.scope, projectionFixture)
	if err != nil || empty.Intent.Fingerprint() == "" || empty.Child.SHA256 != f.claim.Pending.Child.SHA256 || len(empty.Analysis.Decisions) != 0 {
		t.Fatal("known empty snapshot", err)
	}
	wrong := f.scope
	wrong.PrincipalID += "-other"
	if got, err := read(wrong, projectionFixture); !errors.Is(err, requestintent.ErrNotFound) || got.Intent.Fingerprint() != "" {
		t.Fatal("scope leak", err)
	}
	if _, err := f.store.AppendAnalysis(t.Context(), f.scope, f.claim.Pending.After, event, verify); err != nil {
		t.Fatal(err)
	}
	// Admit and lifecycle reads have already fixed the repeatable-read snapshot.
	inserted := false
	project := func(ctx context.Context, original requestintent.AnalysisEvidence) (requestintent.AnalysisDecision, error) {
		if !inserted {
			inserted = true
			data := event.Data()
			data.EventID = "concurrent"
			data.Report = []byte(`{"other":true}`)
			other, err := requestintent.NewAnalysisEvidence(data)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.store.AppendAnalysis(ctx, f.scope, f.claim.Pending.After, other, verify); err != nil {
				t.Fatal(err)
			}
		}
		return projectionFixture(ctx, original)
	}
	got, err := read(f.scope, project)
	if err != nil || len(got.Analysis.Decisions) != 1 {
		t.Fatal("snapshot changed", err)
	}
	later, err := read(f.scope, projectionFixture)
	if err != nil || len(later.Analysis.Decisions) != 2 || !later.Analysis.Conflict {
		t.Fatal("later evidence lost", err)
	}
	_, err = f.store.runtime.pool.Exec(t.Context(), `ALTER TABLE argus_catalog.performance_analysis_events DISABLE TRIGGER trg_performance_analysis_immutable;
 UPDATE argus_catalog.performance_analysis_events SET fingerprint=repeat('0',64) WHERE event_id='stored-analysis';
 ALTER TABLE argus_catalog.performance_analysis_events ENABLE TRIGGER trg_performance_analysis_immutable`)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := read(f.scope, projectionFixture); !errors.Is(err, requestintent.ErrUnavailable) || got.Intent.Fingerprint() != "" || got.Analysis.Decisions != nil {
		t.Fatal("partial corrupt snapshot", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := f.store.DecisionSnapshot(ctx, f.scope, f.claim.Pending.After, admit, verify, projectionFixture); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation", err)
	}
}
