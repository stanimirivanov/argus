package contract

import (
	"fmt"
	"time"

	"github.com/stanimirivanov/argus/contracts"
	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
	"github.com/stanimirivanov/argus/internal/selection"
)

// ImportV1 converts a structurally validated transport document into a
// canonical domain manifest and enforces cross-field selection invariants.
func ImportV1(document contracts.ExecutionManifestV1) (selection.Manifest, error) {
	if err := contracts.ValidateExecutionManifestV1(document); err != nil {
		return selection.Manifest{}, err
	}
	observedAt, err := time.Parse(time.RFC3339Nano, document.Change.ObservedAt)
	if err != nil {
		return selection.Manifest{}, fmt.Errorf("parse manifest observation time: %w", err)
	}
	manifest := selection.Manifest{
		APIVersion: document.APIVersion, PolicyVersion: document.PolicyVersion,
		ImpactAPIVersion:      document.Impact.APIVersion,
		ImpactAnalyzerVersion: document.Impact.AnalyzerVersion,
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
		Catalog: catalog.SnapshotReference{
			SourceRepository:     importRepository(document.Catalog.SourceRepository),
			Revision:             importRevision(document.Catalog.Revision),
			DescriptorAPIVersion: document.Catalog.DescriptorAPIVersion,
		},
		Family: catalog.TestFamily(document.Family), Mode: selection.Mode(document.Mode),
		AffectedCapabilities:  append([]string{}, document.AffectedCapabilities...),
		UncoveredCapabilities: append([]string{}, document.UncoveredCapabilities...),
		Warnings:              append([]string{}, document.Warnings...),
	}
	for _, decision := range document.Decisions {
		manifest.Decisions = append(manifest.Decisions, importDecision(decision))
	}
	manifest = selection.CanonicalManifest(manifest)
	if err := selection.ValidateManifest(manifest); err != nil {
		return selection.Manifest{}, fmt.Errorf("validate imported execution manifest: %w", err)
	}

	return manifest, nil
}

func importDecision(document contracts.TestDecision) selection.Decision {
	reasons := make([]selection.Reason, 0, len(document.Reasons))
	for _, reason := range document.Reasons {
		reasons = append(reasons, selection.Reason{
			Code:         selection.ReasonCode(reason.Code),
			Capabilities: append([]string{}, reason.Capabilities...),
		})
	}

	return selection.Decision{
		Test: selection.TestReference{
			Repository: importRepository(document.TestRepository), SuiteKey: document.Suite.Key,
			Family: catalog.TestFamily(document.Suite.Family), Adapter: document.Suite.Adapter,
			TestKey: document.Test.Key, Name: document.Test.Name,
			Capabilities: append([]string{}, document.Capabilities...),
		},
		Outcome:            selection.Outcome(document.Outcome),
		RemainingExecution: selection.RemainingExecution(document.RemainingExecution),
		Reasons:            reasons,
	}
}

func importRepository(reference contracts.RepositoryReference) catalog.Repository {
	return catalog.Repository{
		Identity: catalog.RepositoryIdentity{
			Provider: catalog.Provider(reference.Provider), Host: reference.Host,
			ProviderRepositoryID: reference.ProviderRepositoryID,
		},
		Owner: reference.Owner, Name: reference.Name,
	}
}

func importRevision(reference contracts.RevisionReference) catalog.Revision {
	return catalog.Revision{Algorithm: catalog.RevisionAlgorithm(reference.Algorithm), Digest: reference.Digest}
}
