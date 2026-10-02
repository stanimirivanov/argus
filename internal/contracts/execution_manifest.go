package contracts

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	// ExecutionManifestV1APIVersion identifies the first selection result contract.
	ExecutionManifestV1APIVersion = "argus.dev/execution-manifest/v1"
	executionManifestV1SchemaID   = "https://argus.dev/contracts/execution-manifest/v1/schema.json"
	// ExecutionManifestV2APIVersion identifies browser capability selection.
	ExecutionManifestV2APIVersion = "argus.dev/execution-manifest/v2"
	executionManifestV2SchemaID   = "https://argus.dev/contracts/execution-manifest/v2/schema.json"
)

var executionManifestV1SchemaJSON = mustReadSchema("execution-manifest/v1/execution-manifest.schema.json")

var loadExecutionManifestV1Schema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(executionManifestV1SchemaJSON, executionManifestV1SchemaID, "execution manifest")
})

var executionManifestV2SchemaJSON = mustReadSchema("execution-manifest/v2/execution-manifest.schema.json")

var loadExecutionManifestV2Schema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(executionManifestV2SchemaJSON, executionManifestV2SchemaID, "browser execution manifest")
})

// ExecutionManifestV1 is the versioned functional API selection result.
type ExecutionManifestV1 struct {
	APIVersion            string                   `json:"apiVersion"`
	PolicyVersion         string                   `json:"policyVersion"`
	Impact                ManifestImpactReference  `json:"impact"`
	Change                ManifestChangeReference  `json:"change"`
	Catalog               CatalogSnapshotReference `json:"catalog"`
	Family                string                   `json:"family"`
	Mode                  string                   `json:"mode"`
	AffectedCapabilities  []string                 `json:"affectedCapabilities"`
	UncoveredCapabilities []string                 `json:"uncoveredCapabilities"`
	Decisions             []TestDecision           `json:"decisions"`
	Warnings              []string                 `json:"warnings"`
}

// ExecutionManifestV2 is the versioned browser capability selection result.
// It intentionally has the same field shape as v1 but distinct family and
// policy invariants enforced by its own Effect-authored schema.
type ExecutionManifestV2 ExecutionManifestV1

// ManifestImpactReference identifies the analyzer contract used for selection.
type ManifestImpactReference struct {
	APIVersion      string `json:"apiVersion"`
	AnalyzerVersion string `json:"analyzerVersion"`
}

// ManifestChangeReference identifies the immutable pull-request comparison.
type ManifestChangeReference struct {
	SourceRepository RepositoryReference  `json:"sourceRepository"`
	PullRequest      PullRequestReference `json:"pullRequest"`
	BaseRevision     RevisionReference    `json:"baseRevision"`
	HeadRevision     RevisionReference    `json:"headRevision"`
	ObservedAt       string               `json:"observedAt"`
	Trigger          ChangeTrigger        `json:"trigger"`
}

// TestDecision contains one inclusion or omission with reasons.
type TestDecision struct {
	TestRepository     RepositoryReference       `json:"testRepository"`
	Suite              TestCatalogSuiteReference `json:"suite"`
	Test               TestCatalogTestReference  `json:"test"`
	Capabilities       []string                  `json:"capabilities"`
	Outcome            string                    `json:"outcome"`
	RemainingExecution string                    `json:"remainingExecution"`
	Reasons            []SelectionReason         `json:"reasons"`
}

// SelectionReason is a stable policy explanation.
type SelectionReason struct {
	Code         string   `json:"code"`
	Capabilities []string `json:"capabilities"`
}

// DecodeExecutionManifestV1 validates and decodes an external manifest.
func DecodeExecutionManifestV1(data []byte) (ExecutionManifestV1, error) {
	var document ExecutionManifestV1
	if err := validateExecutionManifestV1JSON(data); err != nil {
		return document, err
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return document, fmt.Errorf("decode execution manifest: %w", err)
	}

	return document, nil
}

// ValidateExecutionManifestV1 verifies a typed document against Effect-authored JSON Schema.
func ValidateExecutionManifestV1(document ExecutionManifestV1) error {
	data, err := json.Marshal(document)
	if err != nil {
		return fmt.Errorf("encode execution manifest for validation: %w", err)
	}

	return validateExecutionManifestV1JSON(data)
}

func validateExecutionManifestV1JSON(data []byte) error {
	schema, err := loadExecutionManifestV1Schema()
	if err != nil {
		return err
	}
	untyped, err := decodeSingleJSONValue(data, "execution manifest")
	if err != nil {
		return err
	}
	if err := schema.Validate(untyped); err != nil {
		return fmt.Errorf("validate execution manifest schema: %w", err)
	}

	return nil
}

// DecodeExecutionManifestV2 validates and decodes a browser selection manifest.
func DecodeExecutionManifestV2(data []byte) (ExecutionManifestV2, error) {
	var document ExecutionManifestV2
	if err := validateExecutionManifestV2JSON(data); err != nil {
		return document, err
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return document, fmt.Errorf("decode browser execution manifest: %w", err)
	}

	return document, nil
}

// ValidateExecutionManifestV2 checks a typed value against the v2 schema.
func ValidateExecutionManifestV2(document ExecutionManifestV2) error {
	data, err := json.Marshal(document)
	if err != nil {
		return fmt.Errorf("encode browser execution manifest for validation: %w", err)
	}

	return validateExecutionManifestV2JSON(data)
}

func validateExecutionManifestV2JSON(data []byte) error {
	schema, err := loadExecutionManifestV2Schema()
	if err != nil {
		return err
	}
	untyped, err := decodeSingleJSONValue(data, "browser execution manifest")
	if err != nil {
		return err
	}
	if err := schema.Validate(untyped); err != nil {
		return fmt.Errorf("validate browser execution manifest schema: %w", err)
	}

	return nil
}
