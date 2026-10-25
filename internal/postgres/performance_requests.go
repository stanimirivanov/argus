package postgres

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

// CreateRequest atomically retains the complete intent and its ordered child work.
// Callers supply independently authorized, adapter-validated evidence. Exact replay
// returns false without writing, even after expiry/catalog replacement; changed
// evidence or reused identities conflict. New work must be live at commit.
// Calls are concurrent-safe, bounded and cancellable; no network calls occur in
// the transaction. A lost commit response requires exact replay to resolve outcome.
func (store *PerformanceStore) CreateRequest(ctx context.Context, intent requestintent.Intent) (bool, error) {
	if intent.Fingerprint() == "" {
		return false, requestintent.ErrInvalid
	}
	tx, ctx, cancel, err := store.runtime.beginWrite(ctx)
	if err != nil {
		return false, requestError(err)
	}
	defer cancel()
	defer func() { _ = tx.Rollback(ctx) }() //nolint:errcheck // Best effort after commit or failure.

	data := intent.Data()
	// Identical JSON field order and seed to Publish serialize the authority scope.
	scope, err := json.Marshal(data.Scope)
	if err != nil {
		return false, requestintent.ErrInvalid
	}
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 809188))", string(scope)); err != nil {
		return false, requestError(err)
	}
	var fingerprint string
	err = tx.QueryRow(ctx, `SELECT fingerprint FROM argus_catalog.performance_request_intents
        WHERE issuer=$1 AND principal_issuer=$2 AND principal_id=$3 AND review_sha256=$4`, data.Scope.Issuer, data.Scope.PrincipalIssuer, data.Scope.PrincipalID, data.ReviewSHA256).Scan(&fingerprint)
	if err == nil {
		if fingerprint != intent.Fingerprint() {
			return false, requestintent.ErrConflict
		}

		return false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, requestError(err)
	}
	if err := insertRequest(ctx, tx, data, intent.Fingerprint()); err != nil {
		return false, requestError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, requestError(err)
	}

	return true, nil
}

func insertRequest(ctx context.Context, tx pgx.Tx, data requestintent.Data, fingerprint string) error {
	var catalogID string
	err := tx.QueryRow(ctx, `SELECT s.snapshot_uuid::text FROM argus_catalog.performance_catalog_current c
        JOIN argus_catalog.performance_catalog_snapshots s ON s.snapshot_uuid=c.snapshot_uuid
        WHERE c.issuer=$1 AND c.principal_issuer=$2 AND c.principal_id=$3 AND s.fingerprint=$4`, data.Scope.Issuer, data.Scope.PrincipalIssuer, data.Scope.PrincipalID, data.CatalogSHA256).Scan(&catalogID)
	if errors.Is(err, pgx.ErrNoRows) {
		return requestintent.ErrConflict
	}
	if err != nil {
		return err
	}
	id := uuid.New()
	_, err = tx.Exec(ctx, `INSERT INTO argus_catalog.performance_request_intents
        (intent_uuid,issuer,principal_issuer,principal_id,review_sha256,catalog_sha256,fingerprint,catalog_uuid,evidence,child_count)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, id, data.Scope.Issuer, data.Scope.PrincipalIssuer, data.Scope.PrincipalID,
		data.ReviewSHA256, data.CatalogSHA256, fingerprint, catalogID, data.Evidence, len(data.Children))
	if err != nil {
		return err
	}
	for ordinal, child := range data.Children {
		reasons, err := json.Marshal(child.Reasons)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO argus_catalog.performance_pending_dispatches
            (intent_uuid,ordinal,issuer,principal_issuer,principal_id,request_issuer,request_id,attempt_issuer,attempt_id,event_id,requested_at,deadline,payload,payload_sha256,reasons)
            VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, id, ordinal, data.Scope.Issuer, data.Scope.PrincipalIssuer, data.Scope.PrincipalID,
			child.Request.Issuer, child.Request.ID, child.Attempt.Issuer, child.Attempt.ID, child.EventID, child.RequestedAt, child.Deadline, child.Payload, child.SHA256, reasons)
		if err != nil {
			return err
		}
	}

	return nil
}

const dispatchColumns = `request_issuer,request_id,attempt_issuer,attempt_id,event_id,requested_at,deadline,payload,payload_sha256,reasons`

// GetRequest reads owned evidence by review digest within the trusted caller scope.
// Immutable rows make the two bounded queries coherent without a long read transaction.
// Missing/wrong-scope evidence returns ErrNotFound; corrupt evidence is unavailable.
func (store *PerformanceStore) GetRequest(ctx context.Context, scope requestintent.Scope, reviewSHA256 string) (requestintent.Intent, error) {
	raw, err := hex.DecodeString(reviewSHA256)
	if !requestintent.ValidScope(scope) || err != nil || len(raw) != 32 || strings.ToLower(reviewSHA256) != reviewSHA256 {
		return requestintent.Intent{}, requestintent.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	data := requestintent.Data{Scope: scope, ReviewSHA256: reviewSHA256}
	var id, fingerprint string
	var count int
	if err := store.runtime.pool.QueryRow(ctx, `SELECT intent_uuid::text,catalog_sha256,evidence,fingerprint,child_count
        FROM argus_catalog.performance_request_intents WHERE issuer=$1 AND principal_issuer=$2 AND principal_id=$3 AND review_sha256=$4`, scope.Issuer, scope.PrincipalIssuer, scope.PrincipalID, reviewSHA256).Scan(&id, &data.CatalogSHA256, &data.Evidence, &fingerprint, &count); err != nil {
		return requestintent.Intent{}, requestError(err)
	}
	rows, err := store.runtime.pool.Query(ctx, `SELECT `+dispatchColumns+` FROM argus_catalog.performance_pending_dispatches WHERE intent_uuid=$1 ORDER BY ordinal`, id)
	if err != nil {
		return requestintent.Intent{}, requestError(err)
	}
	defer rows.Close()
	for rows.Next() {
		child, err := scanDispatch(rows)
		if err != nil {
			return requestintent.Intent{}, requestError(err)
		}
		data.Children = append(data.Children, child)
	}
	if err := rows.Err(); err != nil {
		return requestintent.Intent{}, requestError(err)
	}
	intent, err := requestintent.New(data)
	if err != nil || intent.Fingerprint() != fingerprint || len(data.Children) != count {
		return requestintent.Intent{}, requestintent.ErrUnavailable
	}

	return intent, nil
}

// PendingRequests returns owned work in increasing database-local cursor order,
// including expired work. Limit is 1..100; after is nonnegative. Scope must come
// from independent authorization. Pages grant no lease and are not a durable
// snapshot: restart at zero after concurrent creation (allocation != commit order).
func (store *PerformanceStore) PendingRequests(ctx context.Context, scope requestintent.Scope, after int64, limit int) ([]requestintent.Pending, error) {
	if !requestintent.ValidScope(scope) || after < 0 || limit < 1 || limit > 100 {
		return nil, requestintent.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	rows, err := store.runtime.pool.Query(ctx, `SELECT position,intent_uuid::text,`+dispatchColumns+`
        FROM argus_catalog.performance_pending_dispatches WHERE issuer=$1 AND principal_issuer=$2 AND principal_id=$3 AND position>$4 ORDER BY position LIMIT $5`, scope.Issuer, scope.PrincipalIssuer, scope.PrincipalID, after, limit)
	if err != nil {
		return nil, requestError(err)
	}
	defer rows.Close()
	result := make([]requestintent.Pending, 0, limit)
	for rows.Next() {
		var pending requestintent.Pending
		child, err := scanDispatch(rows, &pending.After, &pending.IntentID)
		if err != nil {
			return nil, requestError(err)
		}
		pending.Child = child
		result = append(result, pending)
	}
	if err := rows.Err(); err != nil {
		return nil, requestError(err)
	}

	return result, nil
}

func scanDispatch(row pgx.Row, prefix ...any) (requestintent.Child, error) {
	var child requestintent.Child
	var reasons []byte
	err := row.Scan(append(prefix, &child.Request.Issuer, &child.Request.ID, &child.Attempt.Issuer, &child.Attempt.ID,
		&child.EventID, &child.RequestedAt, &child.Deadline, &child.Payload, &child.SHA256, &reasons)...)
	if err != nil {
		return child, err
	}
	if json.Unmarshal(reasons, &child.Reasons) != nil || !child.Valid() {
		return requestintent.Child{}, requestintent.ErrUnavailable
	}
	child.RequestedAt, child.Deadline = child.RequestedAt.UTC(), child.Deadline.UTC()

	return child, nil
}

func requestError(err error) error {
	for _, known := range []error{context.Canceled, context.DeadlineExceeded, requestintent.ErrConflict} {
		if errors.Is(err, known) {
			return known
		}
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return requestintent.ErrNotFound
	}
	var database *pgconn.PgError
	if errors.As(err, &database) {
		if database.Code == "23505" || database.ConstraintName == "ck_performance_request_catalog" {
			return requestintent.ErrConflict
		}
		if database.ConstraintName == "ck_performance_request_freshness" {
			return requestintent.ErrExpired
		}
	}

	return requestintent.ErrUnavailable
}
