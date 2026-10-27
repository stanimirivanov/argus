package postgres

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

// AppendLifecycle atomically seeds any recovery anchor and appends one observation.
// Scope and admission must come from independent authorization/adapter validation;
// verify must be a bounded pure wire verifier, never perform I/O. Exact retry is
// read-only even at capacity. Calls serialize on pending work, then event keys.
// Lost commit acknowledgement requires exact retry or scoped readback, not resend.
func (store *PerformanceStore) AppendLifecycle(ctx context.Context, scope requestintent.Scope, position int64, admission requestintent.LifecycleAdmission, event requestintent.LifecycleEvent, verify requestintent.LifecycleVerifier) (bool, error) {
	if !validLifecycleRead(scope, position, admission, verify) || !verifiedLifecycle(event, verify) {
		return false, requestintent.ErrInvalid
	}
	tx, ctx, cancel, err := store.runtime.beginWrite(ctx)
	if err != nil {
		return false, requestError(err)
	}
	defer cancel()
	defer func() { _ = tx.Rollback(ctx) }() //nolint:errcheck // Best effort after commit or failure.
	child, err := lifecycleContext(ctx, tx, scope, position, admission, true)
	if err != nil {
		return false, err
	}
	if !matchesLifecycle(admission, child, event) {
		return false, requestintent.ErrConflict
	}
	history, err := lifecycleRows(ctx, tx, scope, position, admission, child, verify)
	if err != nil {
		return false, err
	}
	additions, err := lifecycleAdditions(history, admission.Data().Initial, event)
	if err != nil || len(additions) == 0 {
		return false, err
	}
	ids := make([]string, len(additions))
	for i, addition := range additions {
		if !verifiedLifecycle(addition, verify) {
			return false, requestintent.ErrInvalid
		}
		ids[i] = addition.Data().Evidence.Data().EventID
	}
	// Acquire the exact trigger keys in one global order before anchor-first writes.
	_, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(key) FROM (
        SELECT DISTINCT hashtextextended($1||encode(sha256(convert_to(id,'UTF8')),'hex'),938071) AS key
        FROM unnest($2::text[]) id ORDER BY key) keys`, scope.Issuer, ids)
	if err != nil {
		return false, requestError(err)
	}
	for _, addition := range additions {
		d, evidence := addition.Data(), addition.Data().Evidence.Data()
		_, err := tx.Exec(ctx, `INSERT INTO argus_catalog.performance_lifecycle_events
            (position,issuer,principal_issuer,principal_id,admission_fingerprint,request_sha256,run_id,event_id,fingerprint,revision,state,occurred_at,payload)
            VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, position, scope.Issuer, scope.PrincipalIssuer, scope.PrincipalID,
			admission.Data().Evidence.Fingerprint(), d.RequestSHA256, d.Run.ID, evidence.EventID, d.Evidence.Fingerprint(), d.Revision, d.State, d.OccurredAt, evidence.Payload)
		if err != nil {
			return false, requestError(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return false, requestError(err)
	}

	return true, nil
}

func lifecycleContext(ctx context.Context, tx pgx.Tx, scope requestintent.Scope, position int64, admission requestintent.LifecycleAdmission, lock bool) (requestintent.Child, error) {
	query := `SELECT o.kind,o.event_id,o.payload,o.fingerprint,` + dispatchColumns + `
        FROM argus_catalog.performance_pending_dispatches p JOIN argus_catalog.performance_dispatch_outcomes o ON o.position=p.position
        WHERE p.position=$1 AND p.issuer=$2 AND p.principal_issuer=$3 AND p.principal_id=$4`
	if lock {
		query += ` FOR UPDATE OF p`
	}
	data := requestintent.OutcomeData{Source: scope.Issuer}
	var fingerprint string
	child, err := scanDispatch(tx.QueryRow(ctx, query, position, scope.Issuer, scope.PrincipalIssuer, scope.PrincipalID), &data.Kind, &data.EventID, &data.Payload, &fingerprint)
	if err != nil {
		return requestintent.Child{}, requestError(err)
	}
	data.Request, data.Attempt = child.Request, child.Attempt
	retained, err := requestintent.NewOutcome(data)
	if err != nil || retained.Fingerprint() != fingerprint {
		return requestintent.Child{}, requestintent.ErrUnavailable
	}
	b := admission.Data()
	if fingerprint != b.Evidence.Fingerprint() || child.SHA256 != b.RequestSHA256 {
		return requestintent.Child{}, requestintent.ErrConflict
	}

	return child, nil
}

func lifecycleAdditions(history []requestintent.LifecycleEvent, initial, event requestintent.LifecycleEvent) ([]requestintent.LifecycleEvent, error) {
	var additions []requestintent.LifecycleEvent
	if len(history) == 0 && initial.Data().Evidence.Fingerprint() != "" {
		additions = append(additions, initial)
	}
	combined := append(slices.Clone(history), additions...)
	eventID := event.Data().Evidence.Data().EventID
	for _, prior := range combined {
		if prior.Data().Evidence.Data().EventID == eventID {
			if !prior.Equal(event) {
				return nil, requestintent.ErrConflict
			}

			return additions, nil
		}
	}
	if _, err := requestintent.ProjectLifecycle(append(combined, event)); err != nil {
		return nil, requestintent.ErrConflict
	}

	return append(additions, event), nil
}

func validLifecycleRead(scope requestintent.Scope, position int64, admission requestintent.LifecycleAdmission, verify requestintent.LifecycleVerifier) bool {
	if !requestintent.ValidScope(scope) || position <= 0 || admission.Data().Evidence.Fingerprint() == "" || verify == nil {
		return false
	}
	checked, err := verify(admission.Data().Evidence)
	a, b := admission.Data(), checked.Data()

	return err == nil && a.Evidence.Fingerprint() == b.Evidence.Fingerprint() && a.Run == b.Run && a.RequestSHA256 == b.RequestSHA256 && a.Initial.Equal(b.Initial)
}

func verifiedLifecycle(event requestintent.LifecycleEvent, verify requestintent.LifecycleVerifier) bool {
	if event.Data().Evidence.Fingerprint() == "" {
		return false
	}
	checked, err := verify(event.Data().Evidence)

	return err == nil && event.Equal(checked.Data().Initial)
}

func matchesLifecycle(admission requestintent.LifecycleAdmission, child requestintent.Child, event requestintent.LifecycleEvent) bool {
	b, d := admission.Data(), event.Data()
	evidence := d.Evidence.Data()
	return b.Run == d.Run && child.SHA256 == d.RequestSHA256 && child.Deadline.Equal(d.Deadline) && !d.OccurredAt.Before(child.RequestedAt) && evidence.Request == child.Request && evidence.Attempt == child.Attempt && evidence.Source == b.Evidence.Data().Source
}
