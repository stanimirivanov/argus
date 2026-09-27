// Package outcome captures terminal human-review evidence without treating provider state as correctness.
package outcome

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/stanimirivanov/argus/internal/adaptation"
	"github.com/stanimirivanov/argus/internal/catalog"
)

// TerminalReview is the complete provider observation after a pull request closes.
type TerminalReview struct {
	FinalRevision catalog.Revision
	ClosedAt      time.Time
	MergedAt      *time.Time
	ObservedAt    time.Time
	ReviewerEdits []adaptation.ReviewFileEdit
}

// Gateway resolves terminal provider state and the complete diff from the
// generated review head to the final reviewed head.
type Gateway interface {
	ObserveOutcome(context.Context, adaptation.ReviewPublication) (TerminalReview, error)
}

// Service correlates the complete repair chain and derives a terminal decision.
type Service struct {
	gateway Gateway
}

// NewService creates the review-outcome capture use case.
func NewService(gateway Gateway) *Service {
	return &Service{gateway: gateway}
}

// Capture derives outcome from provider state while retaining the explicit
// reviewer reason separately from the generated candidate and final diff.
func (service *Service) Capture(
	ctx context.Context,
	proposal adaptation.Proposal,
	evidence adaptation.ValidationEvidence,
	publication adaptation.ReviewPublication,
	reasonCode adaptation.ReviewReasonCode,
	reasonNote string,
) (adaptation.ReviewOutcome, error) {
	if service == nil || service.gateway == nil {
		return adaptation.ReviewOutcome{}, adaptation.ErrUnavailable
	}
	if err := correlate(proposal, evidence, publication); err != nil {
		return adaptation.ReviewOutcome{}, err
	}
	terminal, err := service.gateway.ObserveOutcome(ctx, publication)
	if err != nil {
		return adaptation.ReviewOutcome{}, fmt.Errorf("observe adaptation review outcome: %w", err)
	}
	decision := adaptation.ReviewRejected
	if terminal.MergedAt != nil {
		decision = adaptation.ReviewAcceptedAsProposed
		if len(terminal.ReviewerEdits) != 0 {
			decision = adaptation.ReviewAcceptedWithEdits
		}
	}
	outcome := adaptation.CanonicalReviewOutcome(adaptation.ReviewOutcome{
		APIVersion: adaptation.ReviewOutcomeAPIVersion,
		ReviewID:   publication.ReviewID, ProposalID: proposal.ProposalID,
		ValidationID: evidence.ValidationID, Repository: publication.Repository,
		PullRequestNumber: publication.PullRequestNumber, PullRequestURL: publication.PullRequestURL,
		Decision: decision, ReasonCode: reasonCode, ReasonNote: reasonNote,
		GeneratedRevision: publication.HeadRevision, FinalRevision: terminal.FinalRevision,
		ClosedAt: terminal.ClosedAt, MergedAt: terminal.MergedAt, ObservedAt: terminal.ObservedAt,
		ReviewerEdits: terminal.ReviewerEdits,
	})
	outcome.OutcomeID = deriveOutcomeID(outcome)
	if err := adaptation.ValidateReviewOutcome(outcome); err != nil {
		return adaptation.ReviewOutcome{}, err
	}

	return outcome, nil
}

func correlate(
	proposal adaptation.Proposal,
	evidence adaptation.ValidationEvidence,
	publication adaptation.ReviewPublication,
) error {
	if err := adaptation.ValidateProposal(proposal); err != nil {
		return err
	}
	if err := adaptation.ValidateValidationEvidence(evidence); err != nil {
		return err
	}
	if err := adaptation.ValidateReviewPublication(publication); err != nil {
		return err
	}
	if proposal.ProposalID != evidence.ProposalID || proposal.ProposalID != publication.ProposalID ||
		evidence.ValidationID != publication.ValidationID || proposal.PolicyVersion != evidence.ProposalPolicyVersion ||
		proposal.AdapterID != evidence.AdapterID || proposal.AdapterVersion != evidence.AdapterVersion ||
		proposal.Edit != evidence.Edit || !sameTest(proposal.Test, evidence.Test) ||
		proposal.Test.Repository != publication.Repository || proposal.Test.Revision != publication.BaseRevision {
		return fmt.Errorf("%w: proposal, validation, and review do not correlate", adaptation.ErrInvalid)
	}

	return nil
}

func deriveOutcomeID(outcome adaptation.ReviewOutcome) string {
	hash := sha256.New()
	writeHash := func(value string) {
		_, _ = hash.Write([]byte(value))
		_, _ = hash.Write([]byte{0})
	}
	for _, value := range []string{
		"argus-review-outcome-v1", outcome.ReviewID, outcome.ProposalID, outcome.ValidationID,
		string(outcome.Decision), string(outcome.ReasonCode), outcome.ReasonNote,
		string(outcome.FinalRevision.Algorithm), outcome.FinalRevision.Digest,
		outcome.ClosedAt.Format(time.RFC3339Nano),
	} {
		writeHash(value)
	}
	if outcome.MergedAt != nil {
		writeHash(outcome.MergedAt.Format(time.RFC3339Nano))
	} else {
		writeHash("")
	}
	for _, edit := range outcome.ReviewerEdits {
		for _, value := range []string{
			edit.Path, edit.PreviousPath, string(edit.Kind), strconv.Itoa(edit.Additions),
			strconv.Itoa(edit.Deletions), edit.Patch,
		} {
			writeHash(value)
		}
	}

	return hex.EncodeToString(hash.Sum(nil))
}

func sameTest(left, right adaptation.TestReference) bool {
	return left.Repository == right.Repository && left.Revision == right.Revision &&
		left.SuiteKey == right.SuiteKey && left.TestKey == right.TestKey && left.Name == right.Name &&
		left.Adapter == right.Adapter && slices.Equal(left.Capabilities, right.Capabilities)
}
