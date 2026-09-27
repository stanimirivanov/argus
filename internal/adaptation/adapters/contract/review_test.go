package contract

import (
	"os"
	"testing"

	"github.com/stanimirivanov/argus/contracts"
)

func TestAdaptationReviewRoundTrip(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../../../../contracts/fixtures/adaptation-review/v1/valid/draft-endpoint-repair.json")
	if err != nil {
		t.Fatalf("read review fixture: %v", err)
	}
	document, err := contracts.DecodeAdaptationReviewV1(data)
	if err != nil {
		t.Fatalf("decode review fixture: %v", err)
	}
	publication, err := ImportReviewV1(document)
	if err != nil {
		t.Fatalf("import review: %v", err)
	}
	exported, err := ExportReviewV1(publication)
	if err != nil {
		t.Fatalf("export review: %v", err)
	}
	if exported.ReviewID != document.ReviewID || exported.PullRequest.Number != document.PullRequest.Number {
		t.Fatalf("unexpected review round trip: %+v", exported)
	}
}

func TestValidationEvidenceImportsForReview(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../../../../contracts/fixtures/validation-evidence/v1/valid/endpoint-rename.json")
	if err != nil {
		t.Fatalf("read validation evidence fixture: %v", err)
	}
	document, err := contracts.DecodeValidationEvidenceV1(data)
	if err != nil {
		t.Fatalf("decode validation evidence fixture: %v", err)
	}
	evidence, err := ImportValidationEvidenceV1(document)
	if err != nil {
		t.Fatalf("import validation evidence: %v", err)
	}
	if evidence.ValidationID != document.ValidationID || len(evidence.Runs) != 3 {
		t.Fatalf("unexpected imported validation evidence: %+v", evidence)
	}
}
