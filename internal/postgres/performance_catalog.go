package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stanimirivanov/argus/internal/performance/catalogsync"
)

// Publish atomically appends evidence and advances its scoped current pointer.
// Replay is read-only, even for a historical/expired snapshot. New publication
// must be fresh and strictly newer; a deferred database trigger rechecks at commit.
func (store *PerformanceStore) Publish(ctx context.Context, snapshot catalogsync.Snapshot) (bool, error) {
	if snapshot.Fingerprint() == "" {
		return false, catalogsync.ErrInvalid
	}
	data := snapshot.Evidence()
	tx, ctx, cancel, err := store.runtime.beginWrite(ctx)
	if err != nil {
		return false, performanceError(err)
	}
	defer cancel()
	defer func() {
		_ = tx.Rollback(ctx) //nolint:errcheck // Best effort after commit or failure.
	}()
	scope, err := json.Marshal(data.Scope)
	if err != nil {
		return false, catalogsync.ErrInvalid
	}
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 809188))", string(scope)); err != nil {
		return false, performanceError(err)
	}
	var fingerprint string
	err = tx.QueryRow(ctx, `SELECT fingerprint FROM argus_catalog.performance_catalog_snapshots
        WHERE issuer=$1 AND principal_issuer=$2 AND principal_id=$3 AND source_id=$4`, data.Scope.Issuer, data.Scope.PrincipalIssuer, data.Scope.PrincipalID, data.ID).Scan(&fingerprint)
	if err == nil {
		if fingerprint != snapshot.Fingerprint() {
			return false, catalogsync.ErrConflict
		}
		return false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, performanceError(err)
	}
	if err := checkPerformanceGeneration(ctx, tx, snapshot); err != nil {
		return false, err
	}
	id := uuid.New()
	if _, err := tx.Exec(ctx, `INSERT INTO argus_catalog.performance_catalog_snapshots
        (snapshot_uuid,issuer,principal_issuer,principal_id,source_id,source_revision,generated_at,valid_until,
         fingerprint,content_sha256,schema_sha256,source_pages,resource_count)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, id, data.Scope.Issuer, data.Scope.PrincipalIssuer, data.Scope.PrincipalID,
		data.ID, data.Revision, data.GeneratedAt, data.ValidUntil, snapshot.Fingerprint(), data.ContentSHA256, data.SchemaSHA256, []byte(data.Pages), len(data.Resources)); err != nil {
		return false, performanceError(err)
	}
	rows := make([][]any, 0, len(data.Resources))
	for _, resource := range data.Resources {
		rows = append(rows, []any{id, resource.Kind, resource.Issuer, resource.ID, resource.Revision, resource.SHA256, resource.State})
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"argus_catalog", "performance_catalog_resources"}, []string{"snapshot_uuid", "kind", "issuer", "resource_id", "revision", "sha256", "lifecycle_state"}, pgx.CopyFromRows(rows)); err != nil {
		return false, performanceError(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO argus_catalog.performance_catalog_current
        (issuer,principal_issuer,principal_id,snapshot_uuid) VALUES ($1,$2,$3,$4)
        ON CONFLICT (issuer,principal_issuer,principal_id) DO UPDATE SET snapshot_uuid=EXCLUDED.snapshot_uuid`, data.Scope.Issuer, data.Scope.PrincipalIssuer, data.Scope.PrincipalID, id); err != nil {
		return false, performanceError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, performanceError(err)
	}

	return true, nil
}

func checkPerformanceGeneration(ctx context.Context, tx pgx.Tx, snapshot catalogsync.Snapshot) error {
	var now time.Time
	if err := tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now); err != nil {
		return performanceError(err)
	}
	if !snapshot.Fresh(now) {
		return catalogsync.ErrExpired
	}
	data := snapshot.Evidence()
	var previous time.Time
	err := tx.QueryRow(ctx, `SELECT s.generated_at FROM argus_catalog.performance_catalog_current c
        JOIN argus_catalog.performance_catalog_snapshots s ON s.snapshot_uuid=c.snapshot_uuid
        WHERE c.issuer=$1 AND c.principal_issuer=$2 AND c.principal_id=$3`, data.Scope.Issuer, data.Scope.PrincipalIssuer, data.Scope.PrincipalID).Scan(&previous)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return performanceError(err)
	}
	if err == nil && !data.GeneratedAt.After(previous) {
		return catalogsync.ErrOlder
	}

	return nil
}

// Current reads a coherent last-published view, including expired history. It
// cannot imply eligibility; consumers must inspect Snapshot.Fresh at use time.
func (store *PerformanceStore) Current(ctx context.Context, scope catalogsync.Scope) (catalogsync.Snapshot, error) {
	return store.readPerformance(ctx, scope, `SELECT s.snapshot_uuid::text,s.source_id,s.source_revision,
        s.generated_at,s.valid_until,s.content_sha256,s.schema_sha256,s.source_pages,s.fingerprint,s.resource_count
        FROM argus_catalog.performance_catalog_current c JOIN argus_catalog.performance_catalog_snapshots s
        ON s.snapshot_uuid=c.snapshot_uuid WHERE c.issuer=$1 AND c.principal_issuer=$2 AND c.principal_id=$3`, scope.Issuer, scope.PrincipalIssuer, scope.PrincipalID)
}

// Get reads one immutable source ID within its original authority scope.
func (store *PerformanceStore) Get(ctx context.Context, scope catalogsync.Scope, id string) (catalogsync.Snapshot, error) {
	return store.readPerformance(ctx, scope, `SELECT snapshot_uuid::text,source_id,source_revision,
        generated_at,valid_until,content_sha256,schema_sha256,source_pages,fingerprint,resource_count
        FROM argus_catalog.performance_catalog_snapshots WHERE issuer=$1 AND principal_issuer=$2 AND principal_id=$3 AND source_id=$4`, scope.Issuer, scope.PrincipalIssuer, scope.PrincipalID, id)
}

func (store *PerformanceStore) readPerformance(ctx context.Context, scope catalogsync.Scope, query string, args ...any) (catalogsync.Snapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	data := catalogsync.Evidence{Scope: scope}
	var id, fingerprint string
	var count int
	if err := store.runtime.pool.QueryRow(ctx, query, args...).Scan(&id, &data.ID, &data.Revision, &data.GeneratedAt, &data.ValidUntil, &data.ContentSHA256, &data.SchemaSHA256, &data.Pages, &fingerprint, &count); err != nil {
		return catalogsync.Snapshot{}, performanceError(err)
	}
	rows, err := store.runtime.pool.Query(ctx, `SELECT kind,issuer,resource_id,revision,sha256,lifecycle_state
        FROM argus_catalog.performance_catalog_resources WHERE snapshot_uuid=$1`, id)
	if err != nil {
		return catalogsync.Snapshot{}, performanceError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var resource catalogsync.Resource
		if err := rows.Scan(&resource.Kind, &resource.Issuer, &resource.ID, &resource.Revision, &resource.SHA256, &resource.State); err != nil {
			return catalogsync.Snapshot{}, performanceError(err)
		}
		data.Resources = append(data.Resources, resource)
	}
	if err := rows.Err(); err != nil {
		return catalogsync.Snapshot{}, performanceError(err)
	}
	snapshot, err := catalogsync.NewSnapshot(data)
	if err != nil || snapshot.Fingerprint() != fingerprint || len(data.Resources) != count {
		return catalogsync.Snapshot{}, catalogsync.ErrUnavailable
	}

	return snapshot, nil
}

func performanceError(err error) error {
	for _, known := range []error{context.Canceled, context.DeadlineExceeded} {
		if errors.Is(err, known) {
			return known
		}
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return catalogsync.ErrNotFound
	}
	var database *pgconn.PgError
	if errors.As(err, &database) {
		switch database.ConstraintName {
		case "ck_performance_catalog_freshness":
			return catalogsync.ErrExpired
		case "ck_performance_catalog_generation":
			return catalogsync.ErrOlder
		}
	}

	return catalogsync.ErrUnavailable
}
