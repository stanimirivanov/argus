package contracts

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var maintenanceAssessmentV1SchemaJSON = mustReadSchema("maintenance-assessment/v1/maintenance-assessment.schema.json")

var loadMaintenanceAssessmentV1Schema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(maintenanceAssessmentV1SchemaJSON,
		"https://argus.dev/contracts/maintenance-assessment/v1/schema.json", "maintenance assessment")
})

// MaintenanceAssessmentV1 is conservative API maintenance evidence.
type MaintenanceAssessmentV1 struct {
	APIVersion            string                        `json:"apiVersion"`
	ImpactAPIVersion      string                        `json:"impactApiVersion"`
	ImpactAnalyzerVersion string                        `json:"impactAnalyzerVersion"`
	SourceRepositoryID    string                        `json:"sourceRepositoryId"`
	BaseRevision          string                        `json:"baseRevision"`
	HeadRevision          string                        `json:"headRevision"`
	PolicyDigest          string                        `json:"policyDigest"`
	InventoryDigest       string                        `json:"inventoryDigest"`
	Items                 []MaintenanceAssessmentItemV1 `json:"items"`
}

// MaintenanceAssessmentItemV1 addresses one test or one uncovered capability.
type MaintenanceAssessmentItemV1 struct {
	RepositoryID string   `json:"repositoryId"`
	SuiteKey     string   `json:"suiteKey"`
	TestKey      string   `json:"testKey"`
	Capability   string   `json:"capability"`
	State        string   `json:"state"`
	Reason       string   `json:"reason"`
	AttemptIDs   []string `json:"attemptIds"`
}

// DecodeMaintenanceAssessmentV1 enforces the generated public wire contract.
func DecodeMaintenanceAssessmentV1(data []byte) (MaintenanceAssessmentV1, error) {
	var document MaintenanceAssessmentV1
	schema, err := loadMaintenanceAssessmentV1Schema()
	if err != nil {
		return document, err
	}
	value, err := decodeSingleJSONValue(data, "maintenance assessment")
	if err != nil {
		return document, err
	}
	if err := schema.Validate(value); err != nil {
		return document, fmt.Errorf("validate maintenance assessment schema: %w", err)
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return document, err
	}

	return document, nil
}
