//go:build integration

package postgres

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

func TestNextAnalysisScopesCursorRestartAndLateAdmission(t *testing.T) {
	store, data, pending := workerFixture(t)
	issuer := pending[0].Child.Request.Issuer
	for _, scope := range []requestintent.Scope{
		{Issuer: "other", PrincipalIssuer: data.Scope.PrincipalIssuer, PrincipalID: data.Scope.PrincipalID},
		{Issuer: data.Scope.Issuer, PrincipalIssuer: "other", PrincipalID: data.Scope.PrincipalID},
		{Issuer: data.Scope.Issuer, PrincipalIssuer: data.Scope.PrincipalIssuer, PrincipalID: "other"},
	} {
		assertAnalysisPendingError(t, store, scope, issuer, 0, requestintent.ErrNotFound)
	}
	assertAnalysisPendingError(t, store, data.Scope, issuer, 0, requestintent.ErrNotFound)
	// Hold the first unsettled child while another admission commits ahead of it.
	first, err := store.ClaimRequest(t.Context(), data.Scope, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.ClaimRequest(t.Context(), data.Scope, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SettleRequest(t.Context(), data.Scope, second, outcomeFixture(t, data.Scope, second, "accepted")); err != nil {
		t.Fatal(err)
	}
	got, err := store.NextAnalysis(t.Context(), data.Scope, issuer, 0)
	if err != nil || !reflect.DeepEqual(got, second.Pending) {
		t.Fatal("changed frozen child", err)
	}
	assertAnalysisPendingError(t, store, data.Scope, issuer+"/other", 0, requestintent.ErrNotFound)
	assertAnalysisPendingError(t, store, data.Scope, issuer, got.After, requestintent.ErrNotFound)
	if _, err := store.SettleRequest(t.Context(), data.Scope, first, outcomeFixture(t, data.Scope, first, "accepted")); err != nil {
		t.Fatal(err)
	}
	// A new statement sees the commit, but cannot go backwards without explicit reset.
	assertAnalysisPendingError(t, store, data.Scope, issuer, got.After, requestintent.ErrNotFound)
	restarted, err := store.NextAnalysis(t.Context(), data.Scope, issuer, 0)
	if err != nil || !reflect.DeepEqual(restarted, first.Pending) {
		t.Fatal("reset lost late admission", err)
	}
	next, err := store.NextAnalysis(t.Context(), data.Scope, issuer, restarted.After)
	if err != nil || !reflect.DeepEqual(next, second.Pending) {
		t.Fatal("cursor order changed", err)
	}
	third, err := store.ClaimRequest(t.Context(), data.Scope, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SettleRequest(t.Context(), data.Scope, third, outcomeFixture(t, data.Scope, third, "rejected")); err != nil {
		t.Fatal(err)
	}
	assertAnalysisPendingError(t, store, data.Scope, issuer, next.After, requestintent.ErrNotFound)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	assertAnalysisPendingError(t, store, data.Scope, issuer, 0, context.Canceled, ctx)
}

func assertAnalysisPendingError(t *testing.T, store *PerformanceStore, scope requestintent.Scope, issuer string, after int64, want error, contexts ...context.Context) {
	t.Helper()
	ctx := t.Context()
	if len(contexts) > 0 {
		ctx = contexts[0]
	}
	pending, err := store.NextAnalysis(ctx, scope, issuer, after)
	if !errors.Is(err, want) || !reflect.DeepEqual(pending, requestintent.Pending{}) {
		t.Fatal("unexpected partial work or error", err)
	}
}

func TestNextAnalysisIgnoresCancellationCooldown(t *testing.T) {
	store, data, pending := cancellationScheduleFixture(t)
	c := store.runtime.Performance()
	issuer := pending[0].Child.Request.Issuer
	if _, err := c.DeferCancellation(t.Context(), data.Scope, issuer, pending[0].After, time.Hour); err != nil {
		t.Fatal(err)
	}
	life, err := c.NextLifecycle(t.Context(), data.Scope, issuer, 0)
	if err != nil || life.After != pending[1].After {
		t.Fatal("fixture cooldown missing", err)
	}
	got, err := c.NextAnalysis(t.Context(), data.Scope, issuer, 0)
	if err != nil || !reflect.DeepEqual(got, pending[0]) {
		t.Fatal("cancellation suppressed analysis", err)
	}
}

func TestNextAnalysisKeepsTerminalAndPreviouslyAnalyzed(t *testing.T) {
	store, scope, claim, admission, verify := lifecycleStoreFixture(t, "lifecycle")
	event := storeLifecycleEvent(t, scope, claim, "terminal-analysis", 1, "CANCELLED")
	if _, err := store.AppendLifecycle(t.Context(), scope, claim.Pending.After, admission, event, verify); err != nil {
		t.Fatal(err)
	}
	got, err := store.NextAnalysis(t.Context(), scope, claim.Pending.Child.Request.Issuer, 0)
	if err != nil || !reflect.DeepEqual(got, claim.Pending) {
		t.Fatal("terminal admission excluded", err)
	}
	f, evidence, analysisVerify := analysisStorageFixture(t)
	if _, err := f.store.AppendAnalysis(t.Context(), f.scope, f.claim.Pending.After, evidence, analysisVerify); err != nil {
		t.Fatal(err)
	}
	got, err = f.store.NextAnalysis(t.Context(), f.scope, f.claim.Pending.Child.Request.Issuer, 0)
	if err != nil || !reflect.DeepEqual(got, f.claim.Pending) {
		t.Fatal("prior evidence suppressed analysis", err)
	}
}

func TestNextAnalysisConcurrentReadersAndQueryPlan(t *testing.T) {
	store, scope, claim, _, _ := lifecycleStoreFixture(t, "accepted")
	issuer := claim.Pending.Child.Request.Issuer
	var readers sync.WaitGroup
	for range 2 {
		readers.Go(func() {
			got, err := store.NextAnalysis(t.Context(), scope, issuer, 0)
			if err != nil || !reflect.DeepEqual(got, claim.Pending) {
				t.Error("reader claimed or changed work", err)
			}
		})
	}
	readers.Wait()
	rows, err := store.runtime.pool.Query(t.Context(), "EXPLAIN (COSTS OFF) "+nextAnalysisQuery, scope.Issuer, scope.PrincipalIssuer, scope.PrincipalID, issuer, int64(0))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var plan string
		if err := rows.Scan(&plan); err != nil {
			t.Fatal(err)
		}
		t.Log(plan)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}
