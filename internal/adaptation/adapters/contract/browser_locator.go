package contract

import (
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
		if err := browserlocator.ValidateProposal(*value.Proposal); err != nil {
			return contracts.BrowserLocatorDiagnosisV1{}, err
		}
		proposal := value.Proposal
		document.Proposal = &contracts.BrowserLocatorProposalV1{
			APIVersion: proposal.APIVersion, PolicyVersion: proposal.PolicyVersion,
			ProposalID: proposal.ProposalID, BaselineID: proposal.BaselineID,
			FailedID: proposal.FailedID, BaselineSHA256: proposal.BaselineSHA256,
			FailedSHA256: proposal.FailedSHA256, SuiteKey: proposal.SuiteKey, TestKey: proposal.TestKey,
			OriginalTarget: exportBrowserTarget(proposal.OriginalTarget),
			CurrentTarget:  exportBrowserTarget(proposal.CurrentTarget),
			Edit: contracts.AdaptationTextEdit{
				Path: proposal.Edit.Path, BeforeSHA256: proposal.Edit.BeforeSHA256,
				StartByte: proposal.Edit.StartByte, EndByte: proposal.Edit.EndByte,
				Original: proposal.Edit.Original, Replacement: proposal.Edit.Replacement,
				SemanticRole: proposal.Edit.SemanticRole,
			},
		}
		if err := contracts.ValidateBrowserLocatorProposalV1(*document.Proposal); err != nil {
			return contracts.BrowserLocatorDiagnosisV1{}, err
		}
	}
	if err := contracts.ValidateBrowserLocatorDiagnosisV1(document); err != nil {
		return contracts.BrowserLocatorDiagnosisV1{}, err
	}

	return document, nil
}

func exportBrowserTarget(node browserlocator.Node) contracts.BrowserTarget {
	return contracts.BrowserTarget{Role: node.Role, Name: node.Name, TestID: node.TestID, Visible: node.Visible}
}
