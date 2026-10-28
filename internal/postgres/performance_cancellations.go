package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

// RequestCancellation atomically retains intent and cuts off future first claims.
// Callers independently authorize scope/approval; verify is pure and performs no I/O.
// Exact replay is read-only; changed intent conflicts. Previously acquired claims
// remain uncertain and recoverable, never revoked or declared terminated here.
// Lost commit acknowledgement requires exact replay/scoped readback, never resend.
func (store *PerformanceStore) RequestCancellation(ctx context.Context, scope requestintent.Scope, position int64, intent requestintent.Cancellation, verify requestintent.CancellationVerifier) (bool, error) {
	if !requestintent.ValidScope(scope) || position <= 0 || intent.Fingerprint() == "" || verify == nil {
		return false, requestintent.ErrInvalid
	}
	tx, ctx, cancel, err := store.runtime.beginWrite(ctx)
	if err != nil {
		return false, requestError(err)
	}
	defer cancel()
	defer func() { _ = tx.Rollback(ctx) }() //nolint:errcheck // Best effort after commit or failure.
	child, err := scanDispatch(tx.QueryRow(ctx, `SELECT `+dispatchColumns+` FROM argus_catalog.performance_pending_dispatches
        WHERE position=$1 AND issuer=$2 AND principal_issuer=$3 AND principal_id=$4 FOR UPDATE`, position, scope.Issuer, scope.PrincipalIssuer, scope.PrincipalID))
	if err != nil {
		return false, requestError(err)
	}
	if child.SHA256 != intent.Data().RequestSHA256 || !verifiedCancellation(child.Payload, intent, verify) {
		return false, requestintent.ErrInvalid
	}
	retained, err := scanCancellation(tx.QueryRow(ctx, `SELECT event_id,request_sha256,payload,fingerprint FROM argus_catalog.performance_cancellations WHERE position=$1`, position))
	if err == nil {
		if !verifiedCancellation(child.Payload, retained, verify) {
			return false, requestintent.ErrUnavailable
		}
		if retained.Fingerprint() != intent.Fingerprint() {
			return false, requestintent.ErrConflict
		}

		return false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, requestError(err)
	}
	d := intent.Data()
	_, err = tx.Exec(ctx, `INSERT INTO argus_catalog.performance_cancellations
        (position,issuer,principal_issuer,principal_id,request_issuer,event_id,request_sha256,payload,fingerprint)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, position, scope.Issuer, scope.PrincipalIssuer, scope.PrincipalID, child.Request.Issuer, d.EventID, d.RequestSHA256, d.Payload, intent.Fingerprint())
	if err != nil {
		return false, requestError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, requestError(err)
	}

	return true, nil
}

// GetCancellation returns owned, revalidated scoped intent, never provider state.
// Missing/wrong scope is ErrNotFound; corruption is ErrUnavailable with no evidence.
func (store *PerformanceStore) GetCancellation(ctx context.Context, scope requestintent.Scope, position int64, verify requestintent.CancellationVerifier) (requestintent.Cancellation, error) {
	if !requestintent.ValidScope(scope) || position <= 0 || verify == nil {
		return requestintent.Cancellation{}, requestintent.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	var frozen []byte
	intent, err := scanCancellation(store.runtime.pool.QueryRow(ctx, `SELECT c.event_id,c.request_sha256,c.payload,c.fingerprint,p.payload
        FROM argus_catalog.performance_cancellations c JOIN argus_catalog.performance_pending_dispatches p ON p.position=c.position
        WHERE c.position=$1 AND c.issuer=$2 AND c.principal_issuer=$3 AND c.principal_id=$4`, position, scope.Issuer, scope.PrincipalIssuer, scope.PrincipalID), &frozen)
	if err != nil {
		return requestintent.Cancellation{}, requestError(err)
	}
	if !verifiedCancellation(frozen, intent, verify) {
		return requestintent.Cancellation{}, requestintent.ErrUnavailable
	}

	return intent, nil
}

func scanCancellation(row pgx.Row, suffix ...any) (requestintent.Cancellation, error) {
	var d requestintent.CancellationData
	var fingerprint string
	if err := row.Scan(append([]any{&d.EventID, &d.RequestSHA256, &d.Payload, &fingerprint}, suffix...)...); err != nil {
		return requestintent.Cancellation{}, err
	}
	intent, err := requestintent.NewCancellation(d)
	if err != nil || intent.Fingerprint() != fingerprint {
		return requestintent.Cancellation{}, requestintent.ErrUnavailable
	}

	return intent, nil
}

func verifiedCancellation(frozen []byte, intent requestintent.Cancellation, verify requestintent.CancellationVerifier) bool {
	checked, err := verify(frozen, intent.Data().Payload)

	return err == nil && checked.Fingerprint() != "" && checked.Fingerprint() == intent.Fingerprint()
}
