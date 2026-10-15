package contracts

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// CapabilityImpactV2APIVersion identifies the composite impact wire contract.
const CapabilityImpactV2APIVersion = "argus.dev/capability-impact/v2"

var capabilityImpactV2SchemaJSON = mustReadSchema("capability-impact/v2/capability-impact.schema.json")

var loadCapabilityImpactV2Schema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(capabilityImpactV2SchemaJSON,
		"https://argus.dev/contracts/capability-impact/v2/schema.json", "capability impact v2")
})

// CapabilityImpactV2 preserves the v1 semantic fields and adds changed-file
// coverage. The embedded fields marshal at the top level of the wire document.
type CapabilityImpactV2 struct {
	CapabilityImpactV1
	SemanticStatus   string              `json:"semanticStatus"`
	FileStatus       string              `json:"fileStatus"`
	SemanticWarnings []string            `json:"semanticWarnings"`
	FileWarnings     []string            `json:"fileWarnings"`
	Files            []ChangedFileImpact `json:"files"`
}

// ChangedFileImpact preserves the coverage and mapping evidence for one file.
type ChangedFileImpact struct {
	Path           string   `json:"path"`
	PreviousPath   *string  `json:"previousPath"`
	Kind           string   `json:"kind"`
	Coverage       string   `json:"coverage"`
	Capabilities   []string `json:"capabilities"`
	ComponentRoots []string `json:"componentRoots"`
	UnmappedPaths  []string `json:"unmappedPaths"`
}

// ValidateCapabilityImpactV2 checks a document against the published schema.
func ValidateCapabilityImpactV2(document CapabilityImpactV2) error {
	data, err := json.Marshal(document)
	if err != nil {
		return fmt.Errorf("encode capability impact v2: %w", err)
	}

	return validateCapabilityImpactV2JSON(data)
}

// DecodeCapabilityImpactV2 validates and decodes a published v2 document.
func DecodeCapabilityImpactV2(data []byte) (CapabilityImpactV2, error) {
	var document CapabilityImpactV2
	if err := validateCapabilityImpactV2JSON(data); err != nil {
		return document, err
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return document, fmt.Errorf("decode capability impact v2: %w", err)
	}

	return document, nil
}

func validateCapabilityImpactV2JSON(data []byte) error {
	schema, err := loadCapabilityImpactV2Schema()
	if err != nil {
		return err
	}
	untyped, err := decodeSingleJSONValue(data, "capability impact v2")
	if err != nil {
		return err
	}
	if err := schema.Validate(untyped); err != nil {
		return fmt.Errorf("validate capability impact v2 schema: %w", err)
	}

	return nil
}
