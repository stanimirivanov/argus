package contract

import (
	"fmt"
	"time"

	"github.com/stanimirivanov/argus/contracts"
	"github.com/stanimirivanov/argus/internal/adaptation"
)

// ExportReviewOutcomeV1 converts terminal domain evidence to its public contract.
func ExportReviewOutcomeV1(outcome adaptation.ReviewOutcome) (contracts.ReviewOutcomeV1, error) {
	outcome = adaptation.CanonicalReviewOutcome(outcome)
	if err := adaptation.ValidateReviewOutcome(outcome); err != nil {
		return contracts.ReviewOutcomeV1{}, err
	}
	edits := make([]contracts.ReviewerEdit, 0, len(outcome.ReviewerEdits))
	for _, edit := range outcome.ReviewerEdits {
		edits = append(edits, contracts.ReviewerEdit{
			Path: edit.Path, PreviousPath: optionalPointer(edit.PreviousPath), Kind: string(edit.Kind),
			Additions: edit.Additions, Deletions: edit.Deletions, Patch: edit.Patch,
		})
	}
	document := contracts.ReviewOutcomeV1{
		APIVersion: outcome.APIVersion, OutcomeID: outcome.OutcomeID,
		ReviewID: outcome.ReviewID, ProposalID: outcome.ProposalID, ValidationID: outcome.ValidationID,
		Repository: exportRepository(outcome.Repository),
		PullRequest: contracts.OutcomePullRequest{
			Number: outcome.PullRequestNumber, URL: outcome.PullRequestURL,
		},
		Decision: string(outcome.Decision), Reason: contracts.ReviewOutcomeReason{
			Code: string(outcome.ReasonCode), Note: optionalPointer(outcome.ReasonNote),
		},
		GeneratedRevision: exportRevision(outcome.GeneratedRevision),
		FinalRevision:     exportRevision(outcome.FinalRevision),
		ClosedAt:          outcome.ClosedAt.Format(time.RFC3339Nano),
		MergedAt:          optionalTime(outcome.MergedAt),
		ObservedAt:        outcome.ObservedAt.Format(time.RFC3339Nano), ReviewerEdits: edits,
	}
	if err := contracts.ValidateReviewOutcomeV1(document); err != nil {
		return contracts.ReviewOutcomeV1{}, fmt.Errorf("export review outcome: %w", err)
	}

	return document, nil
}

func optionalPointer(value string) *string {
	if value == "" {
		return nil
	}
	copy := value

	return &copy
}

func optionalTime(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := value.Format(time.RFC3339Nano)

	return &formatted
}
