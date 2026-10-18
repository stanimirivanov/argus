package contract

import (
	"time"

	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/selection/browsermapped"
)

// ExportBrowserMapped converts a browser-owned decision to its distinct wire
// contract. Functional API manifest consumers cannot mistake it for v1-v4.
func ExportBrowserMapped(manifest browsermapped.Manifest) (contracts.BrowserSelectionManifestV1, error) {
	document := contracts.BrowserSelectionManifestV1{
		APIVersion: manifest.APIVersion, PolicyVersion: manifest.PolicyVersion,
		Change:       changeReference(manifest.Impact.Change),
		ChangedPaths: append([]string{}, manifest.Impact.Paths...), PathsComplete: manifest.Impact.Complete,
		Catalog: contracts.CatalogSnapshotReference{SourceRepository: repositoryReference(manifest.Catalog.SourceRepository),
			Revision: revisionReference(manifest.Catalog.Revision), DescriptorAPIVersion: manifest.Catalog.DescriptorAPIVersion},
		EvaluatedAt: manifest.EvaluatedAt.Format(time.RFC3339Nano), Family: "functional-ui", Mode: string(manifest.Mode),
		Decisions: make([]contracts.BrowserSelectionDecision, 0, len(manifest.Decisions)),
		Warnings:  append([]string{}, manifest.Warnings...),
	}
	if manifest.Evidence != nil {
		document.Evidence = &contracts.BrowserSelectionEvidenceReference{
			ObservationID: manifest.Evidence.ObservationID, Digest: manifest.Evidence.Digest,
			CollectorVersion: manifest.Evidence.CollectorVersion,
			MappingRevision:  revisionReference(manifest.Evidence.MappingRevision),
		}
	}
	for _, decision := range manifest.Decisions {
		document.Decisions = append(document.Decisions, contracts.BrowserSelectionDecision{
			TestRepository: repositoryReference(decision.Repository), SuiteKey: decision.SuiteKey,
			TestKey: decision.TestKey, Name: decision.Name, Revision: revisionReference(decision.Revision),
			Projects: append([]string{}, decision.Projects...), Outcome: string(decision.Outcome),
			RemainingExecution: string(decision.RemainingExecution), Reason: decision.Reason,
			MappingKeys: append([]string{}, decision.MappingKeys...),
		})
	}
	if err := contracts.ValidateBrowserSelectionManifestV1(document); err != nil {
		return contracts.BrowserSelectionManifestV1{}, err
	}

	return document, nil
}
