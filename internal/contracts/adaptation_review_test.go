package contracts

import (
	"os"
	"testing"
)

func TestAdaptationReviewFixtureConformsToGeneratedSchema(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(fixturePath("fixtures/adaptation-review/v1/valid/draft-endpoint-repair.json"))
	if err != nil {
		t.Fatalf("read adaptation review fixture: %v", err)
	}
	document, err := DecodeAdaptationReviewV1(data)
	if err != nil {
		t.Fatalf("decode adaptation review: %v", err)
	}
	if document.PullRequest.Number != 17 || !document.PullRequest.Draft {
		t.Fatalf("unexpected adaptation review: %+v", document)
	}
}

func TestAdaptationReviewSchemaRejectsNonDraftReview(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(fixturePath("fixtures/adaptation-review/v1/invalid/non-draft.json"))
	if err != nil {
		t.Fatalf("read invalid adaptation review: %v", err)
	}
	if _, err := DecodeAdaptationReviewV1(data); err == nil {
		t.Fatal("non-draft adaptation review unexpectedly passed contract validation")
	}
}
