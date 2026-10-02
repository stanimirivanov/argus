package endpointrepair

import (
	"errors"
	"testing"

	"github.com/stanimirivanov/argus/internal/adaptation"
)

func TestValidationContractAdmitsEndpointRepairPolicy(t *testing.T) {
	if PolicyVersion != adaptation.ValidationProposalPolicyVersion {
		t.Fatal("validation evidence no longer admits endpoint repair proposals")
	}
}

func TestAdapterResultRejectsNonRequestTargetEdit(t *testing.T) {
	result := AdapterResult{
		APIVersion: ResultAPIVersion,
		ProposalID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		AdapterID:  "reviewed-adapter", AdapterVersion: "v1", Outcome: "candidate",
		Edit: &adaptation.TextEdit{
			Path:         "tests/request.ts",
			BeforeSHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			StartByte:    0, EndByte: 4, Original: "/old", Replacement: "/new",
			SemanticRole: "assertion",
		},
	}
	if err := ValidateAdapterResult(result); !errors.Is(err, adaptation.ErrInvalid) {
		t.Fatalf("non-request-target edit must be rejected, got %v", err)
	}
}
