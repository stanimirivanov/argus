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
func (store *ChangeStore) SaveCapabilityImpact(ctx context.Context, impact change.CapabilityImpact) (bool, error) {
	impact = change.CanonicalCapabilityImpact(impact)
	if err := change.ValidateCapabilityImpact(impact); err != nil {
		return false, err
	}
	fingerprint, err := capabilityImpactFingerprint(impact)
	if err != nil {
		return false, change.ErrUnavailable
	}
	tx, operationContext, cancel, err := store.runtime.beginWrite(ctx)
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
	if impact.APIVersion == change.CompositeImpactAPIVersion {
		stored.ChangeSet.Files, err = readChangeFiles(operationContext, tx, changeSetID)
		if err != nil {
			return false, err
		}
		if err := change.ValidateImpactFileCoverage(impact, stored.ChangeSet); err != nil {
			return false, err
		}
	}

	created, err := persistImpactAssessments(operationContext, tx, changeSetID, impact, fingerprint)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(operationContext); err != nil {
		return false, classifyChangeDatabaseError(err)
	}

	return created, nil
}

func persistImpactAssessments(
	ctx context.Context, tx pgx.Tx, changeSetID int64, impact change.CapabilityImpact, fingerprint string,
) (bool, error) {
	semantic := semanticImpactV1(impact)
	semanticFingerprint, err := capabilityImpactFingerprint(semantic)
	if err != nil {
		return false, change.ErrUnavailable
	}
	assessmentID, semanticCreated, err := claimCapabilityImpact(ctx, tx, changeSetID, semantic, semanticFingerprint)
	if err != nil {
		return false, err
	}
	if semanticCreated {
		if err := insertCapabilityImpactDetails(ctx, tx, assessmentID, semantic); err != nil {
			return false, err
		}
	}
	if impact.APIVersion != change.CompositeImpactAPIVersion {
		return semanticCreated, nil
	}
	created, err := claimCompositeImpact(ctx, tx, changeSetID, impact, fingerprint)
	if err != nil {
		return false, err
	}
	if created {
		if err := insertFileImpacts(ctx, tx, changeSetID, impact.Files); err != nil {
			return false, err
		}
	}

	return created, nil
}

func semanticImpactV1(impact change.CapabilityImpact) change.CapabilityImpact {
	if impact.APIVersion == change.ImpactAPIVersion {
		return impact
	}
	semantic := impact
	semantic.APIVersion = change.ImpactAPIVersion
	semantic.AnalyzerVersion = change.OpenAPIAnalyzerVersion
	semantic.Status = impact.SemanticStatus
	semantic.Warnings = append([]string(nil), impact.SemanticWarnings...)
	semantic.Files = nil
	semantic.SemanticStatus, semantic.FileStatus = "", ""
	semantic.SemanticWarnings, semantic.FileWarnings = nil, nil

	return change.CanonicalCapabilityImpact(semantic)
}

func claimCompositeImpact(ctx context.Context, tx pgx.Tx, changeSetID int64, impact change.CapabilityImpact, fingerprint string) (bool, error) {
	var inserted int64
	err := tx.QueryRow(ctx, `
		INSERT INTO argus_catalog.composite_impact_assessments (
			change_set_id, impact_api_version, analyzer_version, impact_status,
			file_status, file_warnings, impact_sha256
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (change_set_id) DO NOTHING
		RETURNING change_set_id
	`, changeSetID, impact.APIVersion, impact.AnalyzerVersion, impact.Status,
		impact.FileStatus, nonNilStrings(impact.FileWarnings), fingerprint).Scan(&inserted)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, classifyChangeDatabaseError(err)
	}
	var existingFingerprint string
	if err := tx.QueryRow(ctx, `
		SELECT impact_sha256 FROM argus_catalog.composite_impact_assessments WHERE change_set_id = $1
	`, changeSetID).Scan(&existingFingerprint); err != nil {
		return false, classifyChangeDatabaseError(err)
	}
	if existingFingerprint != fingerprint {
		return false, change.ErrConflict
	}

	return false, nil
}

func insertFileImpacts(ctx context.Context, tx pgx.Tx, changeSetID int64, files []change.FileImpact) error {
	for _, file := range files {
		if _, err := tx.Exec(ctx, `
			INSERT INTO argus_catalog.change_file_impacts (
				change_set_id, file_path, previous_file_path, change_kind, coverage,
				capability_keys, component_roots, unmapped_paths
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, changeSetID, file.Path, file.PreviousPath, file.Kind, file.Coverage,
			nonNilStrings(file.Capabilities), nonNilStrings(file.ComponentRoots), nonNilStrings(file.UnmappedPaths)); err != nil {
			return classifyChangeDatabaseError(err)
		}
	}

	return nil
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
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
