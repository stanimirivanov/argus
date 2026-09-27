package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/stanimirivanov/argus/internal/adaptation"
)

// SaveReviewOutcome atomically persists one immutable terminal review outcome
// and its complete reviewer edits. Exact semantic retries return false;
// divergent reuse of a review identity returns adaptation.ErrOutcomeConflict.
func (store *Store) SaveReviewOutcome(
	ctx context.Context,
	reviewOutcome adaptation.ReviewOutcome,
) (bool, error) {
	reviewOutcome = adaptation.CanonicalReviewOutcome(reviewOutcome)
	if err := adaptation.ValidateReviewOutcome(reviewOutcome); err != nil {
		return false, err
	}
	fingerprint, err := reviewOutcomeFingerprint(reviewOutcome)
	if err != nil {
		return false, err
	}
	tx, operationContext, cancel, err := store.beginWrite(ctx)
	if err != nil {
		return false, classifyAdaptationDatabaseError(err)
	}
	defer cancel()
	defer func() {
		_ = tx.Rollback(operationContext) //nolint:errcheck // Best effort after commit or failure.
	}()

	repositoryID, err := findOrCreateRepositoryIdentity(operationContext, tx, reviewOutcome.Repository)
	if err != nil {
		return false, classifyAdaptationDatabaseError(err)
	}
	outcomeKey, created, err := claimReviewOutcome(
		operationContext, tx, reviewOutcome, repositoryID, fingerprint,
	)
	if err != nil || !created {
		return created, err
	}
	if err := insertReviewOutcomeEdits(operationContext, tx, outcomeKey, reviewOutcome); err != nil {
		return false, err
	}
	if err := tx.Commit(operationContext); err != nil {
		return false, classifyAdaptationDatabaseError(err)
	}

	return true, nil
}

func claimReviewOutcome(
	ctx context.Context,
	tx pgx.Tx,
	reviewOutcome adaptation.ReviewOutcome,
	repositoryID int64,
	fingerprint string,
) (int64, bool, error) {
	var outcomeKey int64
	err := tx.QueryRow(ctx, `
		INSERT INTO argus_catalog.adaptation_review_outcomes (
			outcome_id, outcome_api_version, review_id, proposal_id, validation_id,
			repository_id, repository_owner_name, repository_name,
			pull_request_number, pull_request_url, review_decision, reason_code, reason_note,
			generated_revision_algorithm, generated_revision_digest,
			final_revision_algorithm, final_revision_digest,
			closed_at, merged_at, observed_at, outcome_sha256
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11,
			$12, $13, $14, $15, $16, $17, $18, $19, $20, $21
		)
		ON CONFLICT DO NOTHING
		RETURNING review_outcome_id
	`,
		reviewOutcome.OutcomeID, reviewOutcome.APIVersion, reviewOutcome.ReviewID,
		reviewOutcome.ProposalID, reviewOutcome.ValidationID, repositoryID,
		reviewOutcome.Repository.Owner, reviewOutcome.Repository.Name,
		reviewOutcome.PullRequestNumber, reviewOutcome.PullRequestURL,
		reviewOutcome.Decision, reviewOutcome.ReasonCode, nullableString(reviewOutcome.ReasonNote),
		reviewOutcome.GeneratedRevision.Algorithm, reviewOutcome.GeneratedRevision.Digest,
		reviewOutcome.FinalRevision.Algorithm, reviewOutcome.FinalRevision.Digest,
		reviewOutcome.ClosedAt, reviewOutcome.MergedAt, reviewOutcome.ObservedAt, fingerprint,
	).Scan(&outcomeKey)
	if err == nil {
		return outcomeKey, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, false, classifyAdaptationDatabaseError(err)
	}

	var existingFingerprint string
	if err := tx.QueryRow(ctx, `
		SELECT review_outcome_id, outcome_sha256
		FROM argus_catalog.adaptation_review_outcomes
		WHERE review_id = $1
	`, reviewOutcome.ReviewID).Scan(&outcomeKey, &existingFingerprint); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, false, adaptation.ErrOutcomeConflict
		}
		return 0, false, classifyAdaptationDatabaseError(err)
	}
	if existingFingerprint != fingerprint {
		return 0, false, adaptation.ErrOutcomeConflict
	}

	return outcomeKey, false, nil
}

func insertReviewOutcomeEdits(
	ctx context.Context,
	tx pgx.Tx,
	outcomeKey int64,
	reviewOutcome adaptation.ReviewOutcome,
) error {
	rows := make([][]any, 0, len(reviewOutcome.ReviewerEdits))
	for _, edit := range reviewOutcome.ReviewerEdits {
		rows = append(rows, []any{
			outcomeKey, edit.Path, nullableString(edit.PreviousPath), edit.Kind,
			edit.Additions, edit.Deletions, edit.Patch,
		})
	}
	if len(rows) == 0 {
		return nil
	}
	copied, err := tx.CopyFrom(
		ctx,
		pgx.Identifier{"argus_catalog", "adaptation_review_edits"},
		[]string{"review_outcome_id", "path", "previous_path", "edit_kind", "additions", "deletions", "patch"},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return classifyAdaptationDatabaseError(err)
	}
	if copied != int64(len(rows)) {
		return fmt.Errorf("%w: incomplete review outcome write", adaptation.ErrUnavailable)
	}

	return nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}

	return value
}
