package contract

import (
	"fmt"
	"time"

	"github.com/stanimirivanov/argus/contracts"
	"github.com/stanimirivanov/argus/internal/change"
)

// ExportCapabilityImpactV1 converts validated domain evidence to its public contract.
func ExportCapabilityImpactV1(impact change.CapabilityImpact) (contracts.CapabilityImpactV1, error) {
	impact = change.CanonicalCapabilityImpact(impact)
	if err := change.ValidateCapabilityImpact(impact); err != nil {
		return contracts.CapabilityImpactV1{}, err
	}
	documents := make([]contracts.OpenAPIDocumentImpact, 0, len(impact.Documents))
	for _, document := range impact.Documents {
		operations := make([]contracts.OpenAPIOperationImpact, 0, len(document.Operations))
		for _, operation := range document.Operations {
			operations = append(operations, contracts.OpenAPIOperationImpact{
				Method: operation.Method, Path: operation.Path, OperationID: cloneString(operation.OperationID),
				Kind: string(operation.Kind), Capabilities: append([]string{}, operation.Capabilities...),
				PotentiallyBreaking: operation.PotentialBreak,
			})
		}
		documents = append(documents, contracts.OpenAPIDocumentImpact{
			Path: document.Path, PreviousPath: cloneString(document.PreviousPath), Kind: string(document.Kind),
			TotalChanges: document.TotalChanges, BreakingChanges: document.BreakingChanges, Operations: operations,
		})
	}
	reference := impact.Change
	result := contracts.CapabilityImpactV1{
		APIVersion: impact.APIVersion, AnalyzerVersion: impact.AnalyzerVersion,
		SourceRepository: contracts.RepositoryReference{
			Provider:             string(reference.SourceRepository.Identity.Provider),
			Host:                 reference.SourceRepository.Identity.Host,
			ProviderRepositoryID: reference.SourceRepository.Identity.ProviderRepositoryID,
			Owner:                reference.SourceRepository.Owner, Name: reference.SourceRepository.Name,
		},
		PullRequest: contracts.PullRequestReference{Number: reference.PullRequestNumber},
		BaseRevision: contracts.RevisionReference{
			Algorithm: string(reference.BaseRevision.Algorithm), Digest: reference.BaseRevision.Digest,
		},
		HeadRevision: contracts.RevisionReference{
			Algorithm: string(reference.HeadRevision.Algorithm), Digest: reference.HeadRevision.Digest,
		},
		ObservedAt: reference.ObservedAt.Format(time.RFC3339Nano),
		Trigger: contracts.ChangeTrigger{
			Provider: string(reference.Trigger.Provider), DeliveryID: reference.Trigger.DeliveryID,
			Event: reference.Trigger.Event, Action: reference.Trigger.Action,
		},
		Status: string(impact.Status), Documents: documents, Warnings: append([]string{}, impact.Warnings...),
	}
	if err := contracts.ValidateCapabilityImpactV1(result); err != nil {
		return contracts.CapabilityImpactV1{}, fmt.Errorf("export capability impact v1: %w", err)
	}

	return result, nil
}
