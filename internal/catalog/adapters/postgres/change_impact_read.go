package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
)

// FindCapabilityImpact reconstructs one assessment from a repeatable-read
// snapshot so header, documents, operations, mappings, and warnings agree.
func (store *Store) FindCapabilityImpact(
	ctx context.Context,
	provider catalog.Provider,
	deliveryID string,
) (change.CapabilityImpact, error) {
	operationContext, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	tx, err := store.pool.BeginTx(operationContext, pgx.TxOptions{
		IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return change.CapabilityImpact{}, classifyChangeDatabaseError(err)
	}
	defer func() { _ = tx.Rollback(operationContext) }() //nolint:errcheck

	changeSetID, stored, err := readChangeSetHeader(operationContext, tx, provider, deliveryID)
	if err != nil {
		return change.CapabilityImpact{}, err
	}
	impact := change.CapabilityImpact{Change: stored.ChangeSet.Reference()}
	var assessmentID int64
	if err := tx.QueryRow(operationContext, `
		SELECT assessment_id, impact_api_version, analyzer_version, impact_status
		FROM argus_catalog.openapi_impact_assessments
		WHERE change_set_id = $1
	`, changeSetID).Scan(&assessmentID, &impact.APIVersion, &impact.AnalyzerVersion, &impact.Status); err != nil {
		return change.CapabilityImpact{}, classifyChangeDatabaseError(err)
	}
	impact.Documents, err = readDocumentImpacts(operationContext, tx, assessmentID)
	if err != nil {
		return change.CapabilityImpact{}, err
	}
	impact.Warnings, err = readImpactWarnings(operationContext, tx, assessmentID)
	if err != nil {
		return change.CapabilityImpact{}, err
	}
	impact = change.CanonicalCapabilityImpact(impact)
	if err := change.ValidateCapabilityImpact(impact); err != nil {
		return change.CapabilityImpact{}, fmt.Errorf("%w: invalid persisted capability impact", change.ErrUnavailable)
	}
	if err := tx.Commit(operationContext); err != nil {
		return change.CapabilityImpact{}, classifyChangeDatabaseError(err)
	}

	return impact, nil
}

func readDocumentImpacts(ctx context.Context, tx pgx.Tx, assessmentID int64) ([]change.DocumentImpact, error) {
	rows, err := tx.Query(ctx, `
		SELECT document_path, previous_document_path, change_kind, total_changes, breaking_changes
		FROM argus_catalog.openapi_document_impacts
		WHERE assessment_id = $1
		ORDER BY document_path
	`, assessmentID)
	if err != nil {
		return nil, classifyChangeDatabaseError(err)
	}
	documents, err := pgx.CollectRows(rows, pgx.RowToStructByPos[documentRow])
	if err != nil {
		return nil, classifyChangeDatabaseError(err)
	}
	result := make([]change.DocumentImpact, 0, len(documents))
	for _, row := range documents {
		operations, err := readOperationImpacts(ctx, tx, assessmentID, row.Path)
		if err != nil {
			return nil, err
		}
		result = append(result, change.DocumentImpact{
			Path: row.Path, PreviousPath: row.PreviousPath, Kind: row.Kind,
			TotalChanges: row.TotalChanges, BreakingChanges: row.BreakingChanges,
			Operations: operations,
		})
	}

	return result, nil
}

type documentRow struct {
	Path            string
	PreviousPath    *string
	Kind            change.SemanticChangeKind
	TotalChanges    int
	BreakingChanges int
}

func readOperationImpacts(
	ctx context.Context,
	tx pgx.Tx,
	assessmentID int64,
	documentPath string,
) ([]change.OperationImpact, error) {
	rows, err := tx.Query(ctx, `
		SELECT
			o.http_method, o.operation_path, o.operation_id, o.change_kind,
			o.potentially_breaking,
			COALESCE(array_agg(c.capability_key ORDER BY c.capability_key)
				FILTER (WHERE c.capability_key IS NOT NULL), '{}')
		FROM argus_catalog.openapi_operation_impacts AS o
		LEFT JOIN argus_catalog.openapi_operation_capabilities AS c
		  ON c.assessment_id = o.assessment_id
		 AND c.document_path = o.document_path
		 AND c.http_method = o.http_method
		 AND c.operation_path = o.operation_path
		WHERE o.assessment_id = $1 AND o.document_path = $2
		GROUP BY o.http_method, o.operation_path, o.operation_id, o.change_kind, o.potentially_breaking
		ORDER BY o.operation_path, o.http_method
	`, assessmentID, documentPath)
	if err != nil {
		return nil, classifyChangeDatabaseError(err)
	}
	defer rows.Close()
	operations := make([]change.OperationImpact, 0)
	for rows.Next() {
		var operation change.OperationImpact
		if err := rows.Scan(
			&operation.Method, &operation.Path, &operation.OperationID, &operation.Kind,
			&operation.PotentialBreak, &operation.Capabilities,
		); err != nil {
			return nil, classifyChangeDatabaseError(err)
		}
		operations = append(operations, operation)
	}
	if err := rows.Err(); err != nil {
		return nil, classifyChangeDatabaseError(err)
	}

	return operations, nil
}

func readImpactWarnings(ctx context.Context, tx pgx.Tx, assessmentID int64) ([]string, error) {
	rows, err := tx.Query(ctx, `
		SELECT warning_text
		FROM argus_catalog.openapi_impact_warnings
		WHERE assessment_id = $1
		ORDER BY warning_ordinal
	`, assessmentID)
	if err != nil {
		return nil, classifyChangeDatabaseError(err)
	}
	warnings, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, classifyChangeDatabaseError(err)
	}

	return warnings, nil
}
