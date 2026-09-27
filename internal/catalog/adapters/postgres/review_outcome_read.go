package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stanimirivanov/argus/internal/adaptation"
)

// FindReviewOutcome reconstructs one immutable outcome and all reviewer edits
// from a repeatable-read snapshot.
func (store *Store) FindReviewOutcome(
	ctx context.Context,
	outcomeID string,
) (adaptation.ReviewOutcome, error) {
	operationContext, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	tx, err := store.pool.BeginTx(operationContext, pgx.TxOptions{
		IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return adaptation.ReviewOutcome{}, classifyAdaptationDatabaseError(err)
	}
	defer func() {
		_ = tx.Rollback(operationContext) //nolint:errcheck // Best effort after commit or failure.
	}()

	outcomeKey, reviewOutcome, err := readReviewOutcomeHeader(operationContext, tx, outcomeID)
	if err != nil {
		return adaptation.ReviewOutcome{}, err
	}
	reviewOutcome.ReviewerEdits, err = readReviewOutcomeEdits(operationContext, tx, outcomeKey)
	if err != nil {
		return adaptation.ReviewOutcome{}, err
	}
	reviewOutcome = adaptation.CanonicalReviewOutcome(reviewOutcome)
	if err := adaptation.ValidateReviewOutcome(reviewOutcome); err != nil {
		return adaptation.ReviewOutcome{}, fmt.Errorf(
			"%w: invalid persisted review outcome", adaptation.ErrUnavailable,
		)
	}
	if err := tx.Commit(operationContext); err != nil {
		return adaptation.ReviewOutcome{}, classifyAdaptationDatabaseError(err)
	}

	return reviewOutcome, nil
}

func readReviewOutcomeHeader(
	ctx context.Context,
	tx pgx.Tx,
	outcomeID string,
) (int64, adaptation.ReviewOutcome, error) {
	var outcomeKey int64
	var reviewOutcome adaptation.ReviewOutcome
	var reasonNote pgtype.Text
	var mergedAt pgtype.Timestamptz
	err := tx.QueryRow(ctx, `
		SELECT
			o.review_outcome_id, o.outcome_api_version, o.outcome_id,
			o.review_id, o.proposal_id, o.validation_id,
			r.provider, r.host, r.provider_repository_id,
			o.repository_owner_name, o.repository_name,
			o.pull_request_number, o.pull_request_url,
			o.review_decision, o.reason_code, o.reason_note,
			o.generated_revision_algorithm, o.generated_revision_digest,
			o.final_revision_algorithm, o.final_revision_digest,
			o.closed_at, o.merged_at, o.observed_at
		FROM argus_catalog.adaptation_review_outcomes AS o
		JOIN argus_catalog.repositories AS r ON r.repository_id = o.repository_id
		WHERE o.outcome_id = $1
	`, outcomeID).Scan(
		&outcomeKey, &reviewOutcome.APIVersion, &reviewOutcome.OutcomeID,
		&reviewOutcome.ReviewID, &reviewOutcome.ProposalID, &reviewOutcome.ValidationID,
		&reviewOutcome.Repository.Identity.Provider, &reviewOutcome.Repository.Identity.Host,
		&reviewOutcome.Repository.Identity.ProviderRepositoryID,
		&reviewOutcome.Repository.Owner, &reviewOutcome.Repository.Name,
		&reviewOutcome.PullRequestNumber, &reviewOutcome.PullRequestURL,
		&reviewOutcome.Decision, &reviewOutcome.ReasonCode, &reasonNote,
		&reviewOutcome.GeneratedRevision.Algorithm, &reviewOutcome.GeneratedRevision.Digest,
		&reviewOutcome.FinalRevision.Algorithm, &reviewOutcome.FinalRevision.Digest,
		&reviewOutcome.ClosedAt, &mergedAt, &reviewOutcome.ObservedAt,
	)
	if err != nil {
		return 0, adaptation.ReviewOutcome{}, classifyAdaptationDatabaseError(err)
	}
	if reasonNote.Valid {
		reviewOutcome.ReasonNote = reasonNote.String
	}
	if mergedAt.Valid {
		value := mergedAt.Time.UTC()
		reviewOutcome.MergedAt = &value
	}
	reviewOutcome.ClosedAt = reviewOutcome.ClosedAt.UTC()
	reviewOutcome.ObservedAt = reviewOutcome.ObservedAt.UTC()

	return outcomeKey, reviewOutcome, nil
}

func readReviewOutcomeEdits(
	ctx context.Context,
	tx pgx.Tx,
	outcomeKey int64,
) ([]adaptation.ReviewFileEdit, error) {
	rows, err := tx.Query(ctx, `
		SELECT path, previous_path, edit_kind, additions, deletions, patch
		FROM argus_catalog.adaptation_review_edits
		WHERE review_outcome_id = $1
		ORDER BY path COLLATE "C", previous_path COLLATE "C" NULLS FIRST
	`, outcomeKey)
	if err != nil {
		return nil, classifyAdaptationDatabaseError(err)
	}
	defer rows.Close()

	edits := make([]adaptation.ReviewFileEdit, 0)
	for rows.Next() {
		var edit adaptation.ReviewFileEdit
		var previousPath pgtype.Text
		if err := rows.Scan(
			&edit.Path, &previousPath, &edit.Kind, &edit.Additions, &edit.Deletions, &edit.Patch,
		); err != nil {
			return nil, classifyAdaptationDatabaseError(err)
		}
		if previousPath.Valid {
			edit.PreviousPath = previousPath.String
		}
		edits = append(edits, edit)
	}
	if err := rows.Err(); err != nil {
		return nil, classifyAdaptationDatabaseError(err)
	}

	return edits, nil
}
