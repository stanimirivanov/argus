package contract

import (
	"fmt"
	"time"

	"github.com/stanimirivanov/argus/internal/adaptation"
	"github.com/stanimirivanov/argus/internal/contracts"
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

// ImportReviewOutcomeV1 converts structurally valid terminal evidence to the
// domain model and verifies its semantic identity and decision invariants.
func ImportReviewOutcomeV1(document contracts.ReviewOutcomeV1) (adaptation.ReviewOutcome, error) {
	closedAt, err := time.Parse(time.RFC3339Nano, document.ClosedAt)
	if err != nil {
		return adaptation.ReviewOutcome{}, fmt.Errorf("parse review outcome close time: %w", err)
	}
	observedAt, err := time.Parse(time.RFC3339Nano, document.ObservedAt)
	if err != nil {
		return adaptation.ReviewOutcome{}, fmt.Errorf("parse review outcome observation time: %w", err)
	}
	mergedAt, err := importOptionalTime(document.MergedAt)
	if err != nil {
		return adaptation.ReviewOutcome{}, err
	}
	edits := make([]adaptation.ReviewFileEdit, 0, len(document.ReviewerEdits))
	for _, edit := range document.ReviewerEdits {
		edits = append(edits, adaptation.ReviewFileEdit{
			Path: edit.Path, PreviousPath: optionalValue(edit.PreviousPath),
			Kind: adaptation.ReviewFileKind(edit.Kind), Additions: edit.Additions,
			Deletions: edit.Deletions, Patch: edit.Patch,
		})
	}
	reviewOutcome := adaptation.CanonicalReviewOutcome(adaptation.ReviewOutcome{
		APIVersion: document.APIVersion, OutcomeID: document.OutcomeID,
		ReviewID: document.ReviewID, ProposalID: document.ProposalID, ValidationID: document.ValidationID,
		Repository: importRepository(document.Repository), PullRequestNumber: document.PullRequest.Number,
		PullRequestURL: document.PullRequest.URL, Decision: adaptation.ReviewDecision(document.Decision),
		ReasonCode:        adaptation.ReviewReasonCode(document.Reason.Code),
		ReasonNote:        optionalValue(document.Reason.Note),
		GeneratedRevision: importRevision(document.GeneratedRevision),
		FinalRevision:     importRevision(document.FinalRevision), ClosedAt: closedAt,
		MergedAt: mergedAt, ObservedAt: observedAt, ReviewerEdits: edits,
	})
	if err := adaptation.ValidateReviewOutcome(reviewOutcome); err != nil {
		return adaptation.ReviewOutcome{}, fmt.Errorf("validate imported review outcome: %w", err)
	}

	return reviewOutcome, nil
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

func importOptionalTime(value *string) (*time.Time, error) {
	if value == nil {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, *value)
	if err != nil {
		return nil, fmt.Errorf("parse review outcome merge time: %w", err)
	}

	return &parsed, nil
}

func optionalValue(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}
