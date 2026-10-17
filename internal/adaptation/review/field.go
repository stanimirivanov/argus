package review

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html"
	"strings"

	"github.com/stanimirivanov/argus/internal/adaptation"
	"github.com/stanimirivanov/argus/internal/adaptation/fieldrepair"
	"github.com/stanimirivanov/argus/internal/adaptation/fieldvalidation"
	"github.com/stanimirivanov/argus/internal/catalog"
)

// PublishField opens a draft only for a repeated field-specific success proof.
func (service *Service) PublishField(ctx context.Context, proposal fieldrepair.Proposal, evidence fieldvalidation.Evidence, baseBranch string) (adaptation.ReviewPublication, error) {
	if service == nil || service.source == nil || service.publisher == nil {
		return adaptation.ReviewPublication{}, adaptation.ErrUnavailable
	}
	if err := correlateField(proposal, evidence, baseBranch); err != nil {
		return adaptation.ReviewPublication{}, err
	}
	original, err := service.source.LoadSource(ctx, proposal.Test.Repository, proposal.Test.Revision, proposal.Edit.Path)
	if err != nil {
		return adaptation.ReviewPublication{}, fmt.Errorf("load field review source: %w", err)
	}
	candidate, err := materializeCandidate(original, proposal.Edit)
	if err != nil {
		return adaptation.ReviewPublication{}, err
	}
	if digest(candidate) != evidence.Source.CandidateSHA256 {
		return adaptation.ReviewPublication{}, fmt.Errorf("%w: field candidate digest", adaptation.ErrInvalid)
	}
	id := deriveFieldReviewID(proposal.ProposalID, evidence.ValidationID, proposal.Test.Repository.Identity.ProviderRepositoryID, baseBranch)
	headBranch := "argus/field-repair-" + proposal.ProposalID[:12]
	request := PublicationRequest{ReviewID: id, Repository: proposal.Test.Repository, BaseRevision: proposal.Test.Revision,
		BaseBranch: baseBranch, HeadBranch: headBranch, Path: proposal.Edit.Path, OriginalSHA256: proposal.Edit.BeforeSHA256,
		Candidate: candidate, Title: fmt.Sprintf("Repair %s/%s %s JSON field", proposal.Test.SuiteKey, proposal.Test.TestKey, proposal.Rename.Direction),
		Body: fieldReviewBody(id, proposal, evidence), CommitMessage: "Repair " + proposal.Test.SuiteKey + "/" + proposal.Test.TestKey + " JSON field reference"}
	published, err := service.publisher.Publish(ctx, request)
	if err != nil {
		return adaptation.ReviewPublication{}, fmt.Errorf("publish field review: %w", err)
	}
	publication := adaptation.ReviewPublication{APIVersion: adaptation.ReviewAPIVersion, ReviewID: id, ProposalID: proposal.ProposalID,
		ValidationID: evidence.ValidationID, Repository: proposal.Test.Repository, Provider: adaptation.ReviewProviderGitHub,
		BaseBranch: baseBranch, BaseRevision: proposal.Test.Revision, HeadBranch: headBranch, HeadRevision: published.HeadRevision,
		PullRequestNumber: published.Number, PullRequestURL: published.URL, Draft: published.Draft, State: published.State,
		PublishedAt: published.CreatedAt.UTC()}
	if err := adaptation.ValidateReviewPublication(publication); err != nil {
		return adaptation.ReviewPublication{}, err
	}

	return publication, nil
}
func correlateField(proposal fieldrepair.Proposal, evidence fieldvalidation.Evidence, baseBranch string) error {
	if err := fieldrepair.ValidateProposal(proposal); err != nil {
		return err
	}
	if err := fieldvalidation.ValidateEvidence(evidence); err != nil {
		return err
	}
	if err := adaptation.ValidateBranchName(baseBranch); err != nil {
		return err
	}
	if proposal.Test.Repository.Identity.Provider != catalog.ProviderGitHub || proposal.ProposalID != evidence.ProposalID ||
		proposal.PolicyVersion != evidence.ProposalPolicyVersion || proposal.AdapterID != evidence.AdapterID ||
		proposal.AdapterVersion != evidence.AdapterVersion || proposal.Edit != evidence.Edit || !sameTest(proposal.Test, evidence.Test) {
		return fmt.Errorf("%w: field proposal and validation do not correlate", adaptation.ErrInvalid)
	}

	return nil
}
func deriveFieldReviewID(proposalID, validationID, repositoryID, baseBranch string) string {
	hash := sha256.New()
	for _, value := range []string{"argus-field-review-v1", proposalID, validationID, repositoryID, baseBranch} {
		_, _ = hash.Write([]byte(value))
		_, _ = hash.Write([]byte{0})
	}

	return hex.EncodeToString(hash.Sum(nil))
}
func fieldReviewBody(id string, proposal fieldrepair.Proposal, evidence fieldvalidation.Evidence) string {
	var body strings.Builder
	body.WriteString("## Argus validated JSON field repair\n\nThis draft changes one field reference and requires human review. Argus does not authorize merge.\n\n")
	writeBodyField(&body, "Review", id)
	writeBodyField(&body, "Proposal", proposal.ProposalID)
	writeBodyField(&body, "Validation", evidence.ValidationID)
	writeBodyField(&body, "Test", proposal.Test.SuiteKey+"/"+proposal.Test.TestKey)
	writeBodyField(&body, "Source", proposal.Edit.Path)
	writeBodyField(&body, "Field", proposal.Rename.Direction+" "+proposal.Rename.PreviousName+" → "+proposal.Rename.Name)
	body.WriteString("\n## Repeated validation gates\n\n| Phase | Repeat | Required | Observed | Source SHA-256 |\n|:--|--:|:--|:--|:--|\n")
	for _, run := range evidence.Runs {
		required := "fail"
		if run.Phase == fieldvalidation.Candidate {
			required = "pass"
		}
		fmt.Fprintf(&body, "| %s | %d | %s | %s | `%s` |\n", html.EscapeString(string(run.Phase)), run.Repeat,
			required, html.EscapeString(string(run.Outcome)), run.SourceSHA256)
	}
	body.WriteString("\nThe exact source was restored after each materialized run. The negative control changes only the field reference; assertions and expected values were unchanged.\n\n")
	body.WriteString("<!-- argus-review-id:")
	body.WriteString(id)
	body.WriteString(" -->\n")

	return body.String()
}
