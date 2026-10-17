package contract

import (
	"fmt"
	"time"

	"github.com/stanimirivanov/argus/internal/adaptation"
	"github.com/stanimirivanov/argus/internal/adaptation/fieldrepair"
	"github.com/stanimirivanov/argus/internal/catalog"
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

// ImportFieldProposalV1 verifies a public proposal before any source mutation.
func ImportFieldProposalV1(document contracts.FieldAdaptationProposalV1) (fieldrepair.Proposal, error) {
	if err := contracts.ValidateFieldAdaptationProposalV1(document); err != nil {
		return fieldrepair.Proposal{}, err
	}
	observedAt, err := time.Parse(time.RFC3339Nano, document.Change.ObservedAt)
	if err != nil {
		return fieldrepair.Proposal{}, err
	}
	proposal := fieldrepair.Proposal{
		APIVersion: document.APIVersion, PolicyVersion: document.PolicyVersion, ProposalID: document.ProposalID,
		ImpactAPIVersion: document.SourceImpact.APIVersion, ImpactAnalyzerVersion: document.SourceImpact.AnalyzerVersion,
		Change: change.Reference{SourceRepository: importRepository(document.Change.SourceRepository),
			PullRequestNumber: document.Change.PullRequest.Number, BaseRevision: importRevision(document.Change.BaseRevision),
			HeadRevision: importRevision(document.Change.HeadRevision), ObservedAt: observedAt,
			Trigger: change.Trigger{Provider: catalog.Provider(document.Change.Trigger.Provider),
				DeliveryID: document.Change.Trigger.DeliveryID, Event: document.Change.Trigger.Event, Action: document.Change.Trigger.Action}},
		Test: importTest(document.Test), Classification: document.Classification, Decision: document.Decision,
		Rename: change.FieldRename{DocumentPath: document.FieldRename.DocumentPath, BaseSHA256: document.FieldRename.BaseSHA256,
			HeadSHA256: document.FieldRename.HeadSHA256, Method: document.FieldRename.Method, Path: document.FieldRename.Path,
			OperationID: document.FieldRename.OperationID, Capabilities: append([]string{}, document.FieldRename.Capabilities...),
			Direction: document.FieldRename.Direction, StatusCode: document.FieldRename.StatusCode,
			PreviousName: document.FieldRename.PreviousName, Name: document.FieldRename.Name},
		AdapterID: document.Adapter.ID, AdapterVersion: document.Adapter.Version,
		Edit: adaptation.TextEdit{Path: document.Edit.Path, BeforeSHA256: document.Edit.BeforeSHA256,
			StartByte: document.Edit.StartByte, EndByte: document.Edit.EndByte,
			Original: document.Edit.Original, Replacement: document.Edit.Replacement, SemanticRole: document.Edit.SemanticRole},
	}
	proposal.Test = adaptation.CanonicalTestReference(proposal.Test)
	if err := fieldrepair.ValidateProposal(proposal); err != nil {
		return fieldrepair.Proposal{}, err
	}

	return proposal, nil
}
