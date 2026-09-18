package contracts

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	// ImpactEdgePageV1APIVersion identifies the first versioned impact-edge
	// query result contract.
	ImpactEdgePageV1APIVersion = "argus.dev/impact-edge-page/v1"
	impactEdgePageV1SchemaID   = "https://argus.dev/contracts/impact-edge-page/v1/schema.json"
)

//go:embed generated/impact-edge-page/v1/impact-edge-page.schema.json
var impactEdgePageV1SchemaJSON []byte

var loadImpactEdgePageV1Schema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(
		impactEdgePageV1SchemaJSON,
		impactEdgePageV1SchemaID,
		"impact edge page",
	)
})

// ImpactEdgePageV1 is a deterministic page of relationships evaluated at an
// explicit instant.
type ImpactEdgePageV1 struct {
	APIVersion  string                   `json:"apiVersion"`
	Snapshot    CatalogSnapshotReference `json:"snapshot"`
	EvaluatedAt string                   `json:"evaluatedAt"`
	Items       []ImpactEdge             `json:"items"`
	NextCursor  *string                  `json:"nextCursor"`
}

// ImpactEdge combines one stable capability-to-test relation with all
// evidence visible at the evaluation time.
type ImpactEdge struct {
	Capability     CapabilityReference       `json:"capability"`
	TestRepository RepositoryReference       `json:"testRepository"`
	Suite          TestCatalogSuiteReference `json:"suite"`
	Test           TestCatalogTestReference  `json:"test"`
	Status         string                    `json:"status"`
	Evidence       []ImpactEvidenceReference `json:"evidence"`
	Conflict       *MappingConflict          `json:"conflict"`
}

// ImpactEvidenceReference describes one visible observation and whether it is
// active or expired at the requested evaluation time.
type ImpactEvidenceReference struct {
	Producer              ImpactEvidenceProducer `json:"producer"`
	ObservationKey        string                 `json:"observationKey"`
	Assertion             string                 `json:"assertion"`
	EvidenceType          string                 `json:"evidenceType"`
	ConfidenceBasisPoints int                    `json:"confidenceBasisPoints"`
	Rationale             string                 `json:"rationale"`
	ObservedAt            string                 `json:"observedAt"`
	ExpiresAt             *string                `json:"expiresAt"`
	State                 string                 `json:"state"`
}

// MappingConflict reports contradictory active assertions without selecting a
// winner or discarding either source.
type MappingConflict struct {
	Kind                    string `json:"kind"`
	SupportingEvidenceCount int    `json:"supportingEvidenceCount"`
	RefutingEvidenceCount   int    `json:"refutingEvidenceCount"`
}

// DecodeImpactEdgePageV1 validates and decodes an external impact page.
func DecodeImpactEdgePageV1(data []byte) (ImpactEdgePageV1, error) {
	var document ImpactEdgePageV1

	if err := validateImpactEdgePageV1JSON(data); err != nil {
		return document, err
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return document, fmt.Errorf("decode impact edge page: %w", err)
	}

	return document, nil
}

// ValidateImpactEdgePageV1 verifies that a typed page conforms to the
// Effect-authored result contract before it crosses a transport boundary.
func ValidateImpactEdgePageV1(document ImpactEdgePageV1) error {
	data, err := json.Marshal(document)
	if err != nil {
		return fmt.Errorf("encode impact edge page for validation: %w", err)
	}

	return validateImpactEdgePageV1JSON(data)
}

func validateImpactEdgePageV1JSON(data []byte) error {
	schema, err := loadImpactEdgePageV1Schema()
	if err != nil {
		return err
	}
	document, err := decodeSingleJSONValue(data, "impact edge page")
	if err != nil {
		return err
	}
	if err := schema.Validate(document); err != nil {
		return fmt.Errorf("validate impact edge page schema: %w", err)
	}

	return nil
}
