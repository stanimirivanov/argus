// Package contract translates versioned adaptation protocol documents.
package contract

import (
	"fmt"
	"time"

	"github.com/stanimirivanov/argus/internal/adaptation"
	"github.com/stanimirivanov/argus/internal/adaptation/endpointrepair"
	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
	"github.com/stanimirivanov/argus/internal/contracts"
)

// ExportRequestV1 converts a validated domain request to adapter JSON.
func ExportRequestV1(request endpointrepair.AdapterRequest) (contracts.FunctionalAPIAdaptationRequestV1, error) {
	request.Test = adaptation.CanonicalTestReference(request.Test)
	request.Rename = endpointrepair.CanonicalEndpointRename(request.Rename)
	if err := endpointrepair.ValidateAdapterRequest(request); err != nil {
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
func ImportResultV1(document contracts.FunctionalAPIAdaptationResultV1) (endpointrepair.AdapterResult, error) {
	result := endpointrepair.AdapterResult{
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
	if err := endpointrepair.ValidateAdapterResult(result); err != nil {
		return endpointrepair.AdapterResult{}, fmt.Errorf("validate adaptation result semantics: %w", err)
	}

	return result, nil
}

// ExportProposalV1 converts a validated proposal to its public contract.
func ExportProposalV1(proposal endpointrepair.Proposal) (contracts.AdaptationProposalV1, error) {
	proposal.Test = adaptation.CanonicalTestReference(proposal.Test)
	proposal.Rename = endpointrepair.CanonicalEndpointRename(proposal.Rename)
	if err := endpointrepair.ValidateProposal(proposal); err != nil {
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

// ImportProposalV1 converts a structurally valid public proposal into the
// canonical domain value used by isolated validation.
func ImportProposalV1(document contracts.AdaptationProposalV1) (endpointrepair.Proposal, error) {
	if err := contracts.ValidateAdaptationProposalV1(document); err != nil {
		return endpointrepair.Proposal{}, err
	}
	observedAt, err := time.Parse(time.RFC3339Nano, document.Change.ObservedAt)
	if err != nil {
		return endpointrepair.Proposal{}, fmt.Errorf("parse proposal observation time: %w", err)
	}
	proposal := endpointrepair.Proposal{
		APIVersion: document.APIVersion, PolicyVersion: document.PolicyVersion,
		ProposalID: document.ProposalID, ImpactAPIVersion: document.SourceImpact.APIVersion,
		ImpactAnalyzerVersion: document.SourceImpact.AnalyzerVersion,
		Change: change.Reference{
			SourceRepository:  importRepository(document.Change.SourceRepository),
			PullRequestNumber: document.Change.PullRequest.Number,
			BaseRevision:      importRevision(document.Change.BaseRevision),
			HeadRevision:      importRevision(document.Change.HeadRevision), ObservedAt: observedAt,
			Trigger: change.Trigger{
				Provider:   catalog.Provider(document.Change.Trigger.Provider),
				DeliveryID: document.Change.Trigger.DeliveryID,
				Event:      document.Change.Trigger.Event, Action: document.Change.Trigger.Action,
			},
		},
		Test: importTest(document.Test), Classification: endpointrepair.Classification(document.Classification),
		Decision: endpointrepair.Decision(document.Decision),
		Rename: endpointrepair.EndpointRename{
			Method: document.EndpointRename.Method, OperationID: document.EndpointRename.OperationID,
			PreviousPath: document.EndpointRename.PreviousPath, Path: document.EndpointRename.Path,
			Capabilities: append([]string{}, document.EndpointRename.Capabilities...),
		},
		AdapterID: document.Adapter.ID, AdapterVersion: document.Adapter.Version,
		Edit: adaptation.TextEdit{
			Path: document.Edit.Path, BeforeSHA256: document.Edit.BeforeSHA256,
			StartByte: document.Edit.StartByte, EndByte: document.Edit.EndByte,
			Original: document.Edit.Original, Replacement: document.Edit.Replacement,
			SemanticRole: document.Edit.SemanticRole,
		},
	}
	proposal.Test = adaptation.CanonicalTestReference(proposal.Test)
	proposal.Rename = endpointrepair.CanonicalEndpointRename(proposal.Rename)
	if err := endpointrepair.ValidateProposal(proposal); err != nil {
		return endpointrepair.Proposal{}, fmt.Errorf("validate imported adaptation proposal: %w", err)
	}

	return proposal, nil
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

func exportRename(rename endpointrepair.EndpointRename) contracts.EndpointRenameEvidence {
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

func importTest(test contracts.AdaptationTestReference) adaptation.TestReference {
	return adaptation.TestReference{
		Repository: importRepository(test.Repository), Revision: importRevision(test.Revision),
		SuiteKey: test.SuiteKey, TestKey: test.TestKey, Name: test.Name, Adapter: test.Adapter,
		Capabilities: append([]string{}, test.Capabilities...),
	}
}

func importRepository(repository contracts.RepositoryReference) catalog.Repository {
	return catalog.Repository{
		Identity: catalog.RepositoryIdentity{
			Provider: catalog.Provider(repository.Provider), Host: repository.Host,
			ProviderRepositoryID: repository.ProviderRepositoryID,
		},
		Owner: repository.Owner, Name: repository.Name,
	}
}

func importRevision(revision contracts.RevisionReference) catalog.Revision {
	return catalog.Revision{Algorithm: catalog.RevisionAlgorithm(revision.Algorithm), Digest: revision.Digest}
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
