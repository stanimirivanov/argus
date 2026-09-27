package adaptation

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
)

const (
	// MaxReviewerEditFiles bounds one terminal review comparison.
	MaxReviewerEditFiles = 100
	// MaxReviewerPatchBytes bounds one file patch retained as learning evidence.
	MaxReviewerPatchBytes = 64 << 10
	// MaxReviewerTotalPatchBytes bounds all retained patches for one review.
	MaxReviewerTotalPatchBytes = 2 << 20
)

// ReviewDecision describes the terminal disposition derived from provider
// state and the diff after Argus' generated commit.
type ReviewDecision string

const (
	// ReviewAcceptedAsProposed means the generated tree was merged unchanged.
	ReviewAcceptedAsProposed ReviewDecision = "accepted-as-proposed"
	// ReviewAcceptedWithEdits means the PR merged after additional file changes.
	ReviewAcceptedWithEdits ReviewDecision = "accepted-with-edits"
	// ReviewRejected means the PR closed without merge.
	ReviewRejected ReviewDecision = "rejected"
)

// ReviewReasonCode is an explicit human classification used for later
// adaptation evaluation. It does not replace the provider-derived decision.
type ReviewReasonCode string

const (
	// ReviewReasonApproved confirms an unchanged generated candidate.
	ReviewReasonApproved ReviewReasonCode = "approved"
	// ReviewReasonCorrected records that reviewer edits were needed before merge.
	ReviewReasonCorrected ReviewReasonCode = "corrected"
	// ReviewReasonIncorrectRepair rejects a candidate that did not repair the intended behavior.
	ReviewReasonIncorrectRepair ReviewReasonCode = "incorrect-repair"
	// ReviewReasonUnsafeRepair rejects a candidate that could weaken intent or safety.
	ReviewReasonUnsafeRepair ReviewReasonCode = "unsafe-repair"
	// ReviewReasonNoLongerNeeded rejects a repair made unnecessary by another change.
	ReviewReasonNoLongerNeeded ReviewReasonCode = "no-longer-needed"
	// ReviewReasonSuperseded rejects a review replaced by another implementation.
	ReviewReasonSuperseded ReviewReasonCode = "superseded"
	// ReviewReasonOther retains a bounded explanation for an unclassified rejection.
	ReviewReasonOther ReviewReasonCode = "other"
)

// ReviewFileKind is the normalized provider change kind after the generated commit.
type ReviewFileKind string

const (
	// ReviewFileAdded records a new file.
	ReviewFileAdded ReviewFileKind = "added"
	// ReviewFileModified records changes to an existing file.
	ReviewFileModified ReviewFileKind = "modified"
	// ReviewFileDeleted records a removed file.
	ReviewFileDeleted ReviewFileKind = "deleted"
	// ReviewFileRenamed records a path change and its previous path.
	ReviewFileRenamed ReviewFileKind = "renamed"
	// ReviewFileCopied records a new path copied from provider history.
	ReviewFileCopied ReviewFileKind = "copied"
)

// ReviewFileEdit retains one complete bounded unified patch introduced after
// Argus published the generated candidate.
type ReviewFileEdit struct {
	Path         string
	PreviousPath string
	Kind         ReviewFileKind
	Additions    int
	Deletions    int
	Patch        string
}

// ReviewOutcome is immutable terminal learning evidence. The decision is
// derived from provider state; the reason code is explicitly supplied by the reviewer workflow.
type ReviewOutcome struct {
	APIVersion        string
	OutcomeID         string
	ReviewID          string
	ProposalID        string
	ValidationID      string
	Repository        catalog.Repository
	PullRequestNumber int
	PullRequestURL    string
	Decision          ReviewDecision
	ReasonCode        ReviewReasonCode
	ReasonNote        string
	GeneratedRevision catalog.Revision
	FinalRevision     catalog.Revision
	ClosedAt          time.Time
	MergedAt          *time.Time
	ObservedAt        time.Time
	ReviewerEdits     []ReviewFileEdit
}

// CanonicalReviewOutcome deep-copies and orders reviewer edits by stable path.
func CanonicalReviewOutcome(outcome ReviewOutcome) ReviewOutcome {
	canonical := outcome
	canonical.ClosedAt = outcome.ClosedAt.UTC()
	canonical.ObservedAt = outcome.ObservedAt.UTC()
	if outcome.MergedAt != nil {
		mergedAt := outcome.MergedAt.UTC()
		canonical.MergedAt = &mergedAt
	}
	canonical.ReviewerEdits = append([]ReviewFileEdit{}, outcome.ReviewerEdits...)
	slices.SortFunc(canonical.ReviewerEdits, func(left, right ReviewFileEdit) int {
		return cmp.Or(cmp.Compare(left.Path, right.Path), cmp.Compare(left.PreviousPath, right.PreviousPath))
	})

	return canonical
}

// DeriveReviewOutcomeID returns the v1 terminal-evidence identity. Its field
// sequence is a compatibility boundary: observation time is excluded so a
// later exact retry retains the same identity.
func DeriveReviewOutcomeID(outcome ReviewOutcome) string {
	outcome = CanonicalReviewOutcome(outcome)
	hash := sha256.New()
	writeHashField := func(value string) {
		_, _ = hash.Write([]byte(value))
		_, _ = hash.Write([]byte{0})
	}
	for _, value := range []string{
		"argus-review-outcome-v1", outcome.ReviewID, outcome.ProposalID, outcome.ValidationID,
		string(outcome.Decision), string(outcome.ReasonCode), outcome.ReasonNote,
		string(outcome.FinalRevision.Algorithm), outcome.FinalRevision.Digest,
		outcome.ClosedAt.Format(time.RFC3339Nano),
	} {
		writeHashField(value)
	}
	if outcome.MergedAt == nil {
		writeHashField("")
	} else {
		writeHashField(outcome.MergedAt.Format(time.RFC3339Nano))
	}
	for _, edit := range outcome.ReviewerEdits {
		for _, value := range []string{
			edit.Path, edit.PreviousPath, string(edit.Kind), strconv.Itoa(edit.Additions),
			strconv.Itoa(edit.Deletions), edit.Patch,
		} {
			writeHashField(value)
		}
	}

	return hex.EncodeToString(hash.Sum(nil))
}

// ValidateReviewOutcome enforces terminal decision, reason, time, and complete-diff invariants.
func ValidateReviewOutcome(outcome ReviewOutcome) error {
	pullRequestURL, urlErr := url.Parse(outcome.PullRequestURL)
	if outcome.APIVersion != ReviewOutcomeAPIVersion || !sha256Pattern.MatchString(outcome.OutcomeID) ||
		!sha256Pattern.MatchString(outcome.ReviewID) || !sha256Pattern.MatchString(outcome.ProposalID) ||
		!sha256Pattern.MatchString(outcome.ValidationID) || outcome.PullRequestNumber < 1 ||
		urlErr != nil || pullRequestURL.Scheme != "https" || pullRequestURL.Host == "" ||
		len(outcome.PullRequestURL) > 2048 ||
		outcome.ClosedAt.IsZero() || outcome.ObservedAt.Before(outcome.ClosedAt) ||
		outcome.ClosedAt.Location() != time.UTC || outcome.ObservedAt.Location() != time.UTC {
		return fmt.Errorf("%w: review outcome envelope", ErrInvalid)
	}
	if err := ValidateRepository(outcome.Repository); err != nil {
		return err
	}
	if err := ValidateRevision(outcome.GeneratedRevision); err != nil {
		return err
	}
	if err := ValidateRevision(outcome.FinalRevision); err != nil {
		return err
	}
	if outcome.Repository.Identity.Provider != catalog.ProviderGitHub ||
		outcome.GeneratedRevision.Algorithm != outcome.FinalRevision.Algorithm ||
		len(outcome.ReviewerEdits) > MaxReviewerEditFiles ||
		!slices.Equal(outcome.ReviewerEdits, CanonicalReviewOutcome(outcome).ReviewerEdits) ||
		outcome.OutcomeID != DeriveReviewOutcomeID(outcome) {
		return fmt.Errorf("%w: review outcome provenance", ErrInvalid)
	}
	if err := validateReviewDecision(outcome); err != nil {
		return err
	}
	totalPatchBytes := 0
	paths := make(map[string]struct{}, len(outcome.ReviewerEdits))
	for _, edit := range outcome.ReviewerEdits {
		if err := validateReviewFileEdit(edit); err != nil {
			return err
		}
		if _, duplicate := paths[edit.Path]; duplicate {
			return fmt.Errorf("%w: duplicate reviewer edit path", ErrInvalid)
		}
		paths[edit.Path] = struct{}{}
		totalPatchBytes += len(edit.Patch)
	}
	if totalPatchBytes > MaxReviewerTotalPatchBytes {
		return fmt.Errorf("%w: reviewer patch budget", ErrInvalid)
	}

	return nil
}

func validateReviewDecision(outcome ReviewOutcome) error {
	if len(outcome.ReasonNote) > 1000 || strings.TrimSpace(outcome.ReasonNote) != outcome.ReasonNote ||
		strings.ContainsRune(outcome.ReasonNote, '\x00') {
		return fmt.Errorf("%w: review reason note", ErrInvalid)
	}
	switch outcome.Decision {
	case ReviewAcceptedAsProposed:
		if outcome.ReasonCode != ReviewReasonApproved || outcome.MergedAt == nil ||
			len(outcome.ReviewerEdits) != 0 {
			return fmt.Errorf("%w: unchanged acceptance", ErrInvalid)
		}
	case ReviewAcceptedWithEdits:
		if outcome.ReasonCode != ReviewReasonCorrected || outcome.MergedAt == nil ||
			len(outcome.ReviewerEdits) == 0 {
			return fmt.Errorf("%w: edited acceptance", ErrInvalid)
		}
	case ReviewRejected:
		if outcome.MergedAt != nil || !validRejectionReason(outcome.ReasonCode) {
			return fmt.Errorf("%w: rejected review", ErrInvalid)
		}
	default:
		return fmt.Errorf("%w: review decision", ErrInvalid)
	}
	if outcome.MergedAt != nil && (outcome.MergedAt.Location() != time.UTC ||
		outcome.MergedAt.After(outcome.ClosedAt)) {
		return fmt.Errorf("%w: review merge time", ErrInvalid)
	}
	if outcome.ReasonCode == ReviewReasonOther && strings.TrimSpace(outcome.ReasonNote) == "" {
		return fmt.Errorf("%w: other review reason requires note", ErrInvalid)
	}
	if len(outcome.ReviewerEdits) != 0 && outcome.FinalRevision == outcome.GeneratedRevision {
		return fmt.Errorf("%w: reviewer edits require a distinct final revision", ErrInvalid)
	}

	return nil
}

func validRejectionReason(reason ReviewReasonCode) bool {
	switch reason {
	case ReviewReasonIncorrectRepair, ReviewReasonUnsafeRepair, ReviewReasonNoLongerNeeded,
		ReviewReasonSuperseded, ReviewReasonOther:
		return true
	default:
		return false
	}
}

func validateReviewFileEdit(edit ReviewFileEdit) error {
	if !validRepositoryPath(edit.Path) || edit.Additions < 0 || edit.Deletions < 0 ||
		edit.Additions+edit.Deletions == 0 || edit.Patch == "" || len(edit.Patch) > MaxReviewerPatchBytes ||
		strings.ContainsRune(edit.Patch, '\x00') {
		return fmt.Errorf("%w: reviewer file edit", ErrInvalid)
	}
	if edit.PreviousPath != "" && !validRepositoryPath(edit.PreviousPath) {
		return fmt.Errorf("%w: reviewer previous path", ErrInvalid)
	}
	switch edit.Kind {
	case ReviewFileAdded, ReviewFileModified, ReviewFileDeleted, ReviewFileCopied:
		if edit.PreviousPath != "" {
			return fmt.Errorf("%w: unexpected previous path", ErrInvalid)
		}
	case ReviewFileRenamed:
		if edit.PreviousPath == "" || edit.PreviousPath == edit.Path {
			return fmt.Errorf("%w: renamed reviewer file", ErrInvalid)
		}
	default:
		return fmt.Errorf("%w: reviewer file kind", ErrInvalid)
	}

	return nil
}
