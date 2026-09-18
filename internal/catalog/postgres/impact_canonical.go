package postgres

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
)

// The impact fingerprint is a private persistence encoding. Keep it explicit:
// changing public DTOs or domain layout must not change existing bundle hashes.
type fingerprintImpactBundle struct {
	APIVersion   string                         `json:"apiVersion"`
	Snapshot     fingerprintImpactSnapshot      `json:"snapshot"`
	Producer     fingerprintImpactProducer      `json:"producer"`
	ObservedAt   string                         `json:"observedAt"`
	ExpiresAt    *string                        `json:"expiresAt"`
	Observations []fingerprintImpactObservation `json:"observations"`
}

type fingerprintImpactSnapshot struct {
	SourceRepository     fingerprintRepository `json:"sourceRepository"`
	Revision             fingerprintRevision   `json:"revision"`
	DescriptorAPIVersion string                `json:"descriptorApiVersion"`
}

type fingerprintImpactProducer struct {
	Repository fingerprintRepository `json:"repository"`
	Revision   fingerprintRevision   `json:"revision"`
	Adapter    string                `json:"adapter"`
}

type fingerprintImpactObservation struct {
	Key                   string                        `json:"key"`
	CapabilityKey         string                        `json:"capabilityKey"`
	Test                  fingerprintImpactTestIdentity `json:"test"`
	Assertion             catalog.ImpactAssertion       `json:"assertion"`
	EvidenceType          catalog.ImpactEvidenceType    `json:"evidenceType"`
	ConfidenceBasisPoints int                           `json:"confidenceBasisPoints"`
	Rationale             string                        `json:"rationale"`
}

type fingerprintImpactTestIdentity struct {
	TestRepository fingerprintRepositoryIdentity `json:"testRepository"`
	SuiteKey       string                        `json:"suiteKey"`
	TestKey        string                        `json:"testKey"`
}

func impactEvidenceFingerprint(bundle catalog.ImpactEvidenceBundle) (string, error) {
	observations := make([]fingerprintImpactObservation, len(bundle.Observations))
	for index, observation := range bundle.Observations {
		observations[index] = fingerprintImpactObservation{
			Key:           observation.Key,
			CapabilityKey: observation.CapabilityKey,
			Test: fingerprintImpactTestIdentity{
				TestRepository: fingerprintRepositoryIdentity(observation.Test.TestRepository),
				SuiteKey:       observation.Test.SuiteKey,
				TestKey:        observation.Test.TestKey,
			},
			Assertion:             observation.Assertion,
			EvidenceType:          observation.EvidenceType,
			ConfidenceBasisPoints: observation.ConfidenceBasisPoints,
			Rationale:             observation.Rationale,
		}
	}

	var expiresAt *string
	if bundle.ExpiresAt != nil {
		formatted := bundle.ExpiresAt.UTC().Format(time.RFC3339Nano)
		expiresAt = &formatted
	}
	document := fingerprintImpactBundle{
		APIVersion: bundle.APIVersion,
		Snapshot: fingerprintImpactSnapshot{
			SourceRepository:     fingerprintRepositoryFrom(bundle.Snapshot.SourceRepository),
			Revision:             fingerprintRevision(bundle.Snapshot.Revision),
			DescriptorAPIVersion: bundle.Snapshot.DescriptorAPIVersion,
		},
		Producer: fingerprintImpactProducer{
			Repository: fingerprintRepositoryFrom(bundle.Producer.Repository),
			Revision:   fingerprintRevision(bundle.Producer.Revision),
			Adapter:    bundle.Producer.Adapter,
		},
		ObservedAt:   bundle.ObservedAt.UTC().Format(time.RFC3339Nano),
		ExpiresAt:    expiresAt,
		Observations: observations,
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)

	return hex.EncodeToString(digest[:]), nil
}
