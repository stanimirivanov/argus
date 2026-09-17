package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/stanimirivanov/argus/internal/catalog"
)

// ListTestCatalogEntries reads one keyset page from an immutable snapshot. The
// application service owns cursor encoding; this adapter sees only the typed,
// exclusive position of the last delivered test.
func (store *Store) ListTestCatalogEntries(
	ctx context.Context,
	request catalog.TestCatalogReadRequest,
) (catalog.TestCatalogReadPage, error) {
	if request.Limit < 1 || request.Limit > catalog.MaxTestCatalogPageSize {
		return catalog.TestCatalogReadPage{}, catalog.ErrInvalidQuery
	}

	operationContext, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()

	tx, err := store.pool.BeginTx(operationContext, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return catalog.TestCatalogReadPage{}, classifyDatabaseError(err)
	}
	defer func() {
		_ = tx.Rollback(operationContext) //nolint:errcheck // Best effort after commit or failure.
	}()

	snapshotID, snapshot, err := readSnapshotHeader(operationContext, tx, request.Snapshot)
	if err != nil {
		return catalog.TestCatalogReadPage{}, err
	}
	items, hasMore, err := readTestCatalogPage(operationContext, tx, snapshotID, request)
	if err != nil {
		return catalog.TestCatalogReadPage{}, err
	}
	if err := tx.Commit(operationContext); err != nil {
		return catalog.TestCatalogReadPage{}, classifyDatabaseError(err)
	}

	return catalog.TestCatalogReadPage{
		Snapshot: catalog.SnapshotReference{
			SourceRepository:     snapshot.Repository,
			Revision:             snapshot.Revision,
			DescriptorAPIVersion: snapshot.APIVersion,
		},
		Items:   items,
		HasMore: hasMore,
	}, nil
}

func readTestCatalogPage(
	ctx context.Context,
	tx pgx.Tx,
	snapshotID int64,
	request catalog.TestCatalogReadRequest,
) ([]catalog.TestCatalogEntry, bool, error) {
	var afterProvider any
	var afterHost any
	var afterRepositoryID any
	var afterSuiteKey any
	var afterTestKey any
	if request.After != nil {
		afterProvider = request.After.TestRepository.Provider
		afterHost = request.After.TestRepository.Host
		afterRepositoryID = request.After.TestRepository.ProviderRepositoryID
		afterSuiteKey = request.After.SuiteKey
		afterTestKey = request.After.TestKey
	}

	rows, err := tx.Query(ctx, `
		SELECT
			r.provider,
			r.host,
			r.provider_repository_id,
			s.test_repository_owner_name,
			s.test_repository_name,
			s.suite_key,
			s.test_family,
			s.adapter_name,
			t.test_key,
			t.test_name,
			array_agg(c.capability_key ORDER BY c.capability_key COLLATE "C"),
			array_agg(c.capability_name ORDER BY c.capability_key COLLATE "C")
		FROM argus_catalog.tests AS t
		JOIN argus_catalog.test_suites AS s
		  ON s.snapshot_id = t.snapshot_id
		 AND s.test_repository_id = t.test_repository_id
		 AND s.suite_key = t.suite_key
		JOIN argus_catalog.repositories AS r
		  ON r.repository_id = t.test_repository_id
		JOIN argus_catalog.test_capabilities AS m
		  ON m.snapshot_id = t.snapshot_id
		 AND m.test_repository_id = t.test_repository_id
		 AND m.suite_key = t.suite_key
		 AND m.test_key = t.test_key
		JOIN argus_catalog.capabilities AS c
		  ON c.snapshot_id = m.snapshot_id
		 AND c.capability_key = m.capability_key
		WHERE t.snapshot_id = $1
		  AND (
			$2::text = ''
			OR EXISTS (
				SELECT 1
				FROM argus_catalog.test_capabilities AS selected
				WHERE selected.snapshot_id = t.snapshot_id
				  AND selected.test_repository_id = t.test_repository_id
				  AND selected.suite_key = t.suite_key
				  AND selected.test_key = t.test_key
				  AND selected.capability_key = $2
			)
		  )
		  AND (
			$3::text IS NULL
			OR (
				r.provider COLLATE "C",
				r.host COLLATE "C",
				r.provider_repository_id COLLATE "C",
				t.suite_key COLLATE "C",
				t.test_key COLLATE "C"
			) > (
				$3::text COLLATE "C",
				$4::text COLLATE "C",
				$5::text COLLATE "C",
				$6::text COLLATE "C",
				$7::text COLLATE "C"
			)
		  )
		GROUP BY
			r.provider,
			r.host,
			r.provider_repository_id,
			t.test_repository_id,
			s.test_repository_owner_name,
			s.test_repository_name,
			s.suite_key,
			s.test_family,
			s.adapter_name,
			t.test_key,
			t.test_name
		ORDER BY
			r.provider COLLATE "C",
			r.host COLLATE "C",
			r.provider_repository_id COLLATE "C",
			s.suite_key COLLATE "C",
			t.test_key COLLATE "C"
		LIMIT $8
	`,
		snapshotID,
		request.CapabilityKey,
		afterProvider,
		afterHost,
		afterRepositoryID,
		afterSuiteKey,
		afterTestKey,
		request.Limit+1,
	)
	if err != nil {
		return nil, false, classifyDatabaseError(err)
	}
	defer rows.Close()

	items := make([]catalog.TestCatalogEntry, 0, request.Limit+1)
	for rows.Next() {
		entry, scanErr := scanTestCatalogEntry(rows)
		if scanErr != nil {
			return nil, false, scanErr
		}
		items = append(items, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, false, classifyDatabaseError(err)
	}
	rows.Close()

	hasMore := len(items) > request.Limit
	if hasMore {
		items = items[:request.Limit]
	}

	return items, hasMore, nil
}

func scanTestCatalogEntry(rows pgx.Rows) (catalog.TestCatalogEntry, error) {
	var entry catalog.TestCatalogEntry
	var capabilityKeys []string
	var capabilityNames []string
	if err := rows.Scan(
		&entry.TestRepository.Identity.Provider,
		&entry.TestRepository.Identity.Host,
		&entry.TestRepository.Identity.ProviderRepositoryID,
		&entry.TestRepository.Owner,
		&entry.TestRepository.Name,
		&entry.SuiteKey,
		&entry.Family,
		&entry.Adapter,
		&entry.TestKey,
		&entry.Name,
		&capabilityKeys,
		&capabilityNames,
	); err != nil {
		return catalog.TestCatalogEntry{}, classifyDatabaseError(err)
	}
	if len(capabilityKeys) != len(capabilityNames) {
		return catalog.TestCatalogEntry{}, catalog.ErrUnavailable
	}

	entry.Capabilities = make([]catalog.Capability, len(capabilityKeys))
	for index := range capabilityKeys {
		entry.Capabilities[index] = catalog.Capability{
			Key:  capabilityKeys[index],
			Name: capabilityNames[index],
		}
	}

	return entry, nil
}
