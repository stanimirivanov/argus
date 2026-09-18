package contracts

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	// CapabilityImpactV1APIVersion identifies the first semantic API impact contract.
	CapabilityImpactV1APIVersion = "argus.dev/capability-impact/v1"
	capabilityImpactV1SchemaID   = "https://argus.dev/contracts/capability-impact/v1/schema.json"
)

//go:embed generated/capability-impact/v1/capability-impact.schema.json
var capabilityImpactV1SchemaJSON []byte

var loadCapabilityImpactV1Schema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(capabilityImpactV1SchemaJSON, capabilityImpactV1SchemaID, "capability impact")
})

// CapabilityImpactV1 is the versioned transport representation of semantic
// OpenAPI impact and explicit capability mappings.
type CapabilityImpactV1 struct {
	APIVersion       string                  `json:"apiVersion"`
	AnalyzerVersion  string                  `json:"analyzerVersion"`
	SourceRepository RepositoryReference     `json:"sourceRepository"`
	PullRequest      PullRequestReference    `json:"pullRequest"`
	BaseRevision     RevisionReference       `json:"baseRevision"`
	HeadRevision     RevisionReference       `json:"headRevision"`
	ObservedAt       string                  `json:"observedAt"`
	Trigger          ChangeTrigger           `json:"trigger"`
	Status           string                  `json:"status"`
	Documents        []OpenAPIDocumentImpact `json:"documents"`
	Warnings         []string                `json:"warnings"`
}

// OpenAPIDocumentImpact summarizes semantic changes within one API document.
type OpenAPIDocumentImpact struct {
	Path            string                   `json:"path"`
	PreviousPath    *string                  `json:"previousPath"`
	Kind            string                   `json:"kind"`
	TotalChanges    int                      `json:"totalChanges"`
	BreakingChanges int                      `json:"breakingChanges"`
	Operations      []OpenAPIOperationImpact `json:"operations"`
}

// OpenAPIOperationImpact is one changed operation and its explicit mappings.
type OpenAPIOperationImpact struct {
	Method              string   `json:"method"`
	Path                string   `json:"path"`
	OperationID         *string  `json:"operationId"`
	Kind                string   `json:"kind"`
	Capabilities        []string `json:"capabilities"`
	PotentiallyBreaking bool     `json:"potentiallyBreaking"`
}

// ValidateCapabilityImpactV1 verifies a typed document against the generated schema.
func ValidateCapabilityImpactV1(document CapabilityImpactV1) error {
	data, err := json.Marshal(document)
	if err != nil {
		return fmt.Errorf("encode capability impact for validation: %w", err)
	}
	schema, err := loadCapabilityImpactV1Schema()
	if err != nil {
		return err
	}
	untyped, err := decodeSingleJSONValue(data, "capability impact")
	if err != nil {
		return err
	}
	if err := schema.Validate(untyped); err != nil {
		return fmt.Errorf("validate capability impact schema: %w", err)
	}

	return nil
}
