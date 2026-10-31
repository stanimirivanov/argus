package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

// AnalysisProjection traverses at most 1000 scoped events in one read-only
// repeatable-read snapshot. It holds one raw bundle at a time and bounds compact
// output to 8 MiB. verify/project must be pure, bounded, context-aware and perform
// no I/O or publication of intermediate results. Unknown scope is ErrNotFound;
// known empty history is an empty projection. Any failure returns zero output.
// No winner, aggregate verdict, approval or lifecycle transition is inferred.
func (store *PerformanceStore) AnalysisProjection(ctx context.Context, scope requestintent.Scope, position int64, admission requestintent.LifecycleAdmission, verify requestintent.AnalysisVerifier, project requestintent.AnalysisProjector) (requestintent.AnalysisProjection, error) {
	if !validAnalysisRead(scope, position, admission, verify) || project == nil {
		return requestintent.AnalysisProjection{}, requestintent.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	tx, err := store.runtime.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return requestintent.AnalysisProjection{}, requestError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }() //nolint:errcheck // Best effort after commit or failure.
	child, err := lifecycleContext(ctx, tx, scope, position, admission, false)
	if err != nil {
		return requestintent.AnalysisProjection{}, err
	}
	rows, err := tx.Query(ctx, `SELECT event_id FROM argus_catalog.performance_analysis_events
 WHERE position=$1 AND issuer=$2 AND principal_issuer=$3 AND principal_id=$4 ORDER BY event_id COLLATE "C" LIMIT 1001`, position, scope.Issuer, scope.PrincipalIssuer, scope.PrincipalID)
	if err != nil {
		return requestintent.AnalysisProjection{}, requestError(err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return requestintent.AnalysisProjection{}, requestError(err)
	}
	if len(ids) > 1000 {
		return requestintent.AnalysisProjection{}, requestintent.ErrUnavailable
	}
	var collector requestintent.AnalysisCollector
	for _, id := range ids {
		event, err := analysisRow(ctx, tx, scope, position, admission, child, id, verify)
		if err != nil {
			return requestintent.AnalysisProjection{}, err
		}
		decision, err := project(ctx, event)
		data := event.Data()
		if err != nil || decision.EventID != id || decision.EvidenceFingerprint != event.Fingerprint() || decision.ReportSHA256 != event.ReportSHA256() || !decision.PublishedAt.Equal(data.PublishedAt) || decision.Late != !data.PublishedAt.Before(child.Deadline) {
			return requestintent.AnalysisProjection{}, analysisStoreError(ctx, requestintent.ErrUnavailable)
		}
		if err := collector.Add(decision); err != nil {
			return requestintent.AnalysisProjection{}, requestintent.ErrUnavailable
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return requestintent.AnalysisProjection{}, requestError(err)
	}

	return collector.Result(), nil
}
