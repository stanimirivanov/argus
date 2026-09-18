package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/stanimirivanov/argus/internal/change"
)

// SaveCapabilityImpact stores one immutable assessment and all explainability
// evidence atomically. Exact retries are no-ops; divergent analyzer output for
// the same change set is rejected.
func (store *Store) SaveCapabilityImpact(ctx context.Context, impact change.CapabilityImpact) (bool, error) {
	impact = change.CanonicalCapabilityImpact(impact)
	if err := change.ValidateCapabilityImpact(impact); err != nil {
		return false, err
	}
	fingerprint, err := capabilityImpactFingerprint(impact)
	if err != nil {
		return false, change.ErrUnavailable
	}
	tx, operationContext, cancel, err := store.beginWrite(ctx)
	if err != nil {
		return false, classifyChangeDatabaseError(err)
	}
	defer cancel()
	defer func() { _ = tx.Rollback(operationContext) }() //nolint:errcheck

	changeSetID, stored, err := readChangeSetHeader(
		operationContext, tx, impact.Change.Trigger.Provider, impact.Change.Trigger.DeliveryID,
	)
	if err != nil {
		return false, err
	}
	if stored.ChangeSet.Reference() != impact.Change {
		return false, change.ErrConflict
	}

	assessmentID, created, err := claimCapabilityImpact(
		operationContext, tx, changeSetID, impact, fingerprint,
	)
	if err != nil || !created {
		return created, err
	}
	if err := insertCapabilityImpactDetails(operationContext, tx, assessmentID, impact); err != nil {
		return false, err
	}
	if err := tx.Commit(operationContext); err != nil {
		return false, classifyChangeDatabaseError(err)
	}

	return true, nil
}

func claimCapabilityImpact(
	ctx context.Context,
	tx pgx.Tx,
	changeSetID int64,
	impact change.CapabilityImpact,
	fingerprint string,
) (int64, bool, error) {
	var assessmentID int64
	err := tx.QueryRow(ctx, `
		INSERT INTO argus_catalog.openapi_impact_assessments (
			change_set_id, impact_api_version, analyzer_version, impact_status, impact_sha256
		) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (change_set_id) DO NOTHING
		RETURNING assessment_id
	`, changeSetID, impact.APIVersion, impact.AnalyzerVersion, impact.Status, fingerprint).Scan(&assessmentID)
	if err == nil {
		return assessmentID, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, false, classifyChangeDatabaseError(err)
	}
	var existingFingerprint string
	if err := tx.QueryRow(ctx, `
		SELECT assessment_id, impact_sha256
		FROM argus_catalog.openapi_impact_assessments
		WHERE change_set_id = $1
	`, changeSetID).Scan(&assessmentID, &existingFingerprint); err != nil {
		return 0, false, classifyChangeDatabaseError(err)
	}
	if existingFingerprint != fingerprint {
		return 0, false, change.ErrConflict
	}

	return assessmentID, false, nil
}

func insertCapabilityImpactDetails(
	ctx context.Context,
	tx pgx.Tx,
	assessmentID int64,
	impact change.CapabilityImpact,
) error {
	for _, document := range impact.Documents {
		if _, err := tx.Exec(ctx, `
			INSERT INTO argus_catalog.openapi_document_impacts (
				assessment_id, document_path, previous_document_path, change_kind,
				total_changes, breaking_changes
			) VALUES ($1, $2, $3, $4, $5, $6)
		`, assessmentID, document.Path, document.PreviousPath, document.Kind,
			document.TotalChanges, document.BreakingChanges); err != nil {
			return classifyChangeDatabaseError(err)
		}
		for _, operation := range document.Operations {
			if _, err := tx.Exec(ctx, `
				INSERT INTO argus_catalog.openapi_operation_impacts (
					assessment_id, document_path, http_method, operation_path,
					operation_id, change_kind, potentially_breaking
				) VALUES ($1, $2, $3, $4, $5, $6, $7)
			`, assessmentID, document.Path, operation.Method, operation.Path,
				operation.OperationID, operation.Kind, operation.PotentialBreak); err != nil {
				return classifyChangeDatabaseError(err)
			}
			for _, capability := range operation.Capabilities {
				if _, err := tx.Exec(ctx, `
					INSERT INTO argus_catalog.openapi_operation_capabilities (
						assessment_id, document_path, http_method, operation_path, capability_key
					) VALUES ($1, $2, $3, $4, $5)
				`, assessmentID, document.Path, operation.Method, operation.Path, capability); err != nil {
					return classifyChangeDatabaseError(err)
				}
			}
		}
	}
	for index, warning := range impact.Warnings {
		if _, err := tx.Exec(ctx, `
			INSERT INTO argus_catalog.openapi_impact_warnings (
				assessment_id, warning_ordinal, warning_text
			) VALUES ($1, $2, $3)
		`, assessmentID, index, warning); err != nil {
			return classifyChangeDatabaseError(err)
		}
	}

	return nil
}
