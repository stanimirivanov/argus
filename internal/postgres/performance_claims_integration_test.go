//go:build integration

package postgres

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

func claimFixture(t *testing.T, expires time.Duration) (*integratedStore, requestintent.Data) {
	t.Helper()
	store, hash := requestSchemaStore(t, "claims")
	data := requestFixture(t, store, hash, "claims")
	if expires > 0 {
		for i := range data.Children {
			data.Children[i].Deadline = data.Children[i].RequestedAt.Add(expires)
		}
	}
	if _, err := store.runtime.Performance().CreateRequest(t.Context(), inwardRequest(t, data)); err != nil {
		t.Fatal(err)
	}

	return store, data
}

func attemptClaim(t *testing.T, capability *PerformanceStore, scope requestintent.Scope, claims chan<- requestintent.Claim) {
	t.Helper()
	claim, err := capability.ClaimRequest(t.Context(), scope, 5*time.Minute)
	if err == nil {
		claims <- claim
	} else if !errors.Is(err, requestintent.ErrNotFound) {
		t.Error(err)
	}
}

func rejectPerformanceRelease(t *testing.T, capability *PerformanceStore, scope requestintent.Scope, claim requestintent.Claim) {
	t.Helper()
	if err := capability.ReleaseRequest(t.Context(), scope, claim, 0); !errors.Is(err, requestintent.ErrConflict) {
		t.Fatal("invalid dispatch ownership was not rejected", err)
	}
}

func TestPerformanceClaimsConcurrentAndScoped(t *testing.T) {
	store, data := claimFixture(t, 0)
	capability := store.runtime.Performance()
	wrongScopes := []requestintent.Scope{
		{Issuer: "other", PrincipalIssuer: data.Scope.PrincipalIssuer, PrincipalID: data.Scope.PrincipalID},
		{Issuer: data.Scope.Issuer, PrincipalIssuer: "other", PrincipalID: data.Scope.PrincipalID},
		{Issuer: data.Scope.Issuer, PrincipalIssuer: data.Scope.PrincipalIssuer, PrincipalID: "other"},
	}
	for _, wrong := range wrongScopes {
		if _, err := capability.ClaimRequest(t.Context(), wrong, time.Minute); !errors.Is(err, requestintent.ErrNotFound) {
			t.Fatal("cross-scope claim", err)
		}
	}
	var workers sync.WaitGroup
	claims := make(chan requestintent.Claim, 4)
	for range 4 {
		workers.Go(func() { attemptClaim(t, capability, data.Scope, claims) })
	}
	workers.Wait()
	close(claims)
	seen := map[int64]bool{}
	for claim := range claims {
		if seen[claim.Pending.After] || claim.Fence != 1 || claim.Token == "" {
			t.Fatal("duplicate/unfenced claim", claim)
		}
		seen[claim.Pending.After] = true
		for _, wrong := range wrongScopes {
			rejectPerformanceRelease(t, capability, wrong, claim)
		}
		if err := capability.ReleaseRequest(t.Context(), data.Scope, claim, time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	if len(seen) != 2 {
		t.Fatalf("claimed %d distinct children", len(seen))
	}
	if _, err := capability.ClaimRequest(t.Context(), data.Scope, time.Minute); !errors.Is(err, requestintent.ErrNotFound) {
		t.Fatal("retry delay ignored", err)
	}
}

func TestPerformanceClaimsExpiryFenceAndReopen(t *testing.T) {
	store, data := claimFixture(t, 3*time.Second)
	capability := store.runtime.Performance()
	first, err := capability.ClaimRequest(t.Context(), data.Scope, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	// Wait using the database clock: lease expiry is the behavior under test.
	if _, err := store.pool.Exec(t.Context(), `SELECT pg_sleep(GREATEST(0,EXTRACT(EPOCH FROM (GREATEST($1::timestamptz,$2::timestamptz)-clock_timestamp())))+0.01)`, first.Until, first.Pending.Child.Deadline); err != nil {
		t.Fatal(err)
	}
	rejectPerformanceRelease(t, capability, data.Scope, first)
	url := store.pool.Config().ConnString()
	store.Close()
	capability = openTestStore(t, url).runtime.Performance()
	second, err := capability.ClaimRequest(t.Context(), data.Scope, 5*time.Minute)
	if err != nil || second.Pending.After != first.Pending.After || second.Fence != first.Fence+1 || second.Token == first.Token || !reflect.DeepEqual(second.Pending, first.Pending) {
		t.Fatal("reclaim lost evidence/fence", err)
	}
	for _, stale := range []requestintent.Claim{
		first,
		{Pending: second.Pending, Token: first.Token, Fence: second.Fence},
		{Pending: second.Pending, Token: second.Token, Fence: second.Fence + 1},
	} {
		rejectPerformanceRelease(t, capability, data.Scope, stale)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := capability.ClaimRequest(ctx, data.Scope, time.Minute); !errors.Is(err, context.Canceled) {
		t.Fatal("claim cancellation lost", err)
	}
	if err := capability.ReleaseRequest(ctx, data.Scope, second, 0); !errors.Is(err, context.Canceled) {
		t.Fatal("release cancellation lost", err)
	}
	if err := capability.ReleaseRequest(t.Context(), data.Scope, second, 0); err != nil {
		t.Fatal(err)
	}
	rejectPerformanceRelease(t, capability, data.Scope, second)
	third, err := capability.ClaimRequest(t.Context(), data.Scope, 5*time.Minute)
	if err != nil || third.Fence != second.Fence+1 || third.Pending.After != first.Pending.After {
		t.Fatal("retry reset fence", err)
	}
}

func TestPerformanceClaimsSkipLocksAndRollback(t *testing.T) {
	store, data := claimFixture(t, 0)
	capability := store.runtime.Performance()
	tx, err := store.pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(t.Context()) }() //nolint:errcheck // Best effort after explicit rollback.
	var locked int64
	if err := tx.QueryRow(t.Context(), `SELECT position FROM argus_catalog.performance_pending_dispatches ORDER BY position LIMIT 1 FOR UPDATE`).Scan(&locked); err != nil {
		t.Fatal(err)
	}
	claim, err := capability.ClaimRequest(t.Context(), data.Scope, 5*time.Minute)
	if err != nil || claim.Pending.After == locked {
		t.Fatal("locked row was not skipped", err)
	}
	if err := tx.Rollback(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.pool.Exec(t.Context(), `CREATE TRIGGER reject_test_claim AFTER INSERT ON argus_catalog.performance_dispatch_claims FOR EACH STATEMENT EXECUTE FUNCTION argus_catalog.reject_performance_catalog_mutation()`); err != nil {
		t.Fatal(err)
	}
	failed, err := capability.ClaimRequest(t.Context(), data.Scope, time.Minute)
	if !errors.Is(err, requestintent.ErrUnavailable) || !reflect.DeepEqual(failed, requestintent.Claim{}) {
		t.Fatal("failed claim exposed success", err)
	}
	if _, err := store.pool.Exec(t.Context(), `DROP TRIGGER reject_test_claim ON argus_catalog.performance_dispatch_claims`); err != nil {
		t.Fatal(err)
	}
	recovered, err := capability.ClaimRequest(t.Context(), data.Scope, time.Minute)
	if err != nil || recovered.Pending.After != locked || recovered.Fence != 1 {
		t.Fatal("failed transaction leaked claim", err)
	}
}

func TestPerformanceClaimSchemaRejectsResetAndCrossScope(t *testing.T) {
	store, data := claimFixture(t, 0)
	claim, err := store.runtime.Performance().ClaimRequest(t.Context(), data.Scope, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`UPDATE argus_catalog.performance_dispatch_claims SET fence=0`,
		`UPDATE argus_catalog.performance_dispatch_claims SET fence=fence+1`,
		`UPDATE argus_catalog.performance_dispatch_claims SET principal_id='other'`,
		`DELETE FROM argus_catalog.performance_dispatch_claims`,
		`TRUNCATE argus_catalog.performance_dispatch_claims`,
	} {
		_, err := store.pool.Exec(t.Context(), statement)
		var failure *pgconn.PgError
		if !errors.As(err, &failure) || failure.Code != "55000" {
			t.Fatal("invalid claim mutation accepted", statement)
		}
	}
	_, err = store.pool.Exec(t.Context(), `INSERT INTO argus_catalog.performance_dispatch_claims SELECT position,'wrong',principal_issuer,principal_id,1,$1,clock_timestamp() FROM argus_catalog.performance_pending_dispatches WHERE position<>$2`, claim.Token, claim.Pending.After)
	var failure *pgconn.PgError
	if !errors.As(err, &failure) || failure.ConstraintName != "fk_performance_claim_scope" {
		t.Fatal("cross-scope claim not rejected by FK", err)
	}
	pending, err := store.runtime.Performance().PendingRequests(t.Context(), data.Scope, 0, 100)
	if err != nil || len(pending) != 2 || !reflect.DeepEqual(pending[0].Child, inwardRequest(t, data).Data().Children[0]) {
		t.Fatal("claims changed immutable evidence", err)
	}
}
