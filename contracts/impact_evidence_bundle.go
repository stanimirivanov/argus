package contracts

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	// ImpactEvidenceBundleV1APIVersion identifies immutable capability-to-test
	// evidence produced for one catalog snapshot.
	ImpactEvidenceBundleV1APIVersion = "argus.dev/impact-evidence-bundle/v1"
	impactEvidenceBundleV1SchemaID   = "https://argus.dev/contracts/impact-evidence-bundle/v1/schema.json"
)

//go:embed generated/impact-evidence-bundle/v1/impact-evidence-bundle.schema.json
var impactEvidenceBundleV1SchemaJSON []byte

var loadImpactEvidenceBundleV1Schema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(
		impactEvidenceBundleV1SchemaJSON,
		impactEvidenceBundleV1SchemaID,
		"impact evidence bundle",
	)
})

// ImpactEvidenceBundleV1 is the versioned transport representation of one
// producer's immutable observations for a catalog snapshot.
type ImpactEvidenceBundleV1 struct {
	APIVersion   string                      `json:"apiVersion"`
	Snapshot     CatalogSnapshotReference    `json:"snapshot"`
	Producer     ImpactEvidenceProducer      `json:"producer"`
	ObservedAt   string                      `json:"observedAt"`
	ExpiresAt    *string                     `json:"expiresAt"`
	Observations []ImpactEvidenceObservation `json:"observations"`
}

// ImpactEvidenceProducer identifies the repository revision and adapter that
// produced an evidence bundle.
type ImpactEvidenceProducer struct {
	Repository RepositoryReference `json:"repository"`
	Revision   RevisionReference   `json:"revision"`
	Adapter    string              `json:"adapter"`
}

// ImpactTestIdentity identifies one stable test in the referenced snapshot.
type ImpactTestIdentity struct {
	TestRepository RepositoryIdentityReference `json:"testRepository"`
	SuiteKey       string                      `json:"suiteKey"`
	TestKey        string                      `json:"testKey"`
}

// RepositoryIdentityReference is a transport-only stable repository identity
// without mutable display coordinates.
type RepositoryIdentityReference struct {
	Provider             string `json:"provider"`
	Host                 string `json:"host"`
	ProviderRepositoryID string `json:"providerRepositoryId"`
}

// ImpactEvidenceObservation is one producer assertion about a
// capability-to-test relationship.
type ImpactEvidenceObservation struct {
	Key                   string             `json:"key"`
	CapabilityKey         string             `json:"capabilityKey"`
	Test                  ImpactTestIdentity `json:"test"`
	Assertion             string             `json:"assertion"`
	EvidenceType          string             `json:"evidenceType"`
	ConfidenceBasisPoints int                `json:"confidenceBasisPoints"`
	Rationale             string             `json:"rationale"`
}

// DecodeImpactEvidenceBundleV1 validates and decodes an external evidence
// bundle without applying catalog-domain invariants.
func DecodeImpactEvidenceBundleV1(data []byte) (ImpactEvidenceBundleV1, error) {
	var document ImpactEvidenceBundleV1

	if err := validateImpactEvidenceBundleV1JSON(data); err != nil {
		return document, err
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return document, fmt.Errorf("decode impact evidence bundle: %w", err)
	}

	return document, nil
}

// ValidateImpactEvidenceBundleV1 verifies that a typed bundle still conforms
// to the Effect-authored transport contract.
func ValidateImpactEvidenceBundleV1(document ImpactEvidenceBundleV1) error {
	data, err := json.Marshal(document)
	if err != nil {
		return fmt.Errorf("encode impact evidence bundle for validation: %w", err)
	}

	return validateImpactEvidenceBundleV1JSON(data)
}

func validateImpactEvidenceBundleV1JSON(data []byte) error {
	schema, err := loadImpactEvidenceBundleV1Schema()
	if err != nil {
		return err
	}
	document, err := decodeSingleJSONValue(data, "impact evidence bundle")
	if err != nil {
		return err
	}
	if err := schema.Validate(document); err != nil {
		return fmt.Errorf("validate impact evidence bundle schema: %w", err)
	}

	return nil
}
