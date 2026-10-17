package contract

import (
	"fmt"

	"github.com/stanimirivanov/argus/internal/adaptation"
	"github.com/stanimirivanov/argus/internal/adaptation/fieldrepair"
	"github.com/stanimirivanov/argus/internal/change"
	"github.com/stanimirivanov/argus/internal/contracts"
)

// ExportFieldRequestV1 translates a validated field request for reviewed code.
func ExportFieldRequestV1(request fieldrepair.Request) (contracts.FieldAdaptationRequestV1, error) {
	if err := fieldrepair.ValidateRequest(request); err != nil {
		return contracts.FieldAdaptationRequestV1{}, err
	}
	document := contracts.FieldAdaptationRequestV1{
		APIVersion: request.APIVersion, ProposalID: request.ProposalID,
		Change: exportChange(request.Change), Test: exportTest(request.Test),
		FieldRename: exportFieldRename(request.Rename),
	}
	if err := contracts.ValidateFieldAdaptationRequestV1(document); err != nil {
		return contracts.FieldAdaptationRequestV1{}, fmt.Errorf("export field request: %w", err)
	}

	return document, nil
}

// ImportFieldResultV1 converts structurally valid, untrusted adapter output.
func ImportFieldResultV1(document contracts.FieldAdaptationResultV1) (fieldrepair.Result, error) {
	result := fieldrepair.Result{
		APIVersion: document.APIVersion, ProposalID: document.ProposalID,
		AdapterID: document.Adapter.ID, AdapterVersion: document.Adapter.Version,
		Outcome: document.Outcome, ReasonCode: optionalString(document.ReasonCode),
		Reason: optionalString(document.Reason),
	}
	if document.Edit != nil {
		result.Edit = &adaptation.TextEdit{
			Path: document.Edit.Path, BeforeSHA256: document.Edit.BeforeSHA256,
			StartByte: document.Edit.StartByte, EndByte: document.Edit.EndByte,
			Original: document.Edit.Original, Replacement: document.Edit.Replacement,
			SemanticRole: document.Edit.SemanticRole,
		}
	}
	if err := fieldrepair.ValidateResult(result); err != nil {
		return fieldrepair.Result{}, err
	}

	return result, nil
}

// ExportFieldProposalV1 publishes a proposal without granting validation or
// review authority to the field candidate.
func ExportFieldProposalV1(proposal fieldrepair.Proposal) (contracts.FieldAdaptationProposalV1, error) {
	if err := fieldrepair.ValidateProposal(proposal); err != nil {
		return contracts.FieldAdaptationProposalV1{}, err
	}
	document := contracts.FieldAdaptationProposalV1{
		APIVersion: proposal.APIVersion, PolicyVersion: proposal.PolicyVersion,
		ProposalID: proposal.ProposalID, SourceImpact: contracts.AdaptationSourceImpact{
			APIVersion: proposal.ImpactAPIVersion, AnalyzerVersion: proposal.ImpactAnalyzerVersion,
		},
		Change: exportChange(proposal.Change), Test: exportTest(proposal.Test),
		Classification: proposal.Classification, Decision: proposal.Decision,
		FieldRename: exportFieldRename(proposal.Rename),
		Adapter:     contracts.AdapterIdentity{ID: proposal.AdapterID, Version: proposal.AdapterVersion},
		Edit: contracts.AdaptationTextEdit{
			Path: proposal.Edit.Path, BeforeSHA256: proposal.Edit.BeforeSHA256,
			StartByte: proposal.Edit.StartByte, EndByte: proposal.Edit.EndByte,
			Original: proposal.Edit.Original, Replacement: proposal.Edit.Replacement,
			SemanticRole: proposal.Edit.SemanticRole,
		},
	}
	if err := contracts.ValidateFieldAdaptationProposalV1(document); err != nil {
		return contracts.FieldAdaptationProposalV1{}, err
	}

	return document, nil
}

func exportFieldRename(rename change.FieldRename) contracts.FieldRenameEvidenceV1 {
	return contracts.FieldRenameEvidenceV1{
		DocumentPath: rename.DocumentPath, BaseSHA256: rename.BaseSHA256,
		HeadSHA256: rename.HeadSHA256, Method: rename.Method, Path: rename.Path,
		OperationID: rename.OperationID, Capabilities: append([]string{}, rename.Capabilities...),
		Direction: rename.Direction, StatusCode: rename.StatusCode,
		PreviousName: rename.PreviousName, Name: rename.Name,
	}
}
