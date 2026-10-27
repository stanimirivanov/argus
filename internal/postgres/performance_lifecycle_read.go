package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/stanimirivanov/argus/internal/performance"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

// LifecycleHistory returns owned revision/event-ID ordered evidence under one
// read-only repeatable-read snapshot. Unknown/wrong scope is ErrNotFound; known
// admission with no history returns an empty slice. Corruption is ErrUnavailable,
// never partial output. The supplied pure verifier must revalidate retained bytes.
func (store *PerformanceStore) LifecycleHistory(ctx context.Context, scope requestintent.Scope, position int64, admission requestintent.LifecycleAdmission, verify requestintent.LifecycleVerifier) ([]requestintent.LifecycleEvent, error) {
	if !validLifecycleRead(scope, position, admission, verify) {
		return nil, requestintent.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	tx, err := store.runtime.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, requestError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }() //nolint:errcheck // Best effort after commit or failure.
	child, err := lifecycleContext(ctx, tx, scope, position, admission, false)
	if err != nil {
		return nil, err
	}
	history, err := lifecycleRows(ctx, tx, scope, position, admission, child, verify)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, requestError(err)
	}

	return history, nil
}

// LifecycleProjection returns the validated latest provider revision, not an
// analysis verdict. Empty history is ErrNotFound; deadlines never imply terminal state.
func (store *PerformanceStore) LifecycleProjection(ctx context.Context, scope requestintent.Scope, position int64, admission requestintent.LifecycleAdmission, verify requestintent.LifecycleVerifier) (requestintent.LifecycleEvent, error) {
	history, err := store.LifecycleHistory(ctx, scope, position, admission, verify)
	if err != nil {
		return requestintent.LifecycleEvent{}, err
	}
	if len(history) == 0 {
		return requestintent.LifecycleEvent{}, requestintent.ErrNotFound
	}

	return requestintent.ProjectLifecycle(history)
}

func lifecycleRows(ctx context.Context, tx pgx.Tx, scope requestintent.Scope, position int64, admission requestintent.LifecycleAdmission, child requestintent.Child, verify requestintent.LifecycleVerifier) ([]requestintent.LifecycleEvent, error) {
	rows, err := tx.Query(ctx, `SELECT event_id,payload,fingerprint,admission_fingerprint,request_sha256,run_id,revision,state,occurred_at
        FROM argus_catalog.performance_lifecycle_events WHERE position=$1 AND issuer=$2 AND principal_issuer=$3 AND principal_id=$4
        ORDER BY revision,event_id COLLATE "C" LIMIT 1001`, position, scope.Issuer, scope.PrincipalIssuer, scope.PrincipalID)
	if err != nil {
		return nil, requestError(err)
	}
	defer rows.Close()
	history := make([]requestintent.LifecycleEvent, 0)
	anchor := admission.Data().Initial.Data().Evidence.Fingerprint() == ""
	for rows.Next() {
		d := requestintent.LifecycleData{Run: performance.Identity{Issuer: scope.Issuer}, Deadline: child.Deadline}
		evidence := requestintent.OutcomeData{Source: scope.Issuer, Request: child.Request, Attempt: child.Attempt, Kind: "lifecycle"}
		var fingerprint, admissionFingerprint string
		if err := rows.Scan(&evidence.EventID, &evidence.Payload, &fingerprint, &admissionFingerprint, &d.RequestSHA256, &d.Run.ID, &d.Revision, &d.State, &d.OccurredAt); err != nil {
			return nil, requestError(err)
		}
		d.Evidence, err = requestintent.NewOutcome(evidence)
		if err != nil || d.Evidence.Fingerprint() != fingerprint || admissionFingerprint != admission.Data().Evidence.Fingerprint() {
			return nil, requestintent.ErrUnavailable
		}
		event, err := requestintent.NewLifecycleEvent(d)
		if err != nil || !matchesLifecycle(admission, child, event) || !verifiedLifecycle(event, verify) {
			return nil, requestintent.ErrUnavailable
		}
		anchor = anchor || event.Equal(admission.Data().Initial)
		history = append(history, event)
	}
	if err := rows.Err(); err != nil {
		return nil, requestError(err)
	}
	if len(history) > 0 {
		if _, err := requestintent.ProjectLifecycle(history); err != nil || !anchor {
			return nil, requestintent.ErrUnavailable
		}
	}

	return history, nil
}
