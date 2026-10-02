// Package review publishes validated repairs through a review-only provider boundary.
package review

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/stanimirivanov/argus/internal/adaptation"
	"github.com/stanimirivanov/argus/internal/catalog"
)

const reviewBranchPrefix = "argus/endpoint-repair-"

// PublicationRequest is the complete provider-neutral write instruction. The
// gateway must use OriginalSHA256 as an optimistic-concurrency precondition.
type PublicationRequest struct {
	ReviewID       string
	Repository     catalog.Repository
	BaseRevision   catalog.Revision
	BaseBranch     string
	HeadBranch     string
	Path           string
	OriginalSHA256 string
	Candidate      []byte
	Title          string
	Body           string
	CommitMessage  string
}

// PublishedPullRequest is the provider acknowledgement required to emit a
// public review reference.
type PublishedPullRequest struct {
	Number       int
	URL          string
	HeadRevision catalog.Revision
	Draft        bool
	State        string
	CreatedAt    time.Time
}

// SourceReader reads immutable source using read-only provider authority.
type SourceReader interface {
	LoadSource(context.Context, catalog.Repository, catalog.Revision, string) ([]byte, error)
}

// Publisher owns externally visible review writes and provider-specific
// idempotency and partial-failure recovery.
type Publisher interface {
	Publish(context.Context, PublicationRequest) (PublishedPullRequest, error)
}

// Service correlates validation evidence, materializes the exact candidate,
// and delegates one guarded draft publication.
type Service struct {
	source    SourceReader
	publisher Publisher
}

// NewService creates the review-publication use case.
func NewService(source SourceReader, publisher Publisher) *Service {
	return &Service{source: source, publisher: publisher}
}

// Publish opens or recovers the deterministic draft pull request for a
// validated proposal. An exact retry returns the same provider review.
func (service *Service) Publish(
	ctx context.Context,
	proposal adaptation.Proposal,
	evidence adaptation.ValidationEvidence,
	baseBranch string,
) (adaptation.ReviewPublication, error) {
	if service == nil || service.source == nil || service.publisher == nil {
		return adaptation.ReviewPublication{}, adaptation.ErrUnavailable
	}
	if err := validateInputs(proposal, evidence, baseBranch); err != nil {
		return adaptation.ReviewPublication{}, err
	}
	original, err := service.source.LoadSource(ctx, proposal.Test.Repository, proposal.Test.Revision, proposal.Edit.Path)
	if err != nil {
		return adaptation.ReviewPublication{}, fmt.Errorf("load review source: %w", err)
	}
	candidate, err := materializeCandidate(original, proposal.Edit)
	if err != nil {
		return adaptation.ReviewPublication{}, err
	}
	if digest(candidate) != evidence.Source.CandidateSHA256 {
		return adaptation.ReviewPublication{}, fmt.Errorf("%w: candidate source digest", adaptation.ErrInvalid)
	}
	reviewID := deriveReviewID(proposal, evidence, baseBranch)
	headBranch := reviewBranchPrefix + proposal.ProposalID[:12]
	request := PublicationRequest{
		ReviewID: reviewID, Repository: proposal.Test.Repository, BaseRevision: proposal.Test.Revision,
		BaseBranch: baseBranch, HeadBranch: headBranch, Path: proposal.Edit.Path,
		OriginalSHA256: proposal.Edit.BeforeSHA256, Candidate: candidate,
		Title: reviewTitle(proposal), Body: reviewBody(reviewID, proposal, evidence),
		CommitMessage: "Repair " + proposal.Test.SuiteKey + "/" + proposal.Test.TestKey + " endpoint reference",
	}
	published, err := service.publisher.Publish(ctx, request)
	if err != nil {
		return adaptation.ReviewPublication{}, fmt.Errorf("publish adaptation review: %w", err)
	}
	publication := adaptation.ReviewPublication{
		APIVersion: adaptation.ReviewAPIVersion, ReviewID: reviewID, ProposalID: proposal.ProposalID,
		ValidationID: evidence.ValidationID, Repository: proposal.Test.Repository,
		Provider: adaptation.ReviewProviderGitHub, BaseBranch: baseBranch, BaseRevision: proposal.Test.Revision,
		HeadBranch: headBranch, HeadRevision: published.HeadRevision, PullRequestNumber: published.Number,
		PullRequestURL: published.URL, Draft: published.Draft, State: published.State,
		PublishedAt: published.CreatedAt.UTC(),
	}
	if err := adaptation.ValidateReviewPublication(publication); err != nil {
		return adaptation.ReviewPublication{}, fmt.Errorf("validate provider review: %w", err)
	}

	return publication, nil
}

func validateInputs(
	proposal adaptation.Proposal,
	evidence adaptation.ValidationEvidence,
	baseBranch string,
) error {
	if err := adaptation.ValidateProposal(proposal); err != nil {
		return err
	}
	if err := adaptation.ValidateValidationEvidence(evidence); err != nil {
		return err
	}
	if err := adaptation.ValidateBranchName(baseBranch); err != nil {
		return err
	}
	if proposal.Test.Repository.Identity.Provider != catalog.ProviderGitHub ||
		proposal.ProposalID != evidence.ProposalID || proposal.PolicyVersion != evidence.ProposalPolicyVersion ||
		proposal.AdapterID != evidence.AdapterID || proposal.AdapterVersion != evidence.AdapterVersion ||
		proposal.Edit != evidence.Edit || !sameTest(proposal.Test, evidence.Test) {
		return fmt.Errorf("%w: proposal and validation evidence do not correlate", adaptation.ErrInvalid)
	}

	return nil
}

func materializeCandidate(original []byte, edit adaptation.TextEdit) ([]byte, error) {
	if len(original) > adaptation.MaxSourceBytes || digest(original) != edit.BeforeSHA256 ||
		edit.StartByte < 0 || edit.EndByte > len(original) ||
		!bytes.Equal(original[edit.StartByte:edit.EndByte], []byte(edit.Original)) {
		return nil, fmt.Errorf("%w: review source preimage", adaptation.ErrInvalid)
	}
	candidate := make([]byte, 0, len(original)-len(edit.Original)+len(edit.Replacement))
	candidate = append(candidate, original[:edit.StartByte]...)
	candidate = append(candidate, edit.Replacement...)
	candidate = append(candidate, original[edit.EndByte:]...)
	if len(candidate) > adaptation.MaxSourceBytes {
		return nil, fmt.Errorf("%w: review candidate exceeds source limit", adaptation.ErrInvalid)
	}

	return candidate, nil
}

func deriveReviewID(
	proposal adaptation.Proposal,
	evidence adaptation.ValidationEvidence,
	baseBranch string,
) string {
	hash := sha256.New()
	for _, value := range []string{
		"argus-review-v1", proposal.ProposalID, evidence.ValidationID,
		proposal.Test.Repository.Identity.ProviderRepositoryID, baseBranch,
	} {
		_, _ = hash.Write([]byte(value))
		_, _ = hash.Write([]byte{0})
	}

	return hex.EncodeToString(hash.Sum(nil))
}

func reviewTitle(proposal adaptation.Proposal) string {
	value := fmt.Sprintf(
		"Repair %s/%s for %s %s",
		proposal.Test.SuiteKey, proposal.Test.TestKey, proposal.Rename.Method, proposal.Rename.Path,
	)
	value = strings.Map(func(character rune) rune {
		if unicode.IsControl(character) {
			return -1
		}
		return character
	}, value)
	if len(value) > 240 {
		return value[:240]
	}

	return value
}

func reviewBody(
	reviewID string,
	proposal adaptation.Proposal,
	evidence adaptation.ValidationEvidence,
) string {
	var builder strings.Builder
	builder.WriteString("## Argus validated repair\n\n")
	builder.WriteString("This draft changes one request target and requires human review. Argus does not authorize merge.\n\n")
	writeBodyField(&builder, "Review", reviewID)
	writeBodyField(&builder, "Proposal", proposal.ProposalID)
	writeBodyField(&builder, "Validation", evidence.ValidationID)
	writeBodyField(&builder, "Test", proposal.Test.SuiteKey+"/"+proposal.Test.TestKey)
	writeBodyField(&builder, "Source", proposal.Edit.Path)
	writeBodyField(&builder, "Endpoint", proposal.Rename.Method+" "+proposal.Rename.PreviousPath+" → "+proposal.Rename.Path)
	builder.WriteString("\n## Validation gates\n\n")
	builder.WriteString("| Phase | Required | Observed | Source SHA-256 |\n|:--|:--|:--|:--|\n")
	for _, run := range evidence.Runs {
		required := "fail"
		if run.Phase == adaptation.ValidationCandidate {
			required = "pass"
		}
		fmt.Fprintf(
			&builder, "| %s | %s | %s | `%s` |\n",
			html.EscapeString(string(run.Phase)), required, html.EscapeString(string(run.Outcome)), run.SourceSHA256,
		)
	}
	builder.WriteString("\nThe unchanged test failed, the exact candidate passed, and a deterministic invalid endpoint failed. ")
	builder.WriteString("The source was restored and verified after validation.\n\n")
	builder.WriteString("<!-- argus-review-id:")
	builder.WriteString(reviewID)
	builder.WriteString(" -->\n")

	return builder.String()
}

func writeBodyField(builder *strings.Builder, name, value string) {
	fmt.Fprintf(builder, "- **%s:** `%s`\n", name, strings.ReplaceAll(html.EscapeString(value), "`", "&#96;"))
}

func sameTest(left, right adaptation.TestReference) bool {
	return left.Repository == right.Repository && left.Revision == right.Revision &&
		left.SuiteKey == right.SuiteKey && left.TestKey == right.TestKey && left.Name == right.Name &&
		left.Adapter == right.Adapter && slices.Equal(left.Capabilities, right.Capabilities)
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:])
}
