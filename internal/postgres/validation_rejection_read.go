package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stanimirivanov/argus/internal/adaptation"
)

// FindValidationRejection reconstructs one immutable rejection and its runs
// from a repeatable-read snapshot.
func (store *AdaptationStore) FindValidationRejection(
	ctx context.Context,
	validationID string,
) (adaptation.ValidationRejectionEvidence, error) {
	operationContext, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	tx, err := store.runtime.pool.BeginTx(operationContext, pgx.TxOptions{
		IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return adaptation.ValidationRejectionEvidence{}, classifyAdaptationDatabaseError(err)
	}
	defer func() { _ = tx.Rollback(operationContext) }() //nolint:errcheck

	evidence, err := readValidationRejectionHeader(operationContext, tx, validationID)
	if err != nil {
		return adaptation.ValidationRejectionEvidence{}, err
	}
	evidence.Runs, err = readValidationRejectionRuns(operationContext, tx, validationID)
	if err != nil {
		return adaptation.ValidationRejectionEvidence{}, err
	}
	evidence = adaptation.CanonicalValidationRejectionEvidence(evidence)
	if err := adaptation.ValidateValidationRejectionEvidence(evidence); err != nil {
		return adaptation.ValidationRejectionEvidence{}, fmt.Errorf(
			"%w: invalid persisted validation rejection", adaptation.ErrUnavailable,
		)
	}
	if err := tx.Commit(operationContext); err != nil {
		return adaptation.ValidationRejectionEvidence{}, classifyAdaptationDatabaseError(err)
	}

	return evidence, nil
}

func readValidationRejectionHeader(
	ctx context.Context,
	tx pgx.Tx,
	validationID string,
) (adaptation.ValidationRejectionEvidence, error) {
	var evidence adaptation.ValidationRejectionEvidence
	err := tx.QueryRow(ctx, `
		SELECT
			v.rejection_api_version, v.validation_policy_version, v.validation_id,
			v.proposal_id, v.proposal_policy_version,
			r.provider, r.host, r.provider_repository_id,
			v.repository_owner_name, v.repository_name,
			v.revision_algorithm, v.revision_digest,
			v.suite_key, v.test_key, v.test_name, v.adapter_id, v.adapter_version, v.capabilities,
			v.edit_path, v.edit_before_sha256, v.edit_start_byte, v.edit_end_byte,
			v.edit_original, v.edit_replacement, v.edit_semantic_role,
			v.candidate_sha256, v.negative_sha256, v.negative_control_path,
			v.rejected_phase, v.rejection_reason, v.expected_outcome, v.actual_outcome
		FROM argus_catalog.adaptation_validation_rejections AS v
		JOIN argus_catalog.repositories AS r ON r.repository_id = v.repository_id
		WHERE v.validation_id = $1
	`, validationID).Scan(
		&evidence.APIVersion, &evidence.PolicyVersion, &evidence.ValidationID,
		&evidence.ProposalID, &evidence.ProposalPolicyVersion,
		&evidence.Test.Repository.Identity.Provider, &evidence.Test.Repository.Identity.Host,
		&evidence.Test.Repository.Identity.ProviderRepositoryID,
		&evidence.Test.Repository.Owner, &evidence.Test.Repository.Name,
		&evidence.Test.Revision.Algorithm, &evidence.Test.Revision.Digest,
		&evidence.Test.SuiteKey, &evidence.Test.TestKey, &evidence.Test.Name,
		&evidence.AdapterID, &evidence.AdapterVersion, &evidence.Test.Capabilities,
		&evidence.Edit.Path, &evidence.Edit.BeforeSHA256, &evidence.Edit.StartByte, &evidence.Edit.EndByte,
		&evidence.Edit.Original, &evidence.Edit.Replacement, &evidence.Edit.SemanticRole,
		&evidence.Source.CandidateSHA256, &evidence.Source.NegativeSHA256,
		&evidence.Source.NegativeControlPath, &evidence.RejectedPhase, &evidence.Reason,
		&evidence.ExpectedOutcome, &evidence.ActualOutcome,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return adaptation.ValidationRejectionEvidence{}, adaptation.ErrValidationRejectionNotFound
	}
	if err != nil {
		return adaptation.ValidationRejectionEvidence{}, classifyAdaptationDatabaseError(err)
	}
	evidence.Test.Adapter = evidence.AdapterID
	evidence.Source.Path = evidence.Edit.Path
	evidence.Source.OriginalSHA256 = evidence.Edit.BeforeSHA256
	evidence.Source.RestoredSHA256 = evidence.Edit.BeforeSHA256

	return evidence, nil
}

func readValidationRejectionRuns(
	ctx context.Context,
	tx pgx.Tx,
	validationID string,
) ([]adaptation.ValidationRun, error) {
	rows, err := tx.Query(ctx, `
		SELECT phase, source_sha256, started_at, completed_at, outcome, failure_code, failure_message
		FROM argus_catalog.adaptation_validation_rejection_runs
		WHERE validation_id = $1
		ORDER BY run_ordinal
	`, validationID)
	if err != nil {
		return nil, classifyAdaptationDatabaseError(err)
	}
	defer rows.Close()

	runs := make([]adaptation.ValidationRun, 0, 3)
	for rows.Next() {
		var run adaptation.ValidationRun
		var failureCode, failureMessage pgtype.Text
		if err := rows.Scan(
			&run.Phase, &run.SourceSHA256, &run.StartedAt, &run.CompletedAt,
			&run.Outcome, &failureCode, &failureMessage,
		); err != nil {
			return nil, classifyAdaptationDatabaseError(err)
		}
		run.StartedAt, run.CompletedAt = run.StartedAt.UTC(), run.CompletedAt.UTC()
		if failureCode.Valid && failureMessage.Valid {
			run.Failure = &adaptation.ValidationFailure{Code: failureCode.String, Message: failureMessage.String}
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, classifyAdaptationDatabaseError(err)
	}

	return runs, nil
}
