package contracts

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	// ExecutionManifestV1APIVersion identifies the first selection result contract.
	ExecutionManifestV1APIVersion = "argus.dev/execution-manifest/v1"
	executionManifestV1SchemaID   = "https://argus.dev/contracts/execution-manifest/v1/schema.json"
)

//go:embed generated/execution-manifest/v1/execution-manifest.schema.json
var executionManifestV1SchemaJSON []byte

var loadExecutionManifestV1Schema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(executionManifestV1SchemaJSON, executionManifestV1SchemaID, "execution manifest")
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

// ValidateExecutionManifestV1 verifies a typed document against Effect-authored JSON Schema.
func ValidateExecutionManifestV1(document ExecutionManifestV1) error {
	data, err := json.Marshal(document)
	if err != nil {
		return fmt.Errorf("encode execution manifest for validation: %w", err)
	}
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
