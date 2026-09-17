package main

import (
	"github.com/stanimirivanov/argus/contracts"
	"github.com/stanimirivanov/argus/internal/catalog"
)

func newTestCatalogPageOutput(page catalog.TestCatalogPage) contracts.TestCatalogPageV1 {
	items := make([]contracts.TestCatalogEntry, len(page.Items))
	for index, entry := range page.Items {
		capabilities := make([]contracts.CapabilityReference, len(entry.Capabilities))
		for capabilityIndex, capability := range entry.Capabilities {
			capabilities[capabilityIndex] = contracts.CapabilityReference{
				Key:  capability.Key,
				Name: capability.Name,
			}
		}
		items[index] = contracts.TestCatalogEntry{
			TestRepository: newContractRepositoryReference(entry.TestRepository),
			Suite: contracts.TestCatalogSuiteReference{
				Key:     entry.SuiteKey,
				Family:  string(entry.Family),
				Adapter: entry.Adapter,
			},
			Test: contracts.TestCatalogTestReference{
				Key:  entry.TestKey,
				Name: entry.Name,
			},
			Capabilities: capabilities,
		}
	}

	var nextCursor *string
	if page.NextCursor != "" {
		nextCursor = &page.NextCursor
	}

	return contracts.TestCatalogPageV1{
		APIVersion: contracts.TestCatalogPageV1APIVersion,
		Snapshot: contracts.CatalogSnapshotReference{
			SourceRepository: newContractRepositoryReference(page.Snapshot.SourceRepository),
			Revision: contracts.RevisionReference{
				Algorithm: string(page.Snapshot.Revision.Algorithm),
				Digest:    page.Snapshot.Revision.Digest,
			},
			DescriptorAPIVersion: page.Snapshot.DescriptorAPIVersion,
		},
		Items:      items,
		NextCursor: nextCursor,
	}
}

func newContractRepositoryReference(repository catalog.Repository) contracts.RepositoryReference {
	return contracts.RepositoryReference{
		Provider:             string(repository.Identity.Provider),
		Host:                 repository.Identity.Host,
		ProviderRepositoryID: repository.Identity.ProviderRepositoryID,
		Owner:                repository.Owner,
		Name:                 repository.Name,
	}
}
