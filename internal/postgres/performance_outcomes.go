package postgres

import (
	"bytes"
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stanimirivanov/argus/internal/performance"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

// SettleRequest atomically retains admission/recovery evidence under live scoped
// ownership. Exact replay by the winning owner returns false even after expiry;
// changed evidence/ownership conflicts. Lost commit responses require exact replay
// or scoped readback, never fresh submission. No network I/O occurs here.
func (store *PerformanceStore) SettleRequest(ctx context.Context, scope requestintent.Scope, claim requestintent.Claim, outcome requestintent.Outcome) (bool, error) {
	token, err := uuid.Parse(claim.Token)
	data := outcome.Data()
	if !requestintent.ValidScope(scope) || claim.Pending.After <= 0 || claim.Fence <= 0 || err != nil || token == uuid.Nil || outcome.Fingerprint() == "" || data.Source != scope.Issuer {
		return false, requestintent.ErrInvalid
	}
	tx, ctx, cancel, err := store.runtime.beginWrite(ctx)
	if err != nil {
		return false, requestError(err)
	}
	defer cancel()
	defer func() { _ = tx.Rollback(ctx) }() //nolint:errcheck // Best effort after commit or failure.
	var request, attempt performance.Identity
	err = tx.QueryRow(ctx, `SELECT request_issuer,request_id,attempt_issuer,attempt_id FROM argus_catalog.performance_pending_dispatches
        WHERE position=$1 AND issuer=$2 AND principal_issuer=$3 AND principal_id=$4 FOR UPDATE`, claim.Pending.After, scope.Issuer, scope.PrincipalIssuer, scope.PrincipalID).Scan(&request.Issuer, &request.ID, &attempt.Issuer, &attempt.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, requestintent.ErrConflict
	}
	if err != nil {
		return false, requestError(err)
	}
	if data.Request != request || data.Attempt != attempt {
		return false, requestintent.ErrConflict
	}
	var fingerprint, owner, kind, eventID string
	var payload []byte
	var fence int64
	err = tx.QueryRow(ctx, `SELECT fingerprint,fence,token::text,kind,event_id,payload FROM argus_catalog.performance_dispatch_outcomes WHERE position=$1`, claim.Pending.After).Scan(&fingerprint, &fence, &owner, &kind, &eventID, &payload)
	if err == nil {
		if fingerprint != outcome.Fingerprint() || fence != claim.Fence || owner != token.String() || kind != data.Kind || eventID != data.EventID || !bytes.Equal(payload, data.Payload) {
			return false, requestintent.ErrConflict
		}

		return false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, requestError(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO argus_catalog.performance_dispatch_outcomes
        (position,issuer,principal_issuer,principal_id,fence,token,kind,event_id,payload,fingerprint)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, claim.Pending.After, scope.Issuer, scope.PrincipalIssuer, scope.PrincipalID, claim.Fence, token, data.Kind, data.EventID, data.Payload, outcome.Fingerprint())
	if err != nil {
		return false, requestError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, requestError(err)
	}

	return true, nil
}

// RequestOutcome returns owned settlement evidence by pending position in an
// independently authorized scope. Missing/wrong scope is ErrNotFound; corrupt
// evidence is unavailable. It neither orders lifecycle nor grants dispatch rights.
func (store *PerformanceStore) RequestOutcome(ctx context.Context, scope requestintent.Scope, position int64) (requestintent.Outcome, error) {
	if !requestintent.ValidScope(scope) || position <= 0 {
		return requestintent.Outcome{}, requestintent.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	data := requestintent.OutcomeData{Source: scope.Issuer}
	var fingerprint string
	err := store.runtime.pool.QueryRow(ctx, `SELECT p.request_issuer,p.request_id,p.attempt_issuer,p.attempt_id,o.kind,o.event_id,o.payload,o.fingerprint
        FROM argus_catalog.performance_dispatch_outcomes o JOIN argus_catalog.performance_pending_dispatches p ON p.position=o.position
        WHERE o.position=$1 AND o.issuer=$2 AND o.principal_issuer=$3 AND o.principal_id=$4`, position, scope.Issuer, scope.PrincipalIssuer, scope.PrincipalID).Scan(&data.Request.Issuer, &data.Request.ID, &data.Attempt.Issuer, &data.Attempt.ID, &data.Kind, &data.EventID, &data.Payload, &fingerprint)
	if err != nil {
		return requestintent.Outcome{}, requestError(err)
	}
	outcome, err := requestintent.NewOutcome(data)
	if err != nil || outcome.Fingerprint() != fingerprint {
		return requestintent.Outcome{}, requestintent.ErrUnavailable
	}

	return outcome, nil
}
