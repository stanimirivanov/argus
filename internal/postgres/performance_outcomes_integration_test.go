//go:build integration

package postgres

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

func outcomeFixture(t *testing.T, scope requestintent.Scope, claim requestintent.Claim, kind string) requestintent.Outcome {
	t.Helper()
	eventID := strings.Repeat("\U0001f600", 256-len(claim.Pending.Child.EventID)) + claim.Pending.Child.EventID
	outcome, err := requestintent.NewOutcome(requestintent.OutcomeData{Request: claim.Pending.Child.Request, Attempt: claim.Pending.Child.Attempt, Source: scope.Issuer, EventID: eventID, Kind: kind, Payload: []byte(`{"validated":"provider"}`)})
	if err != nil {
		t.Fatal(err)
	}

	return outcome
}

func rejectSettlement(t *testing.T, store *PerformanceStore, scope requestintent.Scope, claim requestintent.Claim, outcome requestintent.Outcome, expected error) {
	t.Helper()
	if created, err := store.SettleRequest(t.Context(), scope, claim, outcome); created || !errors.Is(err, expected) {
		t.Fatal("invalid settlement result", created, err)
	}
}

func waitForClaimBlock(t *testing.T, store *integratedStore, blocker int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	for {
		var waiting bool
		if err := store.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid)))`, blocker).Scan(&waiting); err != nil {
			t.Fatal("settlement did not reach claim lock", err)
		}
		if waiting {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestPerformanceOutcomeConcurrentReplayAndScopedRead(t *testing.T) {
	store, data := claimFixture(t, 0)
	capability := store.runtime.Performance()
	claim, err := capability.ClaimRequest(t.Context(), data.Scope, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	outcome := outcomeFixture(t, data.Scope, claim, "accepted")
	var writes atomic.Int32
	var workers sync.WaitGroup
	for range 2 {
		workers.Go(func() {
			created, err := capability.SettleRequest(t.Context(), data.Scope, claim, outcome)
			if err != nil {
				t.Error(err)
			}
			if created {
				writes.Add(1)
			}
		})
	}
	workers.Wait()
	if writes.Load() != 1 {
		t.Fatal("settlement was not atomic", writes.Load())
	}
	if _, err := store.pool.Exec(t.Context(), `SELECT pg_sleep(GREATEST(0,EXTRACT(EPOCH FROM ($1::timestamptz-clock_timestamp())))+0.01)`, claim.Until); err != nil {
		t.Fatal(err)
	}
	if created, err := capability.SettleRequest(t.Context(), data.Scope, claim, outcome); err != nil || created {
		t.Fatal("lost-response replay after expiry", err)
	}
	url := store.pool.Config().ConnString()
	store.Close()
	reopened := openTestStore(t, url)
	capability = reopened.runtime.Performance()
	got, err := capability.RequestOutcome(t.Context(), data.Scope, claim.Pending.After)
	if err != nil || got.Fingerprint() != outcome.Fingerprint() {
		t.Fatal("reopen lost evidence", err)
	}
	for _, wrong := range []requestintent.Scope{
		{Issuer: "other", PrincipalIssuer: data.Scope.PrincipalIssuer, PrincipalID: data.Scope.PrincipalID},
		{Issuer: data.Scope.Issuer, PrincipalIssuer: "other", PrincipalID: data.Scope.PrincipalID},
		{Issuer: data.Scope.Issuer, PrincipalIssuer: data.Scope.PrincipalIssuer, PrincipalID: "other"},
	} {
		if _, err := capability.RequestOutcome(t.Context(), wrong, claim.Pending.After); !errors.Is(err, requestintent.ErrNotFound) {
			t.Fatal("scope leaked", err)
		}
		rejectSettlement(t, capability, wrong, claim, outcomeFixture(t, wrong, claim, "accepted"), requestintent.ErrConflict)
	}
	for _, stale := range []requestintent.Claim{
		{Pending: claim.Pending, Token: claim.Token, Fence: claim.Fence + 1},
		{Pending: claim.Pending, Token: "00000000-0000-0000-0000-000000000001", Fence: claim.Fence},
	} {
		rejectSettlement(t, capability, data.Scope, stale, outcome, requestintent.ErrConflict)
	}
	rejectSettlement(t, capability, data.Scope, claim, outcomeFixture(t, data.Scope, claim, "rejected"), requestintent.ErrConflict)
	spoofed := claim
	spoofed.Pending.Child.Request.ID += "-other"
	rejectSettlement(t, capability, data.Scope, claim, outcomeFixture(t, data.Scope, spoofed, "accepted"), requestintent.ErrConflict)
	_, err = reopened.pool.Exec(t.Context(), `UPDATE argus_catalog.performance_dispatch_claims SET fence=fence+1,available_at=clock_timestamp()+INTERVAL '1 minute' WHERE position=$1`, claim.Pending.After)
	var failure *pgconn.PgError
	if !errors.As(err, &failure) || failure.ConstraintName != "ck_performance_outcome_ownership" {
		t.Fatal("expired settled claim reopened", err)
	}
}

func TestPerformanceOutcomesDrainClaimsWithoutChangingInspection(t *testing.T) {
	store, data := claimFixture(t, 0)
	capability := store.runtime.Performance()
	claim, err := capability.ClaimRequest(t.Context(), data.Scope, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	outcome := outcomeFixture(t, data.Scope, claim, "accepted")
	if created, err := capability.SettleRequest(t.Context(), data.Scope, claim, outcome); err != nil || !created {
		t.Fatal("settlement failed", err)
	}
	rejectPerformanceRelease(t, capability, data.Scope, claim)
	second, err := capability.ClaimRequest(t.Context(), data.Scope, time.Minute)
	if err != nil || second.Pending.After == claim.Pending.After {
		t.Fatal("settled child reclaimed", err)
	}
	duplicate := outcomeFixture(t, data.Scope, second, "rejected").Data()
	duplicate.EventID = outcome.Data().EventID
	collision, err := requestintent.NewOutcome(duplicate)
	if err != nil {
		t.Fatal(err)
	}
	rejectSettlement(t, capability, data.Scope, second, collision, requestintent.ErrConflict)
	if _, err := capability.SettleRequest(t.Context(), data.Scope, second, outcomeFixture(t, data.Scope, second, "rejected")); err != nil {
		t.Fatal(err)
	}
	if _, err := capability.ClaimRequest(t.Context(), data.Scope, time.Minute); !errors.Is(err, requestintent.ErrNotFound) {
		t.Fatal("completed queue not empty", err)
	}
	if pending, err := capability.PendingRequests(t.Context(), data.Scope, 0, 100); err != nil || len(pending) != 2 {
		t.Fatal("immutable inspection changed", err)
	}
	if _, err := store.pool.Exec(t.Context(), `ALTER TABLE argus_catalog.performance_dispatch_outcomes DISABLE TRIGGER trg_performance_outcome_immutable; UPDATE argus_catalog.performance_dispatch_outcomes SET payload=convert_to('{"changed":true}','UTF8')`); err != nil {
		t.Fatal(err)
	}
	if got, err := capability.RequestOutcome(t.Context(), data.Scope, claim.Pending.After); !errors.Is(err, requestintent.ErrUnavailable) || got.Fingerprint() != "" {
		t.Fatal("corruption was not redacted", err)
	}
	rejectSettlement(t, capability, data.Scope, claim, outcome, requestintent.ErrConflict)
}

func TestPerformanceOutcomeRollbackAndDatabaseGuards(t *testing.T) {
	store, data := claimFixture(t, 0)
	capability := store.runtime.Performance()
	claim, err := capability.ClaimRequest(t.Context(), data.Scope, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	outcome := outcomeFixture(t, data.Scope, claim, "lifecycle")
	for _, probe := range []struct {
		stale            bool
		kind, constraint string
	}{
		{true, "lifecycle", "ck_performance_outcome_ownership"},
		{false, "unknown", "ck_performance_outcome_bounds"},
	} {
		_, err = store.pool.Exec(t.Context(), `INSERT INTO argus_catalog.performance_dispatch_outcomes
            (position,issuer,principal_issuer,principal_id,fence,token,kind,event_id,payload,fingerprint,recorded_at)
            SELECT position,issuer,principal_issuer,principal_id,fence+CASE WHEN $3 THEN 1 ELSE 0 END,token,$4,'forged','{}',$2,clock_timestamp()
            FROM argus_catalog.performance_dispatch_claims WHERE position=$1`, claim.Pending.After, outcome.Fingerprint(), probe.stale, probe.kind)
		var failure *pgconn.PgError
		if !errors.As(err, &failure) || failure.ConstraintName != probe.constraint {
			t.Fatal("SQL bypassed outcome invariant", err)
		}
	}
	if _, err := store.pool.Exec(t.Context(), `CREATE TRIGGER reject_test_outcome AFTER INSERT ON argus_catalog.performance_dispatch_outcomes FOR EACH STATEMENT EXECUTE FUNCTION argus_catalog.reject_performance_catalog_mutation()`); err != nil {
		t.Fatal(err)
	}
	if created, err := capability.SettleRequest(t.Context(), data.Scope, claim, outcome); !errors.Is(err, requestintent.ErrUnavailable) || created {
		t.Fatal("failed write exposed success", err)
	}
	if _, err := capability.RequestOutcome(t.Context(), data.Scope, claim.Pending.After); !errors.Is(err, requestintent.ErrNotFound) {
		t.Fatal("rollback leaked", err)
	}
	if _, err := store.pool.Exec(t.Context(), `DROP TRIGGER reject_test_outcome ON argus_catalog.performance_dispatch_outcomes`); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := capability.SettleRequest(ctx, data.Scope, claim, outcome); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost", err)
	}
	if created, err := capability.SettleRequest(t.Context(), data.Scope, claim, outcome); err != nil || !created {
		t.Fatal("recovery failed", err)
	}
	for _, statement := range []string{
		`UPDATE argus_catalog.performance_dispatch_outcomes SET kind='rejected'`,
		`DELETE FROM argus_catalog.performance_dispatch_outcomes`,
		`TRUNCATE argus_catalog.performance_dispatch_outcomes`,
		`UPDATE argus_catalog.performance_dispatch_claims SET token=NULL,available_at=clock_timestamp()`,
	} {
		_, err := store.pool.Exec(t.Context(), statement)
		var failure *pgconn.PgError
		if !errors.As(err, &failure) || failure.Code != "55000" {
			t.Fatal("SQL settlement mutation accepted", err)
		}
	}
}

func TestPerformanceOutcomeRechecksLeaseAfterWaitingForRowLock(t *testing.T) {
	store, data := claimFixture(t, 0)
	claim, err := store.runtime.Performance().ClaimRequest(t.Context(), data.Scope, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := store.pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(t.Context()) }() //nolint:errcheck // Best effort after explicit rollback.
	if _, err := tx.Exec(t.Context(), `SELECT 1 FROM argus_catalog.performance_dispatch_claims WHERE position=$1 FOR UPDATE`, claim.Pending.After); err != nil {
		t.Fatal(err)
	}
	var blocker int
	if err := tx.QueryRow(t.Context(), `SELECT pg_backend_pid()`).Scan(&blocker); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	outcome := outcomeFixture(t, data.Scope, claim, "accepted")
	go func() {
		_, err := store.runtime.Performance().SettleRequest(t.Context(), data.Scope, claim, outcome)
		result <- err
	}()
	waitForClaimBlock(t, store, blocker)
	if _, err := tx.Exec(t.Context(), `SELECT pg_sleep(GREATEST(0,EXTRACT(EPOCH FROM ($1::timestamptz-clock_timestamp())))+0.01)`, claim.Until); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := <-result; !errors.Is(err, requestintent.ErrConflict) {
		t.Fatal("expired owner settled", err)
	}
	reclaimed, err := store.runtime.Performance().ClaimRequest(t.Context(), data.Scope, time.Minute)
	if err != nil || reclaimed.Fence <= claim.Fence {
		t.Fatal("reclaim lost fence", err)
	}
	if _, err := store.runtime.Performance().SettleRequest(t.Context(), data.Scope, claim, outcomeFixture(t, data.Scope, claim, "accepted")); !errors.Is(err, requestintent.ErrConflict) {
		t.Fatal("stale owner settled", err)
	}
}
