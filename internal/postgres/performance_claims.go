package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

// ClaimRequest atomically leases one eligible child in an independently authorized
// scope. Settled/locked children are skipped; no candidate returns ErrNotFound, including
// wrong scope. Lease is 1s..5m in whole microseconds. Expired request evidence remains
// claimable for readback recovery, never automatic resubmission. There is no I/O
// outside PostgreSQL in the transaction. A lost commit response leaves ownership
// uncertain: do not send; let the bounded lease expire and recover with frozen IDs.
// A concurrent settlement can return ErrConflict without acquiring ownership.
func (store *PerformanceStore) ClaimRequest(ctx context.Context, scope requestintent.Scope, lease time.Duration) (requestintent.Claim, error) {
	if !requestintent.ValidScope(scope) || lease < time.Second || lease > 5*time.Minute || lease%time.Microsecond != 0 {
		return requestintent.Claim{}, requestintent.ErrInvalid
	}
	tx, ctx, cancel, err := store.runtime.beginWrite(ctx)
	if err != nil {
		return requestintent.Claim{}, requestError(err)
	}
	defer cancel()
	defer func() { _ = tx.Rollback(ctx) }() //nolint:errcheck // Best effort after commit or failure.
	var claim requestintent.Claim
	child, err := scanDispatch(tx.QueryRow(ctx, `SELECT p.position,p.intent_uuid::text,`+dispatchColumns+`
        FROM argus_catalog.performance_pending_dispatches p
        LEFT JOIN argus_catalog.performance_dispatch_claims c ON c.position=p.position
        WHERE p.issuer=$1 AND p.principal_issuer=$2 AND p.principal_id=$3
        AND NOT EXISTS (SELECT 1 FROM argus_catalog.performance_dispatch_outcomes o WHERE o.position=p.position)
        AND (c.position IS NULL OR c.available_at<=clock_timestamp())
        ORDER BY p.position LIMIT 1 FOR UPDATE OF p SKIP LOCKED`, scope.Issuer, scope.PrincipalIssuer, scope.PrincipalID), &claim.Pending.After, &claim.Pending.IntentID)
	if err != nil {
		return requestintent.Claim{}, requestError(err)
	}
	claim.Pending.Child = child
	err = tx.QueryRow(ctx, `INSERT INTO argus_catalog.performance_dispatch_claims AS c
        (position,issuer,principal_issuer,principal_id,fence,token,available_at)
        VALUES ($1,$2,$3,$4,1,$5,clock_timestamp()+$6*INTERVAL '1 microsecond')
        ON CONFLICT (position) DO UPDATE SET fence=c.fence+1,token=EXCLUDED.token,available_at=EXCLUDED.available_at
        WHERE c.available_at<=clock_timestamp()
        RETURNING fence,token::text,available_at`, claim.Pending.After, scope.Issuer, scope.PrincipalIssuer, scope.PrincipalID, uuid.New(), lease.Microseconds()).Scan(&claim.Fence, &claim.Token, &claim.Until)
	if err != nil {
		return requestintent.Claim{}, requestError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return requestintent.Claim{}, requestError(err)
	}
	claim.Until = claim.Until.UTC()

	return claim, nil
}

// ReleaseRequest relinquishes only the current live fenced acquisition and sets
// retry availability using database time. Delay is 0..1h in whole microseconds.
// Wrong scope, expiry, already released and stale ownership all return ErrConflict.
// This is not completion/acknowledgement; immutable evidence and fence are retained.
// An uncertain response must not authorize another send or fabricate a new token.
func (store *PerformanceStore) ReleaseRequest(ctx context.Context, scope requestintent.Scope, claim requestintent.Claim, delay time.Duration) error {
	token, err := uuid.Parse(claim.Token)
	if !requestintent.ValidScope(scope) || claim.Pending.After <= 0 || claim.Fence <= 0 || err != nil || token == uuid.Nil || delay < 0 || delay > time.Hour || delay%time.Microsecond != 0 {
		return requestintent.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	result, err := store.runtime.pool.Exec(ctx, `UPDATE argus_catalog.performance_dispatch_claims
        SET token=NULL,available_at=clock_timestamp()+$7*INTERVAL '1 microsecond'
        WHERE issuer=$1 AND principal_issuer=$2 AND principal_id=$3 AND position=$4
        AND fence=$5 AND token=$6 AND available_at>clock_timestamp()
        AND NOT EXISTS (SELECT 1 FROM argus_catalog.performance_dispatch_outcomes o WHERE o.position=performance_dispatch_claims.position)`, scope.Issuer, scope.PrincipalIssuer, scope.PrincipalID, claim.Pending.After, claim.Fence, token, delay.Microseconds())
	if err != nil {
		return requestError(err)
	}
	if result.RowsAffected() != 1 {
		return requestintent.ErrConflict
	}

	return nil
}
