package postgres

import (
	"context"
	"time"

	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

// NextCancellation reads one due admitted cancellation after an exclusive cursor.
// Trusted provider/principal and request/attempt issuers must match. Missing/end or
// wrong scope is ErrNotFound. Terminal history remains eligible for revalidation.
// This is not a lease: already selected work is not revoked by later postponement.
// Callers own fair cursor rotation, scheduling and canonical intent/history checks.
func (store *PerformanceStore) NextCancellation(ctx context.Context, scope requestintent.Scope, issuer string, after int64) (requestintent.Pending, error) {
	if !requestintent.ValidScope(scope) || !requestintent.ValidRequestIssuer(issuer) || after < 0 {
		return requestintent.Pending{}, requestintent.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	var pending requestintent.Pending
	row := store.runtime.pool.QueryRow(ctx, `SELECT position,intent_uuid::text,`+dispatchColumns+`
        FROM argus_catalog.performance_pending_dispatches p
        WHERE issuer=$1 AND principal_issuer=$2 AND principal_id=$3 AND request_issuer=$4 AND attempt_issuer=$4 AND position>$5
        AND EXISTS (SELECT 1 FROM argus_catalog.performance_cancellations x WHERE x.position=p.position)
        AND EXISTS (SELECT 1 FROM argus_catalog.performance_dispatch_outcomes o WHERE o.position=p.position AND o.kind IN ('accepted','lifecycle'))
        AND NOT EXISTS (SELECT 1 FROM argus_catalog.performance_cancellation_delays d WHERE d.position=p.position AND d.not_before>clock_timestamp())
        ORDER BY position LIMIT 1`, scope.Issuer, scope.PrincipalIssuer, scope.PrincipalID, issuer, after)
	child, err := scanDispatch(row, &pending.After, &pending.IntentID)
	if err != nil {
		return pending, requestError(err)
	}
	pending.Child = child

	return pending, nil
}

// DeferCancellation durably postpones admitted cancellation polling using database
// time. Delay is positive whole seconds, including representable RetryAfter hints.
// Concurrent/shorter updates never shorten the existing deadline. Returned UTC time
// is not a claim or permission for I/O. Lost acknowledgement permits conservative
// repeat deferral, never assumed scheduling success. No network access occurs here.
func (store *PerformanceStore) DeferCancellation(ctx context.Context, scope requestintent.Scope, issuer string, position int64, delay time.Duration) (time.Time, error) {
	if !requestintent.ValidScope(scope) || !requestintent.ValidRequestIssuer(issuer) || position <= 0 || delay < time.Second || delay%time.Second != 0 {
		return time.Time{}, requestintent.ErrInvalid
	}
	tx, ctx, cancel, err := store.runtime.beginWrite(ctx)
	if err != nil {
		return time.Time{}, requestError(err)
	}
	defer cancel()
	defer func() { _ = tx.Rollback(ctx) }() //nolint:errcheck // Best effort after commit or failure.
	var until time.Time
	err = tx.QueryRow(ctx, `INSERT INTO argus_catalog.performance_cancellation_delays AS d
        (position,issuer,principal_issuer,principal_id,not_before)
        SELECT p.position,p.issuer,p.principal_issuer,p.principal_id,clock_timestamp()+$6::BIGINT*INTERVAL '1 second'
        FROM argus_catalog.performance_pending_dispatches p
        WHERE p.position=$1 AND p.issuer=$2 AND p.principal_issuer=$3 AND p.principal_id=$4 AND p.request_issuer=$5 AND p.attempt_issuer=$5
        AND EXISTS (SELECT 1 FROM argus_catalog.performance_cancellations x WHERE x.position=p.position)
        AND EXISTS (SELECT 1 FROM argus_catalog.performance_dispatch_outcomes o WHERE o.position=p.position AND o.kind IN ('accepted','lifecycle'))
        ON CONFLICT (position) DO UPDATE SET not_before=GREATEST(d.not_before,EXCLUDED.not_before)
        RETURNING not_before`, position, scope.Issuer, scope.PrincipalIssuer, scope.PrincipalID, issuer, int64(delay/time.Second)).Scan(&until)
	if err != nil {
		return time.Time{}, requestError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return time.Time{}, requestError(err)
	}

	return until.UTC(), nil
}
