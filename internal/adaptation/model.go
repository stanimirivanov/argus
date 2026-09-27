// Package adaptation owns framework-neutral, review-first test repair proposals.
package adaptation

import (
	"cmp"
	"errors"
	"fmt"
	pathpkg "path"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
)

const (
	// RequestAPIVersion identifies the process-adapter request contract.
	RequestAPIVersion = "argus.dev/functional-api-adaptation-request/v1"
	// ResultAPIVersion identifies untrusted process-adapter output.
	ResultAPIVersion = "argus.dev/functional-api-adaptation-result/v1"
	// ProposalAPIVersion identifies the reviewable Argus proposal contract.
	ProposalAPIVersion = "argus.dev/adaptation-proposal/v1"
	// PolicyVersion identifies the initial endpoint-rename-only policy.
	PolicyVersion = "argus.dev/adaptation-policy/functional-api-endpoint-rename/v1"
	// MaxSourcePathLength bounds adapter-selected repository paths.
	MaxSourcePathLength = 4096
	// MaxSourceBytes bounds one source file materialized for validation.
	MaxSourceBytes = 16 << 20
)

var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

var (
	// ErrInvalid means an adaptation value violates domain invariants.
	ErrInvalid = errors.New("invalid adaptation")
	// ErrUnavailable means a configured adaptation dependency is unavailable.
	ErrUnavailable = errors.New("adaptation dependency unavailable")
	// ErrNoAuthoritativeRename means evidence cannot prove one safe rename.
	ErrNoAuthoritativeRename = errors.New("no authoritative endpoint rename")
	// ErrAdapterAbstained means the framework adapter could not prove one safe
	// request-target occurrence.
	ErrAdapterAbstained = errors.New("adaptation adapter abstained")
	// ErrValidationRejected means the candidate did not satisfy the required
	// original, repaired, and negative-control outcomes.
	ErrValidationRejected = errors.New("adaptation validation rejected")
	// ErrReviewConflict means a deterministic review branch or pull request
	// exists but no longer represents the validated candidate.
	ErrReviewConflict = errors.New("adaptation review conflict")
	// ErrReviewNotFinal means a review is still open and cannot yet provide a
	// terminal learning outcome.
	ErrReviewNotFinal = errors.New("adaptation review is not final")
	// ErrReviewEvidenceIncomplete means provider evidence cannot describe the
	// complete final reviewer-authored diff within policy bounds.
	ErrReviewEvidenceIncomplete = errors.New("adaptation review evidence incomplete")
)

const (
	// ReviewAPIVersion identifies a published, review-first repair reference.
	ReviewAPIVersion = "argus.dev/adaptation-review/v1"
	// ReviewProviderGitHub identifies the first supported review destination.
	ReviewProviderGitHub = "github"
	// ReviewOutcomeAPIVersion identifies terminal structured review evidence.
	ReviewOutcomeAPIVersion = "argus.dev/review-outcome/v1"
)

// Classification describes the observed state of one test.
type Classification string

const (
	// ClassificationInvalidated means authoritative change evidence made the
	// test's request target stale.
	ClassificationInvalidated Classification = "INVALIDATED"
)

// Decision states the next controlled action for a proposal.
type Decision string

const (
	// DecisionPatchAndValidate permits an isolated patch and validation run; it
	// does not authorize a commit, pull request, merge, or deployment.
	DecisionPatchAndValidate Decision = "PATCH_AND_VALIDATE"
)

// EndpointRename is a unique operation-identity-preserving path change derived
// from a complete semantic OpenAPI impact.
type EndpointRename struct {
	Method       string
	OperationID  string
	PreviousPath string
	Path         string
	Capabilities []string
}

// TestReference identifies the exact catalog test an adapter may inspect.
type TestReference struct {
	Repository   catalog.Repository
	Revision     catalog.Revision
	SuiteKey     string
	TestKey      string
	Name         string
	Adapter      string
	Capabilities []string
}

// AdapterRequest asks reviewed repository code to locate one request-target
// occurrence. It carries evidence, never an executable command.
type AdapterRequest struct {
	APIVersion string
	ProposalID string
	Change     change.Reference
	Test       TestReference
	Rename     EndpointRename
}

// TextEdit is one byte-addressed replacement against immutable source bytes.
type TextEdit struct {
	Path         string
	BeforeSHA256 string
	StartByte    int
	EndByte      int
	Original     string
	Replacement  string
	SemanticRole string
}

// AdapterResult is untrusted output returned by a reviewed framework adapter.
// The first policy accepts exactly one request-target edit or an abstention.
type AdapterResult struct {
	APIVersion     string
	ProposalID     string
	AdapterID      string
	AdapterVersion string
	Outcome        string
	ReasonCode     string
	Reason         string
	Edit           *TextEdit
}

// Proposal is a constrained, reviewable candidate. It is deliberately not a
// mutation instruction and contains no authorization to change a checkout.
type Proposal struct {
	APIVersion            string
	PolicyVersion         string
	ProposalID            string
	ImpactAPIVersion      string
	ImpactAnalyzerVersion string
	Change                change.Reference
	Test                  TestReference
	Classification        Classification
	Decision              Decision
	Rename                EndpointRename
	AdapterID             string
	AdapterVersion        string
	Edit                  TextEdit
}

// ReviewPublication records the externally visible draft created from one
// validated proposal. It is evidence of review handoff, never merge approval.
type ReviewPublication struct {
	APIVersion        string
	ReviewID          string
	ProposalID        string
	ValidationID      string
	Repository        catalog.Repository
	Provider          string
	BaseBranch        string
	BaseRevision      catalog.Revision
	HeadBranch        string
	HeadRevision      catalog.Revision
	PullRequestNumber int
	PullRequestURL    string
	Draft             bool
	State             string
	PublishedAt       time.Time
}

// CanonicalEndpointRename deep-copies and orders set-like evidence.
func CanonicalEndpointRename(rename EndpointRename) EndpointRename {
	canonical := rename
	canonical.Capabilities = canonicalStrings(rename.Capabilities)

	return canonical
}

// CanonicalTestReference deep-copies and orders set-like metadata.
func CanonicalTestReference(test TestReference) TestReference {
	canonical := test
	canonical.Capabilities = canonicalStrings(test.Capabilities)

	return canonical
}

// ValidateAdapterRequest enforces the request independently of its transport.
func ValidateAdapterRequest(request AdapterRequest) error {
	if request.APIVersion != RequestAPIVersion || !sha256Pattern.MatchString(request.ProposalID) {
		return fmt.Errorf("%w: adapter request envelope", ErrInvalid)
	}
	if err := validateChangeReference(request.Change); err != nil {
		return err
	}
	if err := ValidateTestReference(request.Test); err != nil {
		return err
	}

	return ValidateEndpointRename(request.Rename)
}

// ValidateAdapterResult checks structural domain rules before correlation to a
// request. Candidate edit semantics are checked by the application service.
func ValidateAdapterResult(result AdapterResult) error {
	if result.APIVersion != ResultAPIVersion || !sha256Pattern.MatchString(result.ProposalID) ||
		!catalog.IsLocalKey(result.AdapterID) || strings.TrimSpace(result.AdapterVersion) == "" ||
		len(result.AdapterVersion) > 127 {
		return fmt.Errorf("%w: adapter result envelope", ErrInvalid)
	}
	switch result.Outcome {
	case "candidate":
		if result.Edit == nil || result.ReasonCode != "" || result.Reason != "" {
			return fmt.Errorf("%w: candidate result", ErrInvalid)
		}
		return validateTextEdit(*result.Edit)
	case "abstained":
		if result.Edit != nil || !catalog.IsLocalKey(result.ReasonCode) ||
			strings.TrimSpace(result.Reason) == "" || len(result.Reason) > 1000 {
			return fmt.Errorf("%w: abstained result", ErrInvalid)
		}

		return nil
	default:
		return fmt.Errorf("%w: adapter outcome", ErrInvalid)
	}
}

// ValidateProposal protects the boundary consumed by validation and review.
func ValidateProposal(proposal Proposal) error {
	if proposal.APIVersion != ProposalAPIVersion || proposal.PolicyVersion != PolicyVersion ||
		!sha256Pattern.MatchString(proposal.ProposalID) || proposal.ImpactAPIVersion != change.ImpactAPIVersion ||
		proposal.ImpactAnalyzerVersion != change.OpenAPIAnalyzerVersion ||
		proposal.Classification != ClassificationInvalidated || proposal.Decision != DecisionPatchAndValidate ||
		!catalog.IsLocalKey(proposal.AdapterID) || strings.TrimSpace(proposal.AdapterVersion) == "" ||
		len(proposal.AdapterVersion) > 127 {
		return fmt.Errorf("%w: proposal envelope", ErrInvalid)
	}
	if err := validateChangeReference(proposal.Change); err != nil {
		return err
	}
	if err := ValidateTestReference(proposal.Test); err != nil {
		return err
	}
	if err := ValidateEndpointRename(proposal.Rename); err != nil {
		return err
	}
	if err := validateTextEdit(proposal.Edit); err != nil {
		return err
	}
	if proposal.AdapterID != proposal.Test.Adapter || proposal.Edit.Original != proposal.Rename.PreviousPath ||
		proposal.Edit.Replacement != proposal.Rename.Path || proposal.Edit.SemanticRole != "request-target" {
		return fmt.Errorf("%w: proposal edit policy", ErrInvalid)
	}

	return nil
}

// ValidateEndpointRename enforces the proof retained in a proposal.
func ValidateEndpointRename(rename EndpointRename) error {
	if !validHTTPMethod(rename.Method) || strings.TrimSpace(rename.OperationID) == "" ||
		rename.PreviousPath == rename.Path ||
		!strings.HasPrefix(rename.PreviousPath, "/") || !strings.HasPrefix(rename.Path, "/") ||
		len(rename.PreviousPath) > 2048 || len(rename.Path) > 2048 || len(rename.OperationID) > 255 ||
		len(rename.Capabilities) == 0 || len(rename.Capabilities) > change.MaxOperationCapabilities {
		return fmt.Errorf("%w: endpoint rename", ErrInvalid)
	}
	if !slices.Equal(rename.Capabilities, canonicalStrings(rename.Capabilities)) {
		return fmt.Errorf("%w: non-canonical endpoint rename", ErrInvalid)
	}
	for _, capability := range rename.Capabilities {
		if !catalog.IsLocalKey(capability) {
			return fmt.Errorf("%w: endpoint capability", ErrInvalid)
		}
	}

	return nil
}

// ValidateTestReference verifies immutable test identity and display metadata.
func ValidateTestReference(test TestReference) error {
	identity := catalog.TestIdentity{
		TestRepository: test.Repository.Identity, SuiteKey: test.SuiteKey, TestKey: test.TestKey,
	}
	if !identity.Valid() || strings.TrimSpace(test.Repository.Owner) == "" ||
		strings.TrimSpace(test.Repository.Name) == "" || strings.TrimSpace(test.Name) == "" ||
		!catalog.IsLocalKey(test.Adapter) || len(test.Capabilities) == 0 || len(test.Capabilities) > 5000 ||
		len(test.Repository.Owner) > 255 || len(test.Repository.Name) > 255 || len(test.Name) > 255 {
		return fmt.Errorf("%w: test reference", ErrInvalid)
	}
	if normalized, err := catalog.NewRevision(test.Revision.Algorithm, test.Revision.Digest); err != nil ||
		normalized != test.Revision {
		return fmt.Errorf("%w: test revision", ErrInvalid)
	}
	if !slices.Equal(test.Capabilities, canonicalStrings(test.Capabilities)) {
		return fmt.Errorf("%w: non-canonical test reference", ErrInvalid)
	}
	for _, capability := range test.Capabilities {
		if !catalog.IsLocalKey(capability) {
			return fmt.Errorf("%w: test capability", ErrInvalid)
		}
	}

	return nil
}

func validateTextEdit(edit TextEdit) error {
	if !validRepositoryPath(edit.Path) || !sha256Pattern.MatchString(edit.BeforeSHA256) ||
		edit.StartByte < 0 || edit.EndByte <= edit.StartByte || edit.Original == "" ||
		edit.Replacement == "" || edit.Original == edit.Replacement || edit.SemanticRole != "request-target" ||
		edit.EndByte-edit.StartByte != len([]byte(edit.Original)) {
		return fmt.Errorf("%w: text edit", ErrInvalid)
	}

	return nil
}

func validateChangeReference(reference change.Reference) error {
	return change.ValidateSet(change.Set{
		APIVersion: change.SetAPIVersion, SourceRepository: reference.SourceRepository,
		PullRequestNumber: reference.PullRequestNumber, BaseRevision: reference.BaseRevision,
		HeadRevision: reference.HeadRevision, ObservedAt: reference.ObservedAt, Trigger: reference.Trigger,
	})
}

func validRepositoryPath(path string) bool {
	cleaned := strings.ReplaceAll(path, `\`, "/")
	return path == cleaned && path != "" && path != "." && path == pathpkg.Clean(path) &&
		len(path) <= MaxSourcePathLength && !strings.HasPrefix(path, "/") &&
		!strings.HasPrefix(path, "../") && !strings.ContainsRune(path, '\x00')
}

// ValidateSourcePath verifies a normalized repository-relative source path for
// workspace adapters.
func ValidateSourcePath(path string) error {
	if !validRepositoryPath(path) {
		return fmt.Errorf("%w: source path", ErrInvalid)
	}

	return nil
}

func canonicalStrings(values []string) []string {
	result := append([]string{}, values...)
	slices.Sort(result)

	return slices.Compact(result)
}

func validHTTPMethod(method string) bool {
	switch method {
	case "GET", "PUT", "POST", "DELETE", "OPTIONS", "HEAD", "PATCH", "TRACE", "QUERY":
		return true
	default:
		return false
	}
}

// CompareEndpointRenames provides deterministic ordering for diagnostics and tests.
func CompareEndpointRenames(left, right EndpointRename) int {
	if compared := cmp.Compare(left.OperationID, right.OperationID); compared != 0 {
		return compared
	}
	if compared := cmp.Compare(left.Method, right.Method); compared != 0 {
		return compared
	}
	if compared := cmp.Compare(left.PreviousPath, right.PreviousPath); compared != 0 {
		return compared
	}

	return cmp.Compare(left.Path, right.Path)
}
