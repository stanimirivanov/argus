package postgres

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/stanimirivanov/argus/internal/catalog"
)

// SaveSnapshot persists one complete immutable catalog snapshot atomically.
// It returns false for an exact retry and catalog.ErrConflict when the same
// snapshot identity is already bound to different normalized content.
func (store *Store) SaveSnapshot(ctx context.Context, snapshot catalog.Snapshot) (bool, error) {
	canonical := catalog.CanonicalSnapshot(snapshot)
	fingerprint, err := snapshotFingerprint(canonical)
	if err != nil {
		return false, fmt.Errorf("fingerprint catalog snapshot: %w", err)
	}

	tx, operationContext, cancel, err := store.beginWrite(ctx)
	if err != nil {
		return false, err
	}
	defer cancel()
	defer func() {
		_ = tx.Rollback(operationContext) //nolint:errcheck // Best effort after commit or failure.
	}()

	sourceRepositoryID, err := findOrCreateRepositoryIdentity(operationContext, tx, canonical.Repository)
	if err != nil {
		return false, err
	}

	snapshotID, created, err := claimSnapshotIdentity(
		operationContext,
		tx,
		canonical,
		sourceRepositoryID,
		fingerprint,
	)
	if err != nil || !created {
		return created, err
	}

	repositoryIDs, err := resolveSnapshotRepositoryIDs(operationContext, tx, canonical)
	if err != nil {
		return false, err
	}
	if err := insertSnapshotGraph(operationContext, tx, snapshotID, canonical, repositoryIDs); err != nil {
		return false, err
	}
	if err := tx.Commit(operationContext); err != nil {
		return false, classifyDatabaseError(err)
	}

	return true, nil
}

// findOrCreateRepositoryIdentity resolves the provider identity without
// treating mutable owner/name coordinates as part of uniqueness.
func findOrCreateRepositoryIdentity(ctx context.Context, tx pgx.Tx, repository catalog.Repository) (int64, error) {
	if _, err := tx.Exec(ctx, `
		INSERT INTO argus_catalog.repositories (
			provider,
			host,
			provider_repository_id,
			owner_name,
			repository_name
		) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (provider, host, provider_repository_id) DO NOTHING
	`,
		repository.Identity.Provider,
		repository.Identity.Host,
		repository.Identity.ProviderRepositoryID,
		repository.Owner,
		repository.Name,
	); err != nil {
		return 0, classifyDatabaseError(err)
	}

	var repositoryID int64
	if err := tx.QueryRow(ctx, `
		SELECT repository_id
		FROM argus_catalog.repositories
		WHERE provider = $1
		  AND host = $2
		  AND provider_repository_id = $3
	`,
		repository.Identity.Provider,
		repository.Identity.Host,
		repository.Identity.ProviderRepositoryID,
	).Scan(&repositoryID); err != nil {
		return 0, classifyDatabaseError(err)
	}

	return repositoryID, nil
}

// claimSnapshotIdentity uses insert-then-read so concurrent writers agree on
// one immutable identity and exact retries can be distinguished from conflicts.
func claimSnapshotIdentity(
	ctx context.Context,
	tx pgx.Tx,
	snapshot catalog.Snapshot,
	sourceRepositoryID int64,
	fingerprint string,
) (int64, bool, error) {
	var snapshotID int64
	err := tx.QueryRow(ctx, `
		INSERT INTO argus_catalog.catalog_snapshots (
			source_repository_id,
			source_owner_name,
			source_repository_name,
			revision_algorithm,
			revision_digest,
			descriptor_api_version,
			descriptor_sha256
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (
			source_repository_id,
			revision_algorithm,
			revision_digest,
			descriptor_api_version
		) DO NOTHING
		RETURNING snapshot_id
	`,
		sourceRepositoryID,
		snapshot.Repository.Owner,
		snapshot.Repository.Name,
		snapshot.Revision.Algorithm,
		snapshot.Revision.Digest,
		snapshot.APIVersion,
		fingerprint,
	).Scan(&snapshotID)
	if err == nil {
		return snapshotID, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, false, classifyDatabaseError(err)
	}

	var existingFingerprint string
	if err := tx.QueryRow(ctx, `
		SELECT snapshot_id, descriptor_sha256
		FROM argus_catalog.catalog_snapshots
		WHERE source_repository_id = $1
		  AND revision_algorithm = $2
		  AND revision_digest = $3
		  AND descriptor_api_version = $4
	`,
		sourceRepositoryID,
		snapshot.Revision.Algorithm,
		snapshot.Revision.Digest,
		snapshot.APIVersion,
	).Scan(&snapshotID, &existingFingerprint); err != nil {
		return 0, false, classifyDatabaseError(err)
	}
	if existingFingerprint != fingerprint {
		return 0, false, catalog.ErrConflict
	}

	return snapshotID, false, nil
}

// resolveSnapshotRepositoryIDs sorts identities to keep concurrent lock order
// stable, then refreshes only the repository table's latest display coordinates.
// Historical coordinates remain stored on the immutable snapshot and suites.
func resolveSnapshotRepositoryIDs(
	ctx context.Context,
	tx pgx.Tx,
	snapshot catalog.Snapshot,
) (map[catalog.RepositoryIdentity]int64, error) {
	repositories := map[catalog.RepositoryIdentity]catalog.Repository{
		snapshot.Repository.Identity: snapshot.Repository,
	}
	for _, suite := range snapshot.TestSuites {
		repositories[suite.Repository.Identity] = suite.Repository
	}

	identities := make([]catalog.RepositoryIdentity, 0, len(repositories))
	for identity := range repositories {
		identities = append(identities, identity)
	}
	slices.SortFunc(identities, compareRepositoryIdentities)

	repositoryIDs := make(map[catalog.RepositoryIdentity]int64, len(repositories))
	for _, identity := range identities {
		repository := repositories[identity]
		repositoryID, err := findOrCreateRepositoryIdentity(ctx, tx, repository)
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE argus_catalog.repositories
			SET owner_name = $2,
			    repository_name = $3,
			    last_observed_at = statement_timestamp()
			WHERE repository_id = $1
		`, repositoryID, repository.Owner, repository.Name); err != nil {
			return nil, classifyDatabaseError(err)
		}
		repositoryIDs[identity] = repositoryID
	}

	return repositoryIDs, nil
}
