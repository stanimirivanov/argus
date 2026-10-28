package postgres

import (
	"context"

	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

// NextLifecycle returns one settled accepted/lifecycle child after a cursor under
// independently authorized scope and request issuer. Missing/end is ErrNotFound.
// Immutable settlement makes traversal restartable; this read grants no claim or
// submission rights. Callers rotate/reset cursors and validate history separately.
func (store *PerformanceStore) NextLifecycle(ctx context.Context, scope requestintent.Scope, issuer string, after int64) (requestintent.Pending, error) {
	if !requestintent.ValidScope(scope) || !requestintent.ValidRequestIssuer(issuer) || after < 0 {
		return requestintent.Pending{}, requestintent.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	var pending requestintent.Pending
	// Keep child columns unambiguous: the settlement is an existence predicate.
	row := store.runtime.pool.QueryRow(ctx, `SELECT position,intent_uuid::text,`+dispatchColumns+`
        FROM argus_catalog.performance_pending_dispatches p
        WHERE issuer=$1 AND principal_issuer=$2 AND principal_id=$3 AND request_issuer=$4 AND attempt_issuer=$4 AND position>$5
        AND EXISTS (SELECT 1 FROM argus_catalog.performance_dispatch_outcomes o WHERE o.position=p.position AND o.kind IN ('accepted','lifecycle'))
        ORDER BY position LIMIT 1`, scope.Issuer, scope.PrincipalIssuer, scope.PrincipalID, issuer, after)
	child, err := scanDispatch(row, &pending.After, &pending.IntentID)
	if err != nil {
		return pending, requestError(err)
	}
	pending.Child = child

	return pending, nil
}
