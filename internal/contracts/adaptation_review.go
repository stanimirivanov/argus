package contracts

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// AdaptationReviewV1APIVersion identifies the first review-publication contract.
const AdaptationReviewV1APIVersion = "argus.dev/adaptation-review/v1"

var adaptationReviewV1SchemaJSON = mustReadSchema("adaptation-review/v1/adaptation-review.schema.json")

var loadAdaptationReviewV1Schema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(
		adaptationReviewV1SchemaJSON,
		"https://argus.dev/contracts/adaptation-review/v1/schema.json",
		"adaptation review",
	)
})

// AdaptationReviewV1 is the portable reference to an open review-first pull request.
type AdaptationReviewV1 struct {
	APIVersion   string              `json:"apiVersion"`
	ReviewID     string              `json:"reviewId"`
	ProposalID   string              `json:"proposalId"`
	ValidationID string              `json:"validationId"`
	Repository   RepositoryReference `json:"repository"`
	Provider     string              `json:"provider"`
	Base         ReviewBranch        `json:"base"`
	Head         ReviewBranch        `json:"head"`
	PullRequest  ReviewPullRequest   `json:"pullRequest"`
	PublishedAt  string              `json:"publishedAt"`
}

// ReviewBranch binds a provider branch to its observed immutable revision.
type ReviewBranch struct {
	Branch   string            `json:"branch"`
	Revision RevisionReference `json:"revision"`
}

// ReviewPullRequest identifies the enforced open draft review boundary.
type ReviewPullRequest struct {
	Number int    `json:"number"`
	URL    string `json:"url"`
	Draft  bool   `json:"draft"`
	State  string `json:"state"`
}

// ValidateAdaptationReviewV1 validates a typed review publication.
func ValidateAdaptationReviewV1(document AdaptationReviewV1) error {
	return validateTypedAdaptationDocument(document, loadAdaptationReviewV1Schema, "adaptation review")
}

// DecodeAdaptationReviewV1 validates and decodes a review publication.
func DecodeAdaptationReviewV1(data []byte) (AdaptationReviewV1, error) {
	var document AdaptationReviewV1
	if err := validateAdaptationJSON(data, loadAdaptationReviewV1Schema, "adaptation review"); err != nil {
		return document, err
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return document, fmt.Errorf("decode adaptation review: %w", err)
	}

	return document, nil
}
