package contract_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stanimirivanov/argus/internal/adaptation"
	adaptationcontract "github.com/stanimirivanov/argus/internal/adaptation/adapters/contract"
	"github.com/stanimirivanov/argus/internal/adaptation/browserlocator"
)

func TestExportBrowserLocatorDiagnosisChecksProposalSchema(t *testing.T) {
	proposal := browserlocator.Proposal{APIVersion: browserlocator.ProposalVersion,
		PolicyVersion: browserlocator.PolicyVersion, ProposalID: strings.Repeat("a", 64),
		BaselineID: "baseline-1", FailedID: "failed-1", BaselineSHA256: strings.Repeat("b", 64),
		FailedSHA256: strings.Repeat("c", 64), SuiteKey: "shop", TestKey: "checkout",
		OriginalTarget: browserlocator.Node{Role: "button", Name: "Checkout", TestID: "x", Visible: true},
		CurrentTarget:  browserlocator.Node{Role: "button", Name: "Checkout", TestID: "y", Visible: true},
		Edit: adaptation.TextEdit{Path: "tests/shop.spec.mjs", BeforeSHA256: strings.Repeat("d", 64),
			StartByte: 10, EndByte: 26, Original: "getByTestId('x')", Replacement: "getByTestId('y')", SemanticRole: "browser-locator"}}
	proposal.ProposalID = ""
	bytes, err := json.Marshal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(bytes)
	proposal.ProposalID = hex.EncodeToString(hash[:])
	diagnosis := browserlocator.Diagnosis{APIVersion: browserlocator.DiagnosisVersion,
		ReasonCode: "candidate", BaselineID: "baseline-1", FailedID: "failed-1",
		SuiteKey: "shop", TestKey: "checkout", Proposal: &proposal}
	if _, err := adaptationcontract.ExportBrowserLocatorDiagnosisV1(diagnosis); err != nil {
		t.Fatal(err)
	}
	diagnosis.FailedID = "other-attempt"
	if _, err := adaptationcontract.ExportBrowserLocatorDiagnosisV1(diagnosis); err == nil {
		t.Fatal("accepted uncorrelated proposal")
	}
	diagnosis.FailedID = "failed-1"
	proposal.Edit.SemanticRole = "assertion"
	if _, err := adaptationcontract.ExportBrowserLocatorDiagnosisV1(diagnosis); err == nil {
		t.Fatal("accepted assertion edit")
	}
}
