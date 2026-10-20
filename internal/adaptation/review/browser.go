package review

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/stanimirivanov/argus/internal/adaptation"
	"github.com/stanimirivanov/argus/internal/adaptation/browservalidation"
	"github.com/stanimirivanov/argus/internal/catalog"
)

// PublishBrowser opens a deterministic draft for the exact six-run validated
// locator edit. The source reader and publisher remain separate authorities.
func (service *Service) PublishBrowser(ctx context.Context, evidence browservalidation.Evidence, baseBranch string) (adaptation.ReviewPublication, error) {
	if service == nil || service.source == nil || service.publisher == nil {
		return adaptation.ReviewPublication{}, adaptation.ErrUnavailable
	}
	if err := browservalidation.ValidateEvidence(evidence); err != nil {
		return adaptation.ReviewPublication{}, err
	}
	if evidence.Status != browservalidation.Validated || adaptation.ValidateBranchName(baseBranch) != nil {
		return adaptation.ReviewPublication{}, adaptation.ErrInvalid
	}
	proposal := evidence.Proposal
	repository := evidence.Failed.Request.TestRepository
	revision := evidence.Failed.Request.TestRevision
	if repository.Identity.Provider != catalog.ProviderGitHub || !repository.Valid() {
		return adaptation.ReviewPublication{}, adaptation.ErrInvalid
	}
	original, err := service.source.LoadSource(ctx, repository, revision, proposal.Edit.Path)
	if err != nil {
		return adaptation.ReviewPublication{}, fmt.Errorf("load browser review source: %w", err)
	}
	candidate, err := materializeCandidate(original, proposal.Edit)
	if err != nil {
		return adaptation.ReviewPublication{}, err
	}
	if digest(candidate) != evidence.Source.CandidateSHA256 {
		return adaptation.ReviewPublication{}, fmt.Errorf("%w: browser candidate digest", adaptation.ErrInvalid)
	}
	id := DeriveBrowserReviewID(proposal.ProposalID, evidence.ValidationID, repository.Identity.ProviderRepositoryID, baseBranch)
	head := "argus/browser-locator-repair-" + proposal.ProposalID[:12]
	request := PublicationRequest{ReviewID: id, Repository: repository, BaseRevision: revision,
		BaseBranch: baseBranch, HeadBranch: head, Path: proposal.Edit.Path,
		OriginalSHA256: proposal.Edit.BeforeSHA256, Candidate: candidate,
		Title:         fmt.Sprintf("Repair %s/%s browser locator", proposal.SuiteKey, proposal.TestKey),
		Body:          browserReviewBody(id, evidence),
		CommitMessage: "Repair " + proposal.SuiteKey + "/" + proposal.TestKey + " browser locator"}
	published, err := service.publisher.Publish(ctx, request)
	if err != nil {
		return adaptation.ReviewPublication{}, fmt.Errorf("publish browser review: %w", err)
	}
	publication := adaptation.ReviewPublication{APIVersion: adaptation.ReviewAPIVersion,
		ReviewID: id, ProposalID: proposal.ProposalID, ValidationID: evidence.ValidationID,
		Repository: repository, Provider: adaptation.ReviewProviderGitHub, BaseBranch: baseBranch,
		BaseRevision: revision, HeadBranch: head, HeadRevision: published.HeadRevision,
		PullRequestNumber: published.Number, PullRequestURL: published.URL, Draft: published.Draft,
		State: published.State, PublishedAt: published.CreatedAt.UTC()}
	if err := adaptation.ValidateReviewPublication(publication); err != nil {
		return adaptation.ReviewPublication{}, err
	}

	return publication, nil
}

// DeriveBrowserReviewID distinguishes browser reviews from endpoint and field reviews.
func DeriveBrowserReviewID(proposalID, validationID, repositoryID, baseBranch string) string {
	hash := sha256.New()
	for _, value := range []string{"argus-browser-review-v1", proposalID, validationID, repositoryID, baseBranch} {
		_, _ = hash.Write([]byte(value))
		_, _ = hash.Write([]byte{0})
	}

	return hex.EncodeToString(hash.Sum(nil))
}

func browserReviewBody(id string, evidence browservalidation.Evidence) string {
	proposal := evidence.Proposal
	var body strings.Builder
	body.WriteString("## Argus validated browser locator repair\n\nThis draft changes one locator and requires human review. Argus does not authorize merge or visual baseline approval.\n\n")
	writeBodyField(&body, "Review", id)
	writeBodyField(&body, "Proposal", proposal.ProposalID)
	writeBodyField(&body, "Validation", evidence.ValidationID)
	writeBodyField(&body, "Test", proposal.SuiteKey+"/"+proposal.TestKey)
	writeBodyField(&body, "Source", proposal.Edit.Path)
	writeBodyField(&body, "Project", evidence.Failed.Request.Project)
	writeBodyField(&body, "Environment", evidence.Failed.Request.Environment)
	body.WriteString("\nSix runs: original failed twice, candidate passed twice, and negative control failed twice. Artifact references remain in the validation evidence. Assertions and navigation were unchanged.\n\n")
	body.WriteString("<!-- argus-review-id:" + id + " -->\n")

	return body.String()
}
