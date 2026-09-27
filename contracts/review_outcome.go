package contracts

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// ReviewOutcomeV1APIVersion identifies terminal structured review evidence.
const ReviewOutcomeV1APIVersion = "argus.dev/review-outcome/v1"

//go:embed generated/review-outcome/v1/review-outcome.schema.json
var reviewOutcomeV1SchemaJSON []byte

var loadReviewOutcomeV1Schema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(
		reviewOutcomeV1SchemaJSON,
		"https://argus.dev/contracts/review-outcome/v1/schema.json",
		"review outcome",
	)
})

// ReviewOutcomeV1 is terminal provider and reviewer evidence for one adaptation review.
type ReviewOutcomeV1 struct {
	APIVersion        string              `json:"apiVersion"`
	OutcomeID         string              `json:"outcomeId"`
	ReviewID          string              `json:"reviewId"`
	ProposalID        string              `json:"proposalId"`
	ValidationID      string              `json:"validationId"`
	Repository        RepositoryReference `json:"repository"`
	PullRequest       OutcomePullRequest  `json:"pullRequest"`
	Decision          string              `json:"decision"`
	Reason            ReviewOutcomeReason `json:"reason"`
	GeneratedRevision RevisionReference   `json:"generatedRevision"`
	FinalRevision     RevisionReference   `json:"finalRevision"`
	ClosedAt          string              `json:"closedAt"`
	MergedAt          *string             `json:"mergedAt"`
	ObservedAt        string              `json:"observedAt"`
	ReviewerEdits     []ReviewerEdit      `json:"reviewerEdits"`
}

// OutcomePullRequest identifies the provider review whose terminal state was observed.
type OutcomePullRequest struct {
	Number int    `json:"number"`
	URL    string `json:"url"`
}

// ReviewOutcomeReason is the explicit human classification retained beside provider state.
type ReviewOutcomeReason struct {
	Code string  `json:"code"`
	Note *string `json:"note"`
}

// ReviewerEdit is one complete bounded patch after Argus' generated commit.
type ReviewerEdit struct {
	Path         string  `json:"path"`
	PreviousPath *string `json:"previousPath"`
	Kind         string  `json:"kind"`
	Additions    int     `json:"additions"`
	Deletions    int     `json:"deletions"`
	Patch        string  `json:"patch"`
}

// ValidateReviewOutcomeV1 validates a typed review outcome.
func ValidateReviewOutcomeV1(document ReviewOutcomeV1) error {
	return validateTypedAdaptationDocument(document, loadReviewOutcomeV1Schema, "review outcome")
}

// DecodeReviewOutcomeV1 validates and decodes terminal review evidence.
func DecodeReviewOutcomeV1(data []byte) (ReviewOutcomeV1, error) {
	var document ReviewOutcomeV1
	if err := validateAdaptationJSON(data, loadReviewOutcomeV1Schema, "review outcome"); err != nil {
		return document, err
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return document, fmt.Errorf("decode review outcome: %w", err)
	}

	return document, nil
}
