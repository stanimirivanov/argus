package adaptation

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
)

// ValidateReviewPublication enforces the portable handoff contract returned
// after the provider confirms an open draft pull request.
func ValidateReviewPublication(publication ReviewPublication) error {
	pullRequestURL, urlErr := url.Parse(publication.PullRequestURL)
	if publication.APIVersion != ReviewAPIVersion ||
		!sha256Pattern.MatchString(publication.ReviewID) ||
		!sha256Pattern.MatchString(publication.ProposalID) ||
		!sha256Pattern.MatchString(publication.ValidationID) ||
		publication.Provider != ReviewProviderGitHub || publication.PullRequestNumber < 1 ||
		urlErr != nil || pullRequestURL.Scheme != "https" || pullRequestURL.Host == "" ||
		len(publication.PullRequestURL) > 2048 || !publication.Draft || publication.State != "open" ||
		publication.PublishedAt.IsZero() || publication.PublishedAt.Location() != time.UTC {
		return fmt.Errorf("%w: review publication envelope", ErrInvalid)
	}
	if err := ValidateRepository(publication.Repository); err != nil {
		return err
	}
	if err := ValidateRevision(publication.BaseRevision); err != nil {
		return err
	}
	if err := ValidateRevision(publication.HeadRevision); err != nil {
		return err
	}
	if publication.Repository.Identity.Provider != catalog.ProviderGitHub ||
		publication.BaseRevision.Algorithm != publication.HeadRevision.Algorithm {
		return fmt.Errorf("%w: review provider revision", ErrInvalid)
	}
	if !validBranchName(publication.BaseBranch) || !validBranchName(publication.HeadBranch) ||
		publication.BaseBranch == publication.HeadBranch {
		return fmt.Errorf("%w: review branches", ErrInvalid)
	}

	return nil
}

// ValidateRepository checks the provider identity and display coordinates
// required at an adaptation provider boundary.
func ValidateRepository(repository catalog.Repository) error {
	if !repository.Valid() {
		return fmt.Errorf("%w: review repository", ErrInvalid)
	}

	return nil
}

// ValidateRevision verifies that a revision is normalized and immutable.
func ValidateRevision(revision catalog.Revision) error {
	if !revision.Valid() {
		return fmt.Errorf("%w: review revision", ErrInvalid)
	}

	return nil
}

// ValidateBranchName applies the portable subset accepted for caller-selected
// base branches. Provider adapters may impose additional restrictions.
func ValidateBranchName(branch string) error {
	if !validBranchName(branch) {
		return fmt.Errorf("%w: review branch", ErrInvalid)
	}

	return nil
}

func validBranchName(branch string) bool {
	if branch == "" || len(branch) > 255 || branch != strings.TrimSpace(branch) {
		return false
	}
	if strings.HasPrefix(branch, "/") || strings.HasSuffix(branch, "/") ||
		strings.HasSuffix(branch, ".") || strings.HasSuffix(branch, ".lock") ||
		strings.Contains(branch, "..") || strings.Contains(branch, "@{") ||
		strings.ContainsAny(branch, " ~^:?*[\\") {
		return false
	}
	for _, character := range branch {
		if character < 0x20 || character == 0x7f {
			return false
		}
	}

	return true
}
