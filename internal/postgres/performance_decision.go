package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

type decisionQuery interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

// DecisionSnapshot reads intent, authoritative admission, lifecycle and analysis
// in one bounded read-only repeatable-read transaction. Scope is independently
// authorized by the caller. Pure verifiers must perform no I/O. Every failure,
// including corruption or commit failure, returns zero data, never partial history.
func (store *PerformanceStore) DecisionSnapshot(ctx context.Context, scope requestintent.Scope, position int64, admit requestintent.AdmissionVerifier, verify requestintent.AnalysisVerifier, project requestintent.AnalysisProjector) (requestintent.DecisionSnapshot, error) {
	zero := requestintent.DecisionSnapshot{}
	if !requestintent.ValidScope(scope) || position <= 0 || admit == nil || verify == nil || project == nil {
		return zero, requestintent.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	tx, err := store.runtime.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return zero, requestError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }() //nolint:errcheck // Best effort after commit or failure.
	var review string
	var ordinal int
	err = tx.QueryRow(ctx, `SELECT i.review_sha256,p.ordinal FROM argus_catalog.performance_pending_dispatches p
 JOIN argus_catalog.performance_request_intents i ON i.intent_uuid=p.intent_uuid
 WHERE p.position=$1 AND p.issuer=$2 AND p.principal_issuer=$3 AND p.principal_id=$4`, position, scope.Issuer, scope.PrincipalIssuer, scope.PrincipalID).Scan(&review, &ordinal)
	if err != nil {
		return zero, requestError(err)
	}
	intent, err := readRequest(ctx, tx, scope, review)
	if err != nil {
		return zero, err
	}
	children := intent.Data().Children
	if ordinal < 0 || ordinal >= len(children) {
		return zero, requestintent.ErrUnavailable
	}
	child := children[ordinal]
	outcome, err := readOutcome(ctx, tx, scope, position)
	if err != nil {
		return zero, err
	}
	admission, err := admit(child, outcome)
	if err != nil {
		return zero, requestintent.ErrUnavailable
	}
	lifecycleVerify := func(outcome requestintent.Outcome) (requestintent.LifecycleAdmission, error) {
		return admit(child, outcome)
	}
	if !validLifecycleRead(scope, position, admission, lifecycleVerify) {
		return zero, requestintent.ErrUnavailable
	}
	stored, err := lifecycleContext(ctx, tx, scope, position, admission, false)
	if err != nil {
		return zero, err
	}
	if stored.SHA256 != child.SHA256 {
		return zero, requestintent.ErrUnavailable
	}
	history, err := lifecycleRows(ctx, tx, scope, position, admission, child, lifecycleVerify)
	if err != nil {
		return zero, err
	}
	latest := admission.Data().Initial
	if len(history) > 0 {
		latest, err = requestintent.ProjectLifecycle(history)
		if err != nil {
			return zero, requestintent.ErrUnavailable
		}
	}
	analysis, err := readAnalysisProjection(ctx, tx, scope, position, admission, verify, project)
	if err != nil {
		return zero, err
	}
	if err := tx.Commit(ctx); err != nil {
		return zero, requestError(err)
	}

	return requestintent.DecisionSnapshot{Intent: intent, Child: child, Admission: admission, Lifecycle: latest, Analysis: analysis}, nil
}
