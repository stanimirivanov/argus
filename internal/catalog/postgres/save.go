package postgres

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/stanimirivanov/argus/internal/catalog"
)

// SaveSnapshot persists one complete immutable catalog snapshot atomically.
// It returns false for an exact retry and ErrConflict when the same snapshot
// identity is already bound to different normalized content.
func (store *Store) SaveSnapshot(ctx context.Context, snapshot catalog.Snapshot) (bool, error) {
	canonical := canonicalSnapshot(snapshot)
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

	sourceRepositoryID, err := ensureRepository(operationContext, tx, canonical.Repository)
	if err != nil {
		return false, err
	}

	snapshotID, created, err := insertSnapshot(
		operationContext,
		tx,
		canonical,
		sourceRepositoryID,
		fingerprint,
	)
	if err != nil || !created {
		return created, err
	}

	repositoryIDs, err := ensureSnapshotRepositories(operationContext, tx, canonical)
	if err != nil {
		return false, err
	}
	if err := copySnapshotRows(operationContext, tx, snapshotID, canonical, repositoryIDs); err != nil {
		return false, err
	}
	if err := tx.Commit(operationContext); err != nil {
		return false, classifyDatabaseError(err)
	}

	return true, nil
}

func ensureRepository(ctx context.Context, tx pgx.Tx, repository catalog.Repository) (int64, error) {
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

func insertSnapshot(
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
		return 0, false, ErrConflict
	}

	return snapshotID, false, nil
}

func ensureSnapshotRepositories(
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
	slices.SortFunc(identities, func(left, right catalog.RepositoryIdentity) int {
		return cmp.Compare(repositorySortKey(left), repositorySortKey(right))
	})

	repositoryIDs := make(map[catalog.RepositoryIdentity]int64, len(repositories))
	for _, identity := range identities {
		repository := repositories[identity]
		repositoryID, err := ensureRepository(ctx, tx, repository)
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

func copySnapshotRows(
	ctx context.Context,
	tx pgx.Tx,
	snapshotID int64,
	snapshot catalog.Snapshot,
	repositoryIDs map[catalog.RepositoryIdentity]int64,
) error {
	copyOperations := []struct {
		table   pgx.Identifier
		columns []string
		rows    [][]any
	}{
		{
			table:   pgx.Identifier{"argus_catalog", "capabilities"},
			columns: []string{"snapshot_id", "capability_key", "capability_name"},
			rows:    capabilityRows(snapshotID, snapshot.Capabilities),
		},
		{
			table:   pgx.Identifier{"argus_catalog", "components"},
			columns: []string{"snapshot_id", "component_key", "component_root"},
			rows:    componentRows(snapshotID, snapshot.Components),
		},
		{
			table:   pgx.Identifier{"argus_catalog", "component_capabilities"},
			columns: []string{"snapshot_id", "component_key", "capability_key"},
			rows:    componentCapabilityRows(snapshotID, snapshot.Components),
		},
		{
			table: pgx.Identifier{"argus_catalog", "test_suites"},
			columns: []string{
				"snapshot_id",
				"test_repository_id",
				"suite_key",
				"test_repository_owner_name",
				"test_repository_name",
				"test_family",
				"adapter_name",
			},
			rows: testSuiteRows(snapshotID, snapshot.TestSuites, repositoryIDs),
		},
		{
			table: pgx.Identifier{"argus_catalog", "tests"},
			columns: []string{
				"snapshot_id",
				"test_repository_id",
				"suite_key",
				"test_key",
				"test_name",
			},
			rows: testRows(snapshotID, snapshot.TestSuites, repositoryIDs),
		},
		{
			table: pgx.Identifier{"argus_catalog", "test_capabilities"},
			columns: []string{
				"snapshot_id",
				"test_repository_id",
				"suite_key",
				"test_key",
				"capability_key",
			},
			rows: testCapabilityRows(snapshotID, snapshot.TestSuites, repositoryIDs),
		},
	}

	for _, operation := range copyOperations {
		if err := copyRows(ctx, tx, operation.table, operation.columns, operation.rows); err != nil {
			return err
		}
	}

	return nil
}

func copyRows(
	ctx context.Context,
	tx pgx.Tx,
	table pgx.Identifier,
	columns []string,
	rows [][]any,
) error {
	if len(rows) == 0 {
		return nil
	}

	copied, err := tx.CopyFrom(ctx, table, columns, pgx.CopyFromRows(rows))
	if err != nil {
		return classifyDatabaseError(err)
	}
	if copied != int64(len(rows)) {
		return ErrUnavailable
	}

	return nil
}

func capabilityRows(snapshotID int64, capabilities []catalog.Capability) [][]any {
	rows := make([][]any, 0, len(capabilities))
	for _, capability := range capabilities {
		rows = append(rows, []any{snapshotID, capability.Key, capability.Name})
	}

	return rows
}

func componentRows(snapshotID int64, components []catalog.Component) [][]any {
	rows := make([][]any, 0, len(components))
	for _, component := range components {
		rows = append(rows, []any{snapshotID, component.Key, component.Root})
	}

	return rows
}

func componentCapabilityRows(snapshotID int64, components []catalog.Component) [][]any {
	var rows [][]any
	for _, component := range components {
		for _, capability := range component.Capabilities {
			rows = append(rows, []any{snapshotID, component.Key, capability})
		}
	}

	return rows
}

func testSuiteRows(
	snapshotID int64,
	suites []catalog.TestSuite,
	repositoryIDs map[catalog.RepositoryIdentity]int64,
) [][]any {
	rows := make([][]any, 0, len(suites))
	for _, suite := range suites {
		rows = append(rows, []any{
			snapshotID,
			repositoryIDs[suite.Repository.Identity],
			suite.Key,
			suite.Repository.Owner,
			suite.Repository.Name,
			suite.Family,
			suite.Adapter,
		})
	}

	return rows
}

func testRows(
	snapshotID int64,
	suites []catalog.TestSuite,
	repositoryIDs map[catalog.RepositoryIdentity]int64,
) [][]any {
	var rows [][]any
	for _, suite := range suites {
		for _, test := range suite.Tests {
			rows = append(rows, []any{
				snapshotID,
				repositoryIDs[suite.Repository.Identity],
				suite.Key,
				test.Key,
				test.Name,
			})
		}
	}

	return rows
}

func testCapabilityRows(
	snapshotID int64,
	suites []catalog.TestSuite,
	repositoryIDs map[catalog.RepositoryIdentity]int64,
) [][]any {
	var rows [][]any
	for _, suite := range suites {
		for _, test := range suite.Tests {
			for _, capability := range test.Capabilities {
				rows = append(rows, []any{
					snapshotID,
					repositoryIDs[suite.Repository.Identity],
					suite.Key,
					test.Key,
					capability,
				})
			}
		}
	}

	return rows
}
