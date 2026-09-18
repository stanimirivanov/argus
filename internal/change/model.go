// Package change owns the provider-neutral representation and invariants of a
// bounded source change between immutable revisions.
package change

import (
	"cmp"
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/stanimirivanov/argus/internal/catalog"
)

var repositoryHostPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$`)

const (
	// SetAPIVersion is the supported normalized change-set contract major.
	SetAPIVersion = "argus.dev/change-set/v1"
	// MaxFiles bounds one normalized change observation.
	MaxFiles = 1000
	// MaxPatchBytes bounds retained unified-diff evidence for one file.
	MaxPatchBytes = 64 * 1024
)

var (
	// ErrInvalid means a change observation violates domain invariants.
	ErrInvalid = errors.New("invalid change set")
	// ErrConflict means one immutable identity is associated with different
	// content.
	ErrConflict = errors.New("change ingestion identity conflict")
	// ErrNotFound means the requested delivery or provider change was not found.
	ErrNotFound = errors.New("change source repository not found")
	// ErrUnavailable means a dependency prevented a trustworthy result.
	ErrUnavailable = errors.New("change ingestion dependency unavailable")
	// ErrStale means the pull request advanced while its files were resolved.
	ErrStale = errors.New("change delivery is stale")
)

// Kind describes a normalized file operation.
type Kind string

const (
	// KindAdded creates a path.
	KindAdded Kind = "added"
	// KindModified changes content at an existing path.
	KindModified Kind = "modified"
	// KindDeleted removes a path.
	KindDeleted Kind = "deleted"
	// KindRenamed moves a path and may change its content.
	KindRenamed Kind = "renamed"
	// KindCopied creates a path from an existing path.
	KindCopied Kind = "copied"
)

// PatchStatus explains whether bounded patch evidence is present.
type PatchStatus string

const (
	// PatchComplete retains all patch text returned by the provider.
	PatchComplete PatchStatus = "complete"
	// PatchUnavailable means the provider omitted patch text, for example for a
	// binary file.
	PatchUnavailable PatchStatus = "unavailable"
	// PatchTruncated retains a prefix because the per-file bound was reached.
	PatchTruncated PatchStatus = "truncated"
	// PatchBudgetExhausted means the aggregate bundle bound was reached before
	// this patch could be retained.
	PatchBudgetExhausted PatchStatus = "budget-exhausted"
)

// Trigger identifies the verified provider delivery that caused ingestion.
type Trigger struct {
	Provider   catalog.Provider
	DeliveryID string
	Event      string
	Action     string
}

// File contains one normalized, bounded file observation. A nil Patch is
// distinct from an empty patch and its reason is carried by PatchStatus.
type File struct {
	Path         string
	PreviousPath *string
	Kind         Kind
	Additions    int
	Deletions    int
	Patch        *string
	PatchStatus  PatchStatus
}

// Set is a reproducible change observation at immutable base and head
// revisions. FilesTruncated prevents partial provider results from being
// mistaken for proof that no other files changed.
type Set struct {
	APIVersion        string
	SourceRepository  catalog.Repository
	PullRequestNumber int
	BaseRevision      catalog.Revision
	HeadRevision      catalog.Revision
	ObservedAt        time.Time
	Trigger           Trigger
	Files             []File
	FilesTruncated    bool
}

// CanonicalSet returns a deep copy ordered by stable repository path.
func CanonicalSet(set Set) Set {
	canonical := set
	canonical.ObservedAt = set.ObservedAt.UTC()
	canonical.Files = make([]File, len(set.Files))
	for index, file := range set.Files {
		canonical.Files[index] = file
		if file.PreviousPath != nil {
			previous := *file.PreviousPath
			canonical.Files[index].PreviousPath = &previous
		}
		if file.Patch != nil {
			patch := *file.Patch
			canonical.Files[index].Patch = &patch
		}
	}
	slices.SortFunc(canonical.Files, func(left, right File) int {
		if compared := cmp.Compare(left.Path, right.Path); compared != 0 {
			return compared
		}
		return cmp.Compare(stringValue(left.PreviousPath), stringValue(right.PreviousPath))
	})

	return canonical
}

// ValidateSet enforces semantic invariants not expressible in portable JSON
// Schema.
func ValidateSet(set Set) error {
	if set.APIVersion != SetAPIVersion {
		return fmt.Errorf("%w: unsupported API version", ErrInvalid)
	}
	if err := validateRepository(set.SourceRepository); err != nil {
		return fmt.Errorf("%w: source repository: %w", ErrInvalid, err)
	}
	if set.PullRequestNumber < 1 {
		return fmt.Errorf("%w: pull request number", ErrInvalid)
	}
	if err := validateRevision(set.BaseRevision); err != nil {
		return fmt.Errorf("%w: base revision", ErrInvalid)
	}
	if err := validateRevision(set.HeadRevision); err != nil {
		return fmt.Errorf("%w: head revision", ErrInvalid)
	}
	if set.BaseRevision == set.HeadRevision {
		return fmt.Errorf("%w: base and head revisions are equal", ErrInvalid)
	}
	if set.ObservedAt.IsZero() || !isUTC(set.ObservedAt) {
		return fmt.Errorf("%w: observedAt must be a UTC instant", ErrInvalid)
	}
	if err := validateTrigger(set.Trigger); err != nil {
		return err
	}
	if len(set.Files) > MaxFiles {
		return fmt.Errorf("%w: file count exceeds %d", ErrInvalid, MaxFiles)
	}

	seen := make(map[string]struct{}, len(set.Files))
	for _, file := range set.Files {
		if err := validateFile(file); err != nil {
			return err
		}
		if _, exists := seen[file.Path]; exists {
			return fmt.Errorf("%w: duplicate file path %q", ErrInvalid, file.Path)
		}
		seen[file.Path] = struct{}{}
	}

	return nil
}

func validateRepository(repository catalog.Repository) error {
	identity := repository.Identity
	if identity.Provider != catalog.ProviderGitHub || !repositoryHostPattern.MatchString(identity.Host) ||
		len(identity.Host) > 255 || strings.TrimSpace(identity.ProviderRepositoryID) == "" ||
		len(identity.ProviderRepositoryID) > 255 || strings.TrimSpace(repository.Owner) == "" ||
		len(repository.Owner) > 255 || strings.TrimSpace(repository.Name) == "" || len(repository.Name) > 255 {
		return errors.New("incomplete GitHub repository identity")
	}

	return nil
}

func validateRevision(revision catalog.Revision) error {
	validated, err := catalog.NewRevision(revision.Algorithm, revision.Digest)
	if err != nil || validated != revision {
		return errors.New("invalid immutable revision")
	}

	return nil
}

func validateTrigger(trigger Trigger) error {
	if trigger.Provider != catalog.ProviderGitHub || trigger.Event != "pull_request" ||
		strings.TrimSpace(trigger.DeliveryID) == "" || len(trigger.DeliveryID) > 255 {
		return fmt.Errorf("%w: trigger", ErrInvalid)
	}
	switch trigger.Action {
	case "opened", "reopened", "synchronize", "ready_for_review":
		return nil
	default:
		return fmt.Errorf("%w: trigger action", ErrInvalid)
	}
}

func validateFile(file File) error {
	if !validRepositoryPath(file.Path) || (file.PreviousPath != nil && !validRepositoryPath(*file.PreviousPath)) {
		return fmt.Errorf("%w: repository path", ErrInvalid)
	}
	switch file.Kind {
	case KindAdded, KindModified, KindDeleted:
		if file.PreviousPath != nil {
			return fmt.Errorf("%w: previous path is only valid for rename or copy", ErrInvalid)
		}
	case KindRenamed, KindCopied:
		if file.PreviousPath == nil || *file.PreviousPath == file.Path {
			return fmt.Errorf("%w: rename or copy previous path", ErrInvalid)
		}
	default:
		return fmt.Errorf("%w: file kind", ErrInvalid)
	}
	if file.Additions < 0 || file.Deletions < 0 {
		return fmt.Errorf("%w: negative line count", ErrInvalid)
	}
	if file.Patch != nil && (!utf8.ValidString(*file.Patch) || len(*file.Patch) > MaxPatchBytes) {
		return fmt.Errorf("%w: patch encoding or size", ErrInvalid)
	}
	switch file.PatchStatus {
	case PatchComplete, PatchTruncated:
		if file.Patch == nil {
			return fmt.Errorf("%w: patch status requires patch text", ErrInvalid)
		}
	case PatchUnavailable, PatchBudgetExhausted:
		if file.Patch != nil {
			return fmt.Errorf("%w: patch status forbids patch text", ErrInvalid)
		}
	default:
		return fmt.Errorf("%w: patch status", ErrInvalid)
	}

	return nil
}

func validRepositoryPath(value string) bool {
	return value != "" && len(value) <= 4096 && !strings.Contains(value, "\\") &&
		!strings.HasPrefix(value, "/") && path.Clean(value) == value && value != "." &&
		value != ".." && !strings.HasPrefix(value, "../") && !hasWindowsDrivePrefix(value)
}

func hasWindowsDrivePrefix(value string) bool {
	return len(value) >= 2 && ((value[0] >= 'a' && value[0] <= 'z') ||
		(value[0] >= 'A' && value[0] <= 'Z')) && value[1] == ':'
}

func isUTC(value time.Time) bool {
	_, offset := value.Zone()
	return offset == 0
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
