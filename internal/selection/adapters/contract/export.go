// Package contract exports selection decisions through versioned transport DTOs.
package contract

import (
	"fmt"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/selection"
)

// ExportV1 converts a validated manifest to its public Effect-authored contract.
func ExportV1(manifest selection.Manifest) (contracts.ExecutionManifestV1, error) {
	if manifest.APIVersion != selection.ManifestAPIVersion {
		return contracts.ExecutionManifestV1{}, selection.ErrInvalid
	}
	result, err := exportManifest(manifest)
	if err != nil {
		return contracts.ExecutionManifestV1{}, err
	}
	if err := contracts.ValidateExecutionManifestV1(result); err != nil {
		return contracts.ExecutionManifestV1{}, fmt.Errorf("export execution manifest v1: %w", err)
	}

	return result, nil
}

// ExportV2 converts a functional UI manifest to its distinct v2 contract.
func ExportV2(manifest selection.Manifest) (contracts.ExecutionManifestV2, error) {
	if manifest.APIVersion != selection.BrowserManifestAPIVersion {
		return contracts.ExecutionManifestV2{}, selection.ErrInvalid
	}
	result, err := exportManifest(manifest)
	if err != nil {
		return contracts.ExecutionManifestV2{}, err
	}
	document := contracts.ExecutionManifestV2(result)
	if err := contracts.ValidateExecutionManifestV2(document); err != nil {
		return contracts.ExecutionManifestV2{}, fmt.Errorf("export execution manifest v2: %w", err)
	}

	return document, nil
}

func exportManifest(manifest selection.Manifest) (contracts.ExecutionManifestV1, error) {
	manifest = selection.CanonicalManifest(manifest)
	if err := selection.ValidateManifest(manifest); err != nil {
		return contracts.ExecutionManifestV1{}, err
	}
	decisions := make([]contracts.TestDecision, 0, len(manifest.Decisions))
	for _, decision := range manifest.Decisions {
		reasons := make([]contracts.SelectionReason, 0, len(decision.Reasons))
		for _, reason := range decision.Reasons {
			reasons = append(reasons, contracts.SelectionReason{
				Code: string(reason.Code), Capabilities: append([]string{}, reason.Capabilities...),
			})
		}
		decisions = append(decisions, contracts.TestDecision{
			TestRepository: repositoryReference(decision.Test.Repository),
			Suite: contracts.TestCatalogSuiteReference{
				Key: decision.Test.SuiteKey, Family: string(decision.Test.Family), Adapter: decision.Test.Adapter,
			},
			Test:         contracts.TestCatalogTestReference{Key: decision.Test.TestKey, Name: decision.Test.Name},
			Capabilities: append([]string{}, decision.Test.Capabilities...),
			Outcome:      string(decision.Outcome), RemainingExecution: string(decision.RemainingExecution),
			Reasons: reasons,
		})
	}
	result := contracts.ExecutionManifestV1{
		APIVersion: manifest.APIVersion, PolicyVersion: manifest.PolicyVersion,
		Impact: contracts.ManifestImpactReference{
			APIVersion: manifest.ImpactAPIVersion, AnalyzerVersion: manifest.ImpactAnalyzerVersion,
		},
		Change: changeReference(manifest.Change),
		Catalog: contracts.CatalogSnapshotReference{
			SourceRepository:     repositoryReference(manifest.Catalog.SourceRepository),
			Revision:             revisionReference(manifest.Catalog.Revision),
			DescriptorAPIVersion: manifest.Catalog.DescriptorAPIVersion,
		},
		Family: string(manifest.Family), Mode: string(manifest.Mode),
		AffectedCapabilities:  append([]string{}, manifest.AffectedCapabilities...),
		UncoveredCapabilities: append([]string{}, manifest.UncoveredCapabilities...),
		Decisions:             decisions, Warnings: append([]string{}, manifest.Warnings...),
	}

	return result, nil
}

func changeReference(reference change.Reference) contracts.ManifestChangeReference {
	return contracts.ManifestChangeReference{
		SourceRepository: repositoryReference(reference.SourceRepository),
		PullRequest:      contracts.PullRequestReference{Number: reference.PullRequestNumber},
		BaseRevision:     revisionReference(reference.BaseRevision),
		HeadRevision:     revisionReference(reference.HeadRevision),
		ObservedAt:       reference.ObservedAt.Format(time.RFC3339Nano),
		Trigger: contracts.ChangeTrigger{
			Provider: string(reference.Trigger.Provider), DeliveryID: reference.Trigger.DeliveryID,
			Event: reference.Trigger.Event, Action: reference.Trigger.Action,
		},
	}
}

func repositoryReference(repository catalog.Repository) contracts.RepositoryReference {
	return contracts.RepositoryReference{
		Provider: string(repository.Identity.Provider), Host: repository.Identity.Host,
		ProviderRepositoryID: repository.Identity.ProviderRepositoryID,
		Owner:                repository.Owner, Name: repository.Name,
	}
}

func revisionReference(revision catalog.Revision) contracts.RevisionReference {
	return contracts.RevisionReference{Algorithm: string(revision.Algorithm), Digest: revision.Digest}
}
