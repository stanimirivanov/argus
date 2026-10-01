package contracts

import (
	"os"
	"testing"
)

func TestAdaptationProposalFixtureConformsToGeneratedSchema(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile(fixturePath("fixtures/adaptation-proposal/v1/valid/endpoint-rename.json"))
	if err != nil {
		t.Fatalf("read proposal fixture: %v", err)
	}
	document, err := DecodeAdaptationProposalV1(data)
	if err != nil {
		t.Fatalf("decode proposal fixture: %v", err)
	}
	if document.Decision != "PATCH_AND_VALIDATE" || document.Edit.SemanticRole != "request-target" {
		t.Fatalf("unexpected proposal: %+v", document)
	}
}

func TestAdaptationResultSchemaRejectsAssertionEdit(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile(fixturePath("fixtures/functional-api-adaptation-result/v1/invalid/assertion-edit.json"))
	if err != nil {
		t.Fatalf("read invalid result fixture: %v", err)
	}
	if _, err := DecodeFunctionalAPIAdaptationResultV1(data); err == nil {
		t.Fatal("assertion edit unexpectedly passed contract validation")
	}
}
