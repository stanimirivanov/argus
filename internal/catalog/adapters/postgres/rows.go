package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/stanimirivanov/argus/internal/catalog"
)

// insertSnapshotGraph bulk-inserts parent rows before mappings that reference
// them. The caller owns the transaction, so the complete graph is atomic.
func insertSnapshotGraph(
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
		return catalog.ErrUnavailable
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
