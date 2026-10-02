// Package adaptation owns framework-neutral test repair evidence and invariants.
package adaptation

import (
	"errors"
	"fmt"
	pathpkg "path"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
)

const (
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
	// ErrOutcomeConflict means one review identity was reused with different
	// immutable terminal evidence.
	ErrOutcomeConflict = errors.New("adaptation review outcome conflict")
	// ErrOutcomeNotFound means no durable review outcome has the requested identity.
	ErrOutcomeNotFound = errors.New("adaptation review outcome not found")
	// ErrValidationRejectionConflict means one validation identity was reused
	// with different trustworthy rejection evidence.
	ErrValidationRejectionConflict = errors.New("adaptation validation rejection conflict")
	// ErrValidationRejectionNotFound means no durable validation rejection has
	// the requested validation identity.
	ErrValidationRejectionNotFound = errors.New("adaptation validation rejection not found")
)

const (
	// ReviewAPIVersion identifies a published, review-first repair reference.
	ReviewAPIVersion = "argus.dev/adaptation-review/v1"
	// ReviewProviderGitHub identifies the first supported review destination.
	ReviewProviderGitHub = "github"
	// ReviewOutcomeAPIVersion identifies terminal structured review evidence.
	ReviewOutcomeAPIVersion = "argus.dev/review-outcome/v1"
)

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

// CanonicalTestReference deep-copies and orders set-like metadata.
func CanonicalTestReference(test TestReference) TestReference {
	canonical := test
	canonical.Capabilities = canonicalStrings(test.Capabilities)
	return canonical
}

// ValidateTestReference verifies immutable test identity and display metadata.
func ValidateTestReference(test TestReference) error {
	identity := catalog.TestIdentity{
		TestRepository: test.Repository.Identity, SuiteKey: test.SuiteKey, TestKey: test.TestKey,
	}
	if !identity.Valid() || !test.Repository.Valid() || strings.TrimSpace(test.Name) == "" ||
		!catalog.IsLocalKey(test.Adapter) || len(test.Capabilities) == 0 || len(test.Capabilities) > 5000 ||
		len(test.Name) > 255 {
		return fmt.Errorf("%w: test reference", ErrInvalid)
	}
	if !test.Revision.Valid() {
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

// ValidateTextEdit checks transport-independent byte-span and path invariants.
// A test-family policy must separately constrain SemanticRole and replacement.
func ValidateTextEdit(edit TextEdit) error {
	if !validRepositoryPath(edit.Path) || !sha256Pattern.MatchString(edit.BeforeSHA256) ||
		edit.StartByte < 0 || edit.EndByte <= edit.StartByte || edit.Original == "" ||
		edit.Replacement == "" || edit.Original == edit.Replacement ||
		edit.EndByte-edit.StartByte != len([]byte(edit.Original)) {
		return fmt.Errorf("%w: text edit", ErrInvalid)
	}

	return nil
}

func validRepositoryPath(path string) bool {
	cleaned := strings.ReplaceAll(path, `\`, "/")
	return path == cleaned && path != "" && path != "." && path == pathpkg.Clean(path) &&
		len(path) <= MaxSourcePathLength && !strings.HasPrefix(path, "/") &&
		!strings.HasPrefix(path, "../") && !strings.ContainsAny(path, ":\x00")
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
