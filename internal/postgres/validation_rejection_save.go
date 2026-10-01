package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/stanimirivanov/argus/internal/adaptation"
)

// SaveValidationRejection atomically stores one immutable policy rejection and
// its completed run prefix. Exact retries return false; divergent reuse of a
// validation identity is rejected.
func (store *AdaptationStore) SaveValidationRejection(
	ctx context.Context,
	evidence adaptation.ValidationRejectionEvidence,
) (bool, error) {
	evidence = adaptation.CanonicalValidationRejectionEvidence(evidence)
	if err := adaptation.ValidateValidationRejectionEvidence(evidence); err != nil {
		return false, err
	}
	fingerprint, err := validationRejectionFingerprint(evidence)
	if err != nil {
		return false, err
	}
	tx, operationContext, cancel, err := store.runtime.beginWrite(ctx)
	if err != nil {
		return false, classifyAdaptationDatabaseError(err)
	}
	defer cancel()
	defer func() { _ = tx.Rollback(operationContext) }() //nolint:errcheck

	repositoryID, err := findOrCreateRepositoryIdentity(operationContext, tx, evidence.Test.Repository)
	if err != nil {
		return false, classifyAdaptationDatabaseError(err)
	}
	created, err := claimValidationRejection(operationContext, tx, evidence, repositoryID, fingerprint)
	if err != nil || !created {
		return created, err
	}
	if err := insertValidationRejectionRuns(operationContext, tx, evidence); err != nil {
		return false, err
	}
	if err := tx.Commit(operationContext); err != nil {
		return false, classifyAdaptationDatabaseError(err)
	}

	return true, nil
}

func claimValidationRejection(
	ctx context.Context,
	tx pgx.Tx,
	evidence adaptation.ValidationRejectionEvidence,
	repositoryID int64,
	fingerprint string,
) (bool, error) {
	var validationID string
	err := tx.QueryRow(ctx, `
		INSERT INTO argus_catalog.adaptation_validation_rejections (
			validation_id, rejection_api_version, validation_policy_version,
			proposal_id, proposal_policy_version, repository_id,
			repository_owner_name, repository_name, revision_algorithm, revision_digest,
			suite_key, test_key, test_name, adapter_id, adapter_version, capabilities,
			edit_path, edit_before_sha256, edit_start_byte, edit_end_byte,
			edit_original, edit_replacement, edit_semantic_role,
			candidate_sha256, negative_sha256, negative_control_path,
			rejected_phase, rejection_reason, expected_outcome, actual_outcome, rejection_sha256
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16,
			$17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30, $31
		)
		ON CONFLICT (validation_id) DO NOTHING
		RETURNING validation_id
	`, evidence.ValidationID, evidence.APIVersion, evidence.PolicyVersion,
		evidence.ProposalID, evidence.ProposalPolicyVersion, repositoryID,
		evidence.Test.Repository.Owner, evidence.Test.Repository.Name,
		evidence.Test.Revision.Algorithm, evidence.Test.Revision.Digest,
		evidence.Test.SuiteKey, evidence.Test.TestKey, evidence.Test.Name,
		evidence.AdapterID, evidence.AdapterVersion, evidence.Test.Capabilities,
		evidence.Edit.Path, evidence.Edit.BeforeSHA256, evidence.Edit.StartByte, evidence.Edit.EndByte,
		evidence.Edit.Original, evidence.Edit.Replacement, evidence.Edit.SemanticRole,
		evidence.Source.CandidateSHA256, evidence.Source.NegativeSHA256, evidence.Source.NegativeControlPath,
		evidence.RejectedPhase, evidence.Reason, evidence.ExpectedOutcome, evidence.ActualOutcome, fingerprint,
	).Scan(&validationID)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, classifyAdaptationDatabaseError(err)
	}
	var existingFingerprint string
	if err := tx.QueryRow(ctx, `
		SELECT rejection_sha256
		FROM argus_catalog.adaptation_validation_rejections
		WHERE validation_id = $1
	`, evidence.ValidationID).Scan(&existingFingerprint); err != nil {
		return false, classifyAdaptationDatabaseError(err)
	}
	if existingFingerprint != fingerprint {
		return false, adaptation.ErrValidationRejectionConflict
	}

	return false, nil
}

func insertValidationRejectionRuns(
	ctx context.Context,
	tx pgx.Tx,
	evidence adaptation.ValidationRejectionEvidence,
) error {
	rows := make([][]any, 0, len(evidence.Runs))
	for index, run := range evidence.Runs {
		var failureCode, failureMessage any
		if run.Failure != nil {
			failureCode, failureMessage = run.Failure.Code, run.Failure.Message
		}
		rows = append(rows, []any{
			evidence.ValidationID, index, run.Phase, run.SourceSHA256,
			run.StartedAt, run.CompletedAt, run.Outcome, failureCode, failureMessage,
		})
	}
	copied, err := tx.CopyFrom(
		ctx, pgx.Identifier{"argus_catalog", "adaptation_validation_rejection_runs"},
		[]string{
			"validation_id", "run_ordinal", "phase", "source_sha256", "started_at",
			"completed_at", "outcome", "failure_code", "failure_message",
		},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return classifyAdaptationDatabaseError(err)
	}
	if copied != int64(len(rows)) {
		return fmt.Errorf("%w: incomplete validation rejection write", adaptation.ErrUnavailable)
	}

	return nil
}
