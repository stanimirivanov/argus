package contracts

import (
	"os"
	"testing"
)

func TestReviewOutcomeFixtureConformsToGeneratedSchema(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(fixturePath("fixtures/review-outcome/v1/valid/accepted-with-edits.json"))
	if err != nil {
		t.Fatalf("read review outcome fixture: %v", err)
	}
	document, err := DecodeReviewOutcomeV1(data)
	if err != nil {
		t.Fatalf("decode review outcome: %v", err)
	}
	if document.Decision != "accepted-with-edits" || len(document.ReviewerEdits) != 1 {
		t.Fatalf("unexpected review outcome: %+v", document)
	}
}

func TestReviewOutcomeSchemaRejectsUnknownDecision(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(fixturePath("fixtures/review-outcome/v1/invalid/unknown-decision.json"))
	if err != nil {
		t.Fatalf("read invalid review outcome: %v", err)
	}
	if _, err := DecodeReviewOutcomeV1(data); err == nil {
		t.Fatal("unknown review decision unexpectedly passed contract validation")
	}
}
