package contracts

import (
	"os"
	"testing"
)

func TestValidationEvidenceFixtureConformsToGeneratedSchema(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("fixtures/validation-evidence/v1/valid/endpoint-rename.json")
	if err != nil {
		t.Fatalf("read validation evidence fixture: %v", err)
	}
	document, err := DecodeValidationEvidenceV1(data)
	if err != nil {
		t.Fatalf("decode validation evidence: %v", err)
	}
	if len(document.Runs) != 3 || document.Runs[1].Outcome != "passed" {
		t.Fatalf("unexpected validation evidence: %+v", document)
	}
}

func TestValidationResultSchemaRejectsUnknownPhase(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("fixtures/functional-api-repair-validation-result/v1/invalid/unknown-phase.json")
	if err != nil {
		t.Fatalf("read invalid validation result: %v", err)
	}
	if _, err := DecodeFunctionalAPIRepairValidationResultV1(data); err == nil {
		t.Fatal("unknown validation phase unexpectedly passed contract validation")
	}
}

func TestValidationRejectionFixtureConformsToGeneratedSchema(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("fixtures/validation-rejection/v1/valid/candidate-failed.json")
	if err != nil {
		t.Fatalf("read validation rejection fixture: %v", err)
	}
	document, err := DecodeValidationRejectionV1(data)
	if err != nil {
		t.Fatalf("decode validation rejection: %v", err)
	}
	if document.Rejection.Reason != "candidate-failed" || len(document.Runs) != 2 {
		t.Fatalf("unexpected validation rejection: %+v", document)
	}
}
