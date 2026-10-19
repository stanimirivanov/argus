package contract

import (
	"github.com/stanimirivanov/argus/internal/adaptation"
	"github.com/stanimirivanov/argus/internal/adaptation/browserlocator"
	"github.com/stanimirivanov/argus/internal/contracts"
)

// ExportBrowserLocatorDiagnosisV1 converts a policy decision to its versioned
// transport and checks the complete public schema before publication.
func ExportBrowserLocatorDiagnosisV1(value browserlocator.Diagnosis) (contracts.BrowserLocatorDiagnosisV1, error) {
	if err := browserlocator.ValidateDiagnosis(value); err != nil {
		return contracts.BrowserLocatorDiagnosisV1{}, err
	}
	document := contracts.BrowserLocatorDiagnosisV1{APIVersion: value.APIVersion,
		ReasonCode: value.ReasonCode, BaselineID: value.BaselineID, FailedID: value.FailedID,
		SuiteKey: value.SuiteKey, TestKey: value.TestKey}
	if value.Proposal != nil {
		proposal, err := ExportBrowserLocatorProposalV1(*value.Proposal)
		if err != nil {
			return contracts.BrowserLocatorDiagnosisV1{}, err
		}
		document.Proposal = &proposal
	}
	if err := contracts.ValidateBrowserLocatorDiagnosisV1(document); err != nil {
		return contracts.BrowserLocatorDiagnosisV1{}, err
	}

	return document, nil
}

// ImportBrowserLocatorProposalV1 checks a candidate at the transport boundary.
func ImportBrowserLocatorProposalV1(document contracts.BrowserLocatorProposalV1) (browserlocator.Proposal, error) {
	if err := contracts.ValidateBrowserLocatorProposalV1(document); err != nil {
		return browserlocator.Proposal{}, err
	}
	proposal := browserlocator.Proposal{APIVersion: document.APIVersion, PolicyVersion: document.PolicyVersion,
		ProposalID: document.ProposalID, BaselineID: document.BaselineID, FailedID: document.FailedID,
		BaselineSHA256: document.BaselineSHA256, FailedSHA256: document.FailedSHA256,
		SuiteKey: document.SuiteKey, TestKey: document.TestKey,
		OriginalTarget: browserlocator.Node(document.OriginalTarget), CurrentTarget: browserlocator.Node(document.CurrentTarget),
		Edit: adaptation.TextEdit{Path: document.Edit.Path, BeforeSHA256: document.Edit.BeforeSHA256,
			StartByte: document.Edit.StartByte, EndByte: document.Edit.EndByte,
			Original: document.Edit.Original, Replacement: document.Edit.Replacement,
			SemanticRole: document.Edit.SemanticRole}}
	if err := browserlocator.ValidateProposal(proposal); err != nil {
		return browserlocator.Proposal{}, err
	}

	return proposal, nil
}

// ExportBrowserLocatorProposalV1 checks a candidate before crossing the wire boundary.
func ExportBrowserLocatorProposalV1(proposal browserlocator.Proposal) (contracts.BrowserLocatorProposalV1, error) {
	if err := browserlocator.ValidateProposal(proposal); err != nil {
		return contracts.BrowserLocatorProposalV1{}, err
	}
	document := contracts.BrowserLocatorProposalV1{
		APIVersion: proposal.APIVersion, PolicyVersion: proposal.PolicyVersion,
		ProposalID: proposal.ProposalID, BaselineID: proposal.BaselineID,
		FailedID: proposal.FailedID, BaselineSHA256: proposal.BaselineSHA256,
		FailedSHA256: proposal.FailedSHA256, SuiteKey: proposal.SuiteKey, TestKey: proposal.TestKey,
		OriginalTarget: exportBrowserTarget(proposal.OriginalTarget),
		CurrentTarget:  exportBrowserTarget(proposal.CurrentTarget),
		Edit: contracts.AdaptationTextEdit{Path: proposal.Edit.Path, BeforeSHA256: proposal.Edit.BeforeSHA256,
			StartByte: proposal.Edit.StartByte, EndByte: proposal.Edit.EndByte,
			Original: proposal.Edit.Original, Replacement: proposal.Edit.Replacement,
			SemanticRole: proposal.Edit.SemanticRole},
	}
	if err := contracts.ValidateBrowserLocatorProposalV1(document); err != nil {
		return contracts.BrowserLocatorProposalV1{}, err
	}

	return document, nil
}

func exportBrowserTarget(node browserlocator.Node) contracts.BrowserTarget {
	return contracts.BrowserTarget{Role: node.Role, Name: node.Name, TestID: node.TestID, Visible: node.Visible}
}
