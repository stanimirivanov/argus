package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/stanimirivanov/argus/internal/catalog"
)

// GetSnapshot reconstructs one immutable catalog snapshot in deterministic key
// order. Missing and invalid keys both return ErrNotFound without disclosing
// other catalog identities.
func (store *Store) GetSnapshot(ctx context.Context, key catalog.SnapshotKey) (catalog.Snapshot, error) {
	operationContext, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()

	tx, err := store.pool.BeginTx(operationContext, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return catalog.Snapshot{}, classifyDatabaseError(err)
	}
	defer func() {
		_ = tx.Rollback(operationContext) //nolint:errcheck // Best effort after commit or failure.
	}()

	snapshotID, snapshot, err := readSnapshotIdentity(operationContext, tx, key)
	if err != nil {
		return catalog.Snapshot{}, err
	}
	if snapshot.Capabilities, err = readCapabilities(operationContext, tx, snapshotID); err != nil {
		return catalog.Snapshot{}, err
	}
	if snapshot.Components, err = readComponents(operationContext, tx, snapshotID); err != nil {
		return catalog.Snapshot{}, err
	}
	if snapshot.TestSuites, err = readTestSuites(operationContext, tx, snapshotID); err != nil {
		return catalog.Snapshot{}, err
	}
	if err := tx.Commit(operationContext); err != nil {
		return catalog.Snapshot{}, classifyDatabaseError(err)
	}

	return snapshot, nil
}

func readSnapshotIdentity(
	ctx context.Context,
	tx pgx.Tx,
	key catalog.SnapshotKey,
) (int64, catalog.Snapshot, error) {
	var snapshotID int64
	var snapshot catalog.Snapshot
	err := tx.QueryRow(ctx, `
		SELECT
			s.snapshot_id,
			r.provider,
			r.host,
			r.provider_repository_id,
			s.source_owner_name,
			s.source_repository_name,
			s.revision_algorithm,
			s.revision_digest,
			s.descriptor_api_version
		FROM argus_catalog.catalog_snapshots AS s
		JOIN argus_catalog.repositories AS r
		  ON r.repository_id = s.source_repository_id
		WHERE r.provider = $1
		  AND r.host = $2
		  AND r.provider_repository_id = $3
		  AND s.revision_algorithm = $4
		  AND s.revision_digest = $5
		  AND s.descriptor_api_version = $6
	`,
		key.Repository.Provider,
		key.Repository.Host,
		key.Repository.ProviderRepositoryID,
		key.Revision.Algorithm,
		key.Revision.Digest,
		key.APIVersion,
	).Scan(
		&snapshotID,
		&snapshot.Repository.Identity.Provider,
		&snapshot.Repository.Identity.Host,
		&snapshot.Repository.Identity.ProviderRepositoryID,
		&snapshot.Repository.Owner,
		&snapshot.Repository.Name,
		&snapshot.Revision.Algorithm,
		&snapshot.Revision.Digest,
		&snapshot.APIVersion,
	)
	if err != nil {
		return 0, catalog.Snapshot{}, classifyDatabaseError(err)
	}

	return snapshotID, snapshot, nil
}

func readCapabilities(ctx context.Context, tx pgx.Tx, snapshotID int64) ([]catalog.Capability, error) {
	rows, err := tx.Query(ctx, `
		SELECT capability_key, capability_name
		FROM argus_catalog.capabilities
		WHERE snapshot_id = $1
		ORDER BY capability_key
	`, snapshotID)
	if err != nil {
		return nil, classifyDatabaseError(err)
	}
	defer rows.Close()

	capabilities := make([]catalog.Capability, 0)
	for rows.Next() {
		var capability catalog.Capability
		if err := rows.Scan(&capability.Key, &capability.Name); err != nil {
			return nil, classifyDatabaseError(err)
		}
		capabilities = append(capabilities, capability)
	}
	if err := rows.Err(); err != nil {
		return nil, classifyDatabaseError(err)
	}

	return capabilities, nil
}

func readComponents(ctx context.Context, tx pgx.Tx, snapshotID int64) ([]catalog.Component, error) {
	rows, err := tx.Query(ctx, `
		SELECT
			c.component_key,
			c.component_root,
			array_agg(m.capability_key ORDER BY m.capability_key)
		FROM argus_catalog.components AS c
		JOIN argus_catalog.component_capabilities AS m
		  ON m.snapshot_id = c.snapshot_id
		 AND m.component_key = c.component_key
		WHERE c.snapshot_id = $1
		GROUP BY c.component_key, c.component_root
		ORDER BY c.component_key
	`, snapshotID)
	if err != nil {
		return nil, classifyDatabaseError(err)
	}
	defer rows.Close()

	components := make([]catalog.Component, 0)
	for rows.Next() {
		var component catalog.Component
		if err := rows.Scan(&component.Key, &component.Root, &component.Capabilities); err != nil {
			return nil, classifyDatabaseError(err)
		}
		components = append(components, component)
	}
	if err := rows.Err(); err != nil {
		return nil, classifyDatabaseError(err)
	}

	return components, nil
}

func readTestSuites(ctx context.Context, tx pgx.Tx, snapshotID int64) ([]catalog.TestSuite, error) {
	suites, suiteIndexes, err := readSuiteRows(ctx, tx, snapshotID)
	if err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx, `
		SELECT
			r.provider,
			r.host,
			r.provider_repository_id,
			t.suite_key,
			t.test_key,
			t.test_name,
			array_agg(m.capability_key ORDER BY m.capability_key)
		FROM argus_catalog.tests AS t
		JOIN argus_catalog.repositories AS r
		  ON r.repository_id = t.test_repository_id
		JOIN argus_catalog.test_capabilities AS m
		  ON m.snapshot_id = t.snapshot_id
		 AND m.test_repository_id = t.test_repository_id
		 AND m.suite_key = t.suite_key
		 AND m.test_key = t.test_key
		WHERE t.snapshot_id = $1
		GROUP BY
			r.provider,
			r.host,
			r.provider_repository_id,
			t.test_repository_id,
			t.suite_key,
			t.test_key,
			t.test_name
		ORDER BY
			r.provider,
			r.host,
			r.provider_repository_id,
			t.suite_key,
			t.test_key
	`, snapshotID)
	if err != nil {
		return nil, classifyDatabaseError(err)
	}
	defer rows.Close()

	for rows.Next() {
		var identity catalog.RepositoryIdentity
		var suiteKey string
		var test catalog.Test
		if err := rows.Scan(
			&identity.Provider,
			&identity.Host,
			&identity.ProviderRepositoryID,
			&suiteKey,
			&test.Key,
			&test.Name,
			&test.Capabilities,
		); err != nil {
			return nil, classifyDatabaseError(err)
		}

		index, ok := suiteIndexes[suiteMapKey(identity, suiteKey)]
		if !ok {
			return nil, fmt.Errorf("reconstruct catalog snapshot: test references missing suite")
		}
		suites[index].Tests = append(suites[index].Tests, test)
	}
	if err := rows.Err(); err != nil {
		return nil, classifyDatabaseError(err)
	}

	return suites, nil
}

func readSuiteRows(
	ctx context.Context,
	tx pgx.Tx,
	snapshotID int64,
) ([]catalog.TestSuite, map[string]int, error) {
	rows, err := tx.Query(ctx, `
		SELECT
			r.provider,
			r.host,
			r.provider_repository_id,
			s.test_repository_owner_name,
			s.test_repository_name,
			s.suite_key,
			s.test_family,
			s.adapter_name
		FROM argus_catalog.test_suites AS s
		JOIN argus_catalog.repositories AS r
		  ON r.repository_id = s.test_repository_id
		WHERE s.snapshot_id = $1
		ORDER BY r.provider, r.host, r.provider_repository_id, s.suite_key
	`, snapshotID)
	if err != nil {
		return nil, nil, classifyDatabaseError(err)
	}
	defer rows.Close()

	suites := make([]catalog.TestSuite, 0)
	indexes := make(map[string]int)
	for rows.Next() {
		var suite catalog.TestSuite
		if err := rows.Scan(
			&suite.Repository.Identity.Provider,
			&suite.Repository.Identity.Host,
			&suite.Repository.Identity.ProviderRepositoryID,
			&suite.Repository.Owner,
			&suite.Repository.Name,
			&suite.Key,
			&suite.Family,
			&suite.Adapter,
		); err != nil {
			return nil, nil, classifyDatabaseError(err)
		}
		suite.Tests = make([]catalog.Test, 0)
		indexes[suiteMapKey(suite.Repository.Identity, suite.Key)] = len(suites)
		suites = append(suites, suite)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, classifyDatabaseError(err)
	}

	return suites, indexes, nil
}

func suiteMapKey(identity catalog.RepositoryIdentity, suiteKey string) string {
	return repositorySortKey(identity) + "\x00" + suiteKey
}
