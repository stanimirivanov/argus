// Package evidence converts versioned impact-evidence transports into catalog
// domain values and enforces semantic invariants at the ingress boundary.
package evidence

import (
	"fmt"
	"time"

	"github.com/stanimirivanov/argus/contracts"
	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/catalog/impact"
)

// Import converts a structurally validated v1 bundle into a validated catalog
// value. Timestamp ordering, uniqueness, and closed-set semantics are domain
// concerns rather than JSON Schema constraints.
func Import(document contracts.ImpactEvidenceBundleV1) (impact.EvidenceBundle, error) {
	observedAt, err := parseUTC(document.ObservedAt, "/observedAt")
	if err != nil {
		return impact.EvidenceBundle{}, err
	}

	var expiresAt *time.Time
	if document.ExpiresAt != nil {
		parsed, err := parseUTC(*document.ExpiresAt, "/expiresAt")
		if err != nil {
			return impact.EvidenceBundle{}, err
		}
		expiresAt = &parsed
	}

	bundle := impact.EvidenceBundle{
		APIVersion: document.APIVersion,
		Snapshot: catalog.SnapshotReference{
			SourceRepository:     repository(document.Snapshot.SourceRepository),
			Revision:             revision(document.Snapshot.Revision),
			DescriptorAPIVersion: document.Snapshot.DescriptorAPIVersion,
		},
		Producer: impact.EvidenceProducer{
			Repository: repository(document.Producer.Repository),
			Revision:   revision(document.Producer.Revision),
			Adapter:    document.Producer.Adapter,
		},
		ObservedAt:   observedAt,
		ExpiresAt:    expiresAt,
		Observations: make([]impact.Observation, len(document.Observations)),
	}
	for index, observation := range document.Observations {
		bundle.Observations[index] = impact.Observation{
			Key:           observation.Key,
			CapabilityKey: observation.CapabilityKey,
			Test: catalog.TestIdentity{
				TestRepository: catalog.RepositoryIdentity{
					Provider:             catalog.Provider(observation.Test.TestRepository.Provider),
					Host:                 observation.Test.TestRepository.Host,
					ProviderRepositoryID: observation.Test.TestRepository.ProviderRepositoryID,
				},
				SuiteKey: observation.Test.SuiteKey,
				TestKey:  observation.Test.TestKey,
			},
			Assertion:             impact.Assertion(observation.Assertion),
			EvidenceType:          impact.EvidenceType(observation.EvidenceType),
			ConfidenceBasisPoints: observation.ConfidenceBasisPoints,
			Rationale:             observation.Rationale,
		}
	}
	if err := impact.ValidateEvidenceBundle(bundle); err != nil {
		return impact.EvidenceBundle{}, err
	}

	return impact.CanonicalEvidenceBundle(bundle), nil
}

func parseUTC(value, path string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s: parse UTC timestamp: %w", path, err)
	}
	_, offsetSeconds := parsed.Zone()
	if offsetSeconds != 0 {
		return time.Time{}, fmt.Errorf("%s: timestamp must be UTC", path)
	}

	return parsed.UTC(), nil
}

func repository(reference contracts.RepositoryReference) catalog.Repository {
	return catalog.Repository{
		Identity: catalog.RepositoryIdentity{
			Provider:             catalog.Provider(reference.Provider),
			Host:                 reference.Host,
			ProviderRepositoryID: reference.ProviderRepositoryID,
		},
		Owner: reference.Owner,
		Name:  reference.Name,
	}
}

func revision(reference contracts.RevisionReference) catalog.Revision {
	return catalog.Revision{
		Algorithm: catalog.RevisionAlgorithm(reference.Algorithm),
		Digest:    reference.Digest,
	}
}
