// Package contract translates versioned adaptation protocol documents.
package contract

import (
	"fmt"
	"time"

	"github.com/stanimirivanov/argus/contracts"
	"github.com/stanimirivanov/argus/internal/adaptation"
	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
)

// ExportRequestV1 converts a validated domain request to adapter JSON.
func ExportRequestV1(request adaptation.AdapterRequest) (contracts.FunctionalAPIAdaptationRequestV1, error) {
	request.Test = adaptation.CanonicalTestReference(request.Test)
	request.Rename = adaptation.CanonicalEndpointRename(request.Rename)
	if err := adaptation.ValidateAdapterRequest(request); err != nil {
		return contracts.FunctionalAPIAdaptationRequestV1{}, err
	}
	document := contracts.FunctionalAPIAdaptationRequestV1{
		APIVersion: request.APIVersion, ProposalID: request.ProposalID,
		Change: exportChange(request.Change), Test: exportTest(request.Test),
		EndpointRename: exportRename(request.Rename),
	}
	if err := contracts.ValidateFunctionalAPIAdaptationRequestV1(document); err != nil {
		return contracts.FunctionalAPIAdaptationRequestV1{}, fmt.Errorf("export adaptation request: %w", err)
	}

	return document, nil
}

// ImportResultV1 converts structurally valid, untrusted adapter output.
func ImportResultV1(document contracts.FunctionalAPIAdaptationResultV1) (adaptation.AdapterResult, error) {
	result := adaptation.AdapterResult{
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
	if err := adaptation.ValidateAdapterResult(result); err != nil {
		return adaptation.AdapterResult{}, fmt.Errorf("validate adaptation result semantics: %w", err)
	}

	return result, nil
}

// ExportProposalV1 converts a validated proposal to its public contract.
func ExportProposalV1(proposal adaptation.Proposal) (contracts.AdaptationProposalV1, error) {
	proposal.Test = adaptation.CanonicalTestReference(proposal.Test)
	proposal.Rename = adaptation.CanonicalEndpointRename(proposal.Rename)
	if err := adaptation.ValidateProposal(proposal); err != nil {
		return contracts.AdaptationProposalV1{}, err
	}
	document := contracts.AdaptationProposalV1{
		APIVersion: proposal.APIVersion, PolicyVersion: proposal.PolicyVersion,
		ProposalID: proposal.ProposalID,
		SourceImpact: contracts.AdaptationSourceImpact{
			APIVersion: proposal.ImpactAPIVersion, AnalyzerVersion: proposal.ImpactAnalyzerVersion,
		},
		Change: exportChange(proposal.Change), Test: exportTest(proposal.Test),
		Classification: string(proposal.Classification), Decision: string(proposal.Decision),
		EndpointRename: exportRename(proposal.Rename),
		Adapter:        contracts.AdapterIdentity{ID: proposal.AdapterID, Version: proposal.AdapterVersion},
		Edit: contracts.AdaptationTextEdit{
			Path: proposal.Edit.Path, BeforeSHA256: proposal.Edit.BeforeSHA256,
			StartByte: proposal.Edit.StartByte, EndByte: proposal.Edit.EndByte,
			Original: proposal.Edit.Original, Replacement: proposal.Edit.Replacement,
			SemanticRole: proposal.Edit.SemanticRole,
		},
	}
	if err := contracts.ValidateAdaptationProposalV1(document); err != nil {
		return contracts.AdaptationProposalV1{}, fmt.Errorf("export adaptation proposal: %w", err)
	}

	return document, nil
}

func exportChange(reference change.Reference) contracts.AdaptationChangeReference {
	return contracts.AdaptationChangeReference{
		SourceRepository: exportRepository(reference.SourceRepository),
		PullRequest:      contracts.PullRequestReference{Number: reference.PullRequestNumber},
		BaseRevision:     exportRevision(reference.BaseRevision), HeadRevision: exportRevision(reference.HeadRevision),
		ObservedAt: reference.ObservedAt.Format(time.RFC3339Nano),
		Trigger: contracts.ChangeTrigger{
			Provider: string(reference.Trigger.Provider), DeliveryID: reference.Trigger.DeliveryID,
			Event: reference.Trigger.Event, Action: reference.Trigger.Action,
		},
	}
}

func exportTest(test adaptation.TestReference) contracts.AdaptationTestReference {
	return contracts.AdaptationTestReference{
		Repository: exportRepository(test.Repository), Revision: exportRevision(test.Revision),
		SuiteKey: test.SuiteKey, TestKey: test.TestKey, Name: test.Name, Adapter: test.Adapter,
		Capabilities: append([]string{}, test.Capabilities...),
	}
}

func exportRename(rename adaptation.EndpointRename) contracts.EndpointRenameEvidence {
	return contracts.EndpointRenameEvidence{
		Method: rename.Method, OperationID: rename.OperationID, PreviousPath: rename.PreviousPath,
		Path: rename.Path, Capabilities: append([]string{}, rename.Capabilities...),
	}
}

func exportRepository(repository catalog.Repository) contracts.RepositoryReference {
	return contracts.RepositoryReference{
		Provider: string(repository.Identity.Provider), Host: repository.Identity.Host,
		ProviderRepositoryID: repository.Identity.ProviderRepositoryID,
		Owner:                repository.Owner, Name: repository.Name,
	}
}

func exportRevision(revision catalog.Revision) contracts.RevisionReference {
	return contracts.RevisionReference{Algorithm: string(revision.Algorithm), Digest: revision.Digest}
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
