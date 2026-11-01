package postgres

import (
	"context"

	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

// NextAnalysis returns at most one admitted child after an exclusive database-local
// cursor under independently authorized provider/principal and request/attempt issuer.
// Missing/end/wrong scope is ErrNotFound. Zero restarts traversal; callers own fair
// rotation and retry scheduling. Calls observe current committed state, not a snapshot
// spanning the traversal: late admission behind the cursor requires a later reset.
// Terminal, cancelled, cancellation-deferred and already analyzed requests remain
// eligible for late/conflicting evidence. This read grants no lease, claim, report
// access or submission authority; consumers must revalidate retained admission.
func (store *PerformanceStore) NextAnalysis(ctx context.Context, scope requestintent.Scope, issuer string, after int64) (requestintent.Pending, error) {
	if !requestintent.ValidScope(scope) || !requestintent.ValidRequestIssuer(issuer) || after < 0 {
		return requestintent.Pending{}, requestintent.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return requestintent.Pending{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	var pending requestintent.Pending
	row := store.runtime.pool.QueryRow(ctx, nextAnalysisQuery, scope.Issuer, scope.PrincipalIssuer, scope.PrincipalID, issuer, after)
	child, err := scanDispatch(row, &pending.After, &pending.IntentID)
	if err != nil {
		return requestintent.Pending{}, requestError(err)
	}
	if err := ctx.Err(); err != nil {
		return requestintent.Pending{}, err
	}
	pending.Child = child

	return pending, nil
}

const nextAnalysisQuery = `SELECT position,intent_uuid::text,` + dispatchColumns + `
        FROM argus_catalog.performance_pending_dispatches p
        WHERE issuer=$1 AND principal_issuer=$2 AND principal_id=$3 AND request_issuer=$4 AND attempt_issuer=$4 AND position>$5
        AND EXISTS (SELECT 1 FROM argus_catalog.performance_dispatch_outcomes o WHERE o.position=p.position AND o.kind IN ('accepted','lifecycle'))
        ORDER BY position LIMIT 1`
