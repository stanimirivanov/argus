// Package browser converts portable browser discovery documents at the catalog boundary.
package browser

import (
	"fmt"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/catalog/discovery"
	"github.com/stanimirivanov/argus/internal/contracts"
)

// Import converts a schema-checked document; the caller must validate it
// against its immutable catalog snapshot before persistence.
func Import(document contracts.BrowserDiscoveryV1) (discovery.Evidence, error) {
	collectedAt, err := time.Parse(time.RFC3339Nano, document.CollectedAt)
	if err != nil {
		return discovery.Evidence{}, fmt.Errorf("collectedAt: %w", err)
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, document.ExpiresAt)
	if err != nil {
		return discovery.Evidence{}, fmt.Errorf("expiresAt: %w", err)
	}
	evidence := discovery.Evidence{
		APIVersion: document.APIVersion, ObservationID: document.ObservationID,
		Catalog: catalog.SnapshotReference{
			SourceRepository: repository(document.Catalog.SourceRepository),
			Revision:         revision(document.Catalog.Revision), DescriptorAPIVersion: document.Catalog.DescriptorAPIVersion,
		},
		CollectorVersion: document.CollectorVersion, CollectedAt: collectedAt.UTC(), ExpiresAt: expiresAt.UTC(),
		Complete: document.Complete, MappingRevision: revision(document.MappingRevision), MappingComplete: document.MappingComplete,
		Suites:   make([]discovery.SuiteObservation, 0, len(document.Suites)),
		Mappings: make([]discovery.Mapping, 0, len(document.Mappings)),
	}
	for _, suite := range document.Suites {
		observation := discovery.SuiteObservation{Repository: repository(suite.TestRepository), Revision: revision(suite.Revision),
			SuiteKey: suite.SuiteKey, Cases: make([]discovery.Case, 0, len(suite.Cases))}
		for _, item := range suite.Cases {
			observation.Cases = append(observation.Cases, discovery.Case{
				Key: item.TestKey, Project: item.Project, File: item.File, Title: item.Title,
				Tags: append([]string(nil), item.Tags...), Owners: append([]string(nil), item.Owners...),
			})
		}
		evidence.Suites = append(evidence.Suites, observation)
	}
	for _, mapping := range document.Mappings {
		evidence.Mappings = append(evidence.Mappings, discovery.Mapping{
			Kind: discovery.MappingKind(mapping.Kind), Key: mapping.Key, RoutePattern: mapping.RoutePattern,
			SourceRoot: mapping.SourceRoot, Test: catalog.TestIdentity{
				TestRepository: identity(mapping.TestRepository), SuiteKey: mapping.SuiteKey, TestKey: mapping.TestKey,
			},
		})
	}

	return evidence, nil
}

// Export converts validated discovery evidence to its portable v1 document.
func Export(evidence discovery.Evidence) (contracts.BrowserDiscoveryV1, error) {
	evidence = discovery.CanonicalEvidence(evidence)
	document := contracts.BrowserDiscoveryV1{
		APIVersion: evidence.APIVersion, ObservationID: evidence.ObservationID,
		Catalog: contracts.CatalogSnapshotReference{
			SourceRepository: repositoryReference(evidence.Catalog.SourceRepository),
			Revision:         revisionReference(evidence.Catalog.Revision), DescriptorAPIVersion: evidence.Catalog.DescriptorAPIVersion,
		},
		CollectorVersion: evidence.CollectorVersion, CollectedAt: evidence.CollectedAt.Format(time.RFC3339Nano),
		ExpiresAt: evidence.ExpiresAt.Format(time.RFC3339Nano), Complete: evidence.Complete,
		MappingRevision: revisionReference(evidence.MappingRevision), MappingComplete: evidence.MappingComplete,
		Suites:   make([]contracts.BrowserSuiteObservation, 0, len(evidence.Suites)),
		Mappings: make([]contracts.BrowserMapping, 0, len(evidence.Mappings)),
	}
	for _, suite := range evidence.Suites {
		observation := contracts.BrowserSuiteObservation{
			TestRepository: repositoryReference(suite.Repository), Revision: revisionReference(suite.Revision),
			SuiteKey: suite.SuiteKey, Cases: make([]contracts.BrowserCase, 0, len(suite.Cases)),
		}
		for _, item := range suite.Cases {
			observation.Cases = append(observation.Cases, contracts.BrowserCase{TestKey: item.Key, Project: item.Project,
				File: item.File, Title: item.Title, Tags: append([]string{}, item.Tags...), Owners: append([]string{}, item.Owners...)})
		}
		document.Suites = append(document.Suites, observation)
	}
	for _, mapping := range evidence.Mappings {
		document.Mappings = append(document.Mappings, contracts.BrowserMapping{
			Kind: string(mapping.Kind), Key: mapping.Key, RoutePattern: mapping.RoutePattern, SourceRoot: mapping.SourceRoot,
			TestRepository: identityReference(mapping.Test.TestRepository), SuiteKey: mapping.Test.SuiteKey, TestKey: mapping.Test.TestKey,
		})
	}
	if err := contracts.ValidateBrowserDiscoveryV1(document); err != nil {
		return contracts.BrowserDiscoveryV1{}, err
	}

	return document, nil
}

func repository(value contracts.RepositoryReference) catalog.Repository {
	return catalog.Repository{Identity: catalog.RepositoryIdentity{
		Provider: catalog.Provider(value.Provider), Host: value.Host, ProviderRepositoryID: value.ProviderRepositoryID,
	}, Owner: value.Owner, Name: value.Name}
}
func identity(value contracts.RepositoryIdentityReference) catalog.RepositoryIdentity {
	return catalog.RepositoryIdentity{Provider: catalog.Provider(value.Provider), Host: value.Host,
		ProviderRepositoryID: value.ProviderRepositoryID}
}
func revision(value contracts.RevisionReference) catalog.Revision {
	return catalog.Revision{Algorithm: catalog.RevisionAlgorithm(value.Algorithm), Digest: value.Digest}
}
func repositoryReference(value catalog.Repository) contracts.RepositoryReference {
	return contracts.RepositoryReference{Provider: string(value.Identity.Provider), Host: value.Identity.Host,
		ProviderRepositoryID: value.Identity.ProviderRepositoryID, Owner: value.Owner, Name: value.Name}
}
func identityReference(value catalog.RepositoryIdentity) contracts.RepositoryIdentityReference {
	return contracts.RepositoryIdentityReference{Provider: string(value.Provider), Host: value.Host,
		ProviderRepositoryID: value.ProviderRepositoryID}
}
func revisionReference(value catalog.Revision) contracts.RevisionReference {
	return contracts.RevisionReference{Algorithm: string(value.Algorithm), Digest: value.Digest}
}
