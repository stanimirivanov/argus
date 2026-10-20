package outcome

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/stanimirivanov/argus/internal/adaptation"
	"github.com/stanimirivanov/argus/internal/adaptation/browserlocator"
	"github.com/stanimirivanov/argus/internal/adaptation/browservalidation"
	"github.com/stanimirivanov/argus/internal/adaptation/review"
)

func digestBrowserSource(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// CaptureBrowser correlates the browser proof with terminal provider state.
// Edited acceptances require read-only verification of the complete final
// source; provider patches alone cannot prove that assertions were preserved.
func (service *Service) CaptureBrowser(ctx context.Context, evidence browservalidation.Evidence,
	publication adaptation.ReviewPublication, source review.SourceReader,
	reason adaptation.ReviewReasonCode, note string) (adaptation.ReviewOutcome, error) {
	if service == nil || service.gateway == nil || source == nil {
		return adaptation.ReviewOutcome{}, adaptation.ErrUnavailable
	}
	if err := browservalidation.ValidateEvidence(evidence); err != nil {
		return adaptation.ReviewOutcome{}, err
	}
	if err := adaptation.ValidateReviewPublication(publication); err != nil {
		return adaptation.ReviewOutcome{}, err
	}
	proposal := evidence.Proposal
	if evidence.Status != browservalidation.Validated || proposal.ProposalID != publication.ProposalID ||
		evidence.ValidationID != publication.ValidationID ||
		evidence.Failed.Request.TestRepository != publication.Repository ||
		evidence.Failed.Request.TestRevision != publication.BaseRevision ||
		publication.HeadBranch != "argus/browser-locator-repair-"+proposal.ProposalID[:12] ||
		publication.ReviewID != review.DeriveBrowserReviewID(proposal.ProposalID, evidence.ValidationID,
			publication.Repository.Identity.ProviderRepositoryID, publication.BaseBranch) {
		return adaptation.ReviewOutcome{}, fmt.Errorf("%w: browser validation and review do not correlate", adaptation.ErrInvalid)
	}
	result, err := service.captureTerminal(ctx, publication, reason, note)
	if err != nil {
		return adaptation.ReviewOutcome{}, err
	}
	if result.Decision != adaptation.ReviewAcceptedWithEdits {
		return result, nil
	}
	if len(result.ReviewerEdits) != 1 || result.ReviewerEdits[0].Path != proposal.Edit.Path ||
		result.ReviewerEdits[0].Kind != adaptation.ReviewFileModified {
		return adaptation.ReviewOutcome{}, fmt.Errorf("%w: browser review changed files outside the locator", adaptation.ErrInvalid)
	}
	generated, err := source.LoadSource(ctx, publication.Repository, publication.HeadRevision, proposal.Edit.Path)
	if err != nil {
		return adaptation.ReviewOutcome{}, fmt.Errorf("load generated browser source: %w", err)
	}
	final, err := source.LoadSource(ctx, publication.Repository, result.FinalRevision, proposal.Edit.Path)
	if err != nil {
		return adaptation.ReviewOutcome{}, fmt.Errorf("load final browser source: %w", err)
	}
	if len(generated) > adaptation.MaxSourceBytes || len(final) > adaptation.MaxSourceBytes ||
		digestBrowserSource(generated) != evidence.Source.CandidateSHA256 ||
		browserlocator.ValidateReviewerLocatorEdit(generated, final, proposal) != nil {
		return adaptation.ReviewOutcome{}, fmt.Errorf("%w: browser reviewer changed more than one locator", adaptation.ErrInvalid)
	}

	return result, nil
}
