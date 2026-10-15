package contracts

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	// ReviewedSelectionPolicyV1APIVersion identifies reviewed CI policy input.
	ReviewedSelectionPolicyV1APIVersion = "argus.dev/reviewed-selection-policy/v1"
	// HeadTestInventoryV1APIVersion identifies complete observed test inventory.
	HeadTestInventoryV1APIVersion = "argus.dev/head-test-inventory/v1"
	// ExecutionManifestV4APIVersion identifies reviewed API selection output.
	ExecutionManifestV4APIVersion = "argus.dev/execution-manifest/v4"
)

var reviewedPolicySchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(mustReadSchema("reviewed-selection-policy/v1/reviewed-selection-policy.schema.json"),
		"https://argus.dev/contracts/reviewed-selection-policy/v1/schema.json", "reviewed selection policy")
})

var headInventorySchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(mustReadSchema("head-test-inventory/v1/head-test-inventory.schema.json"),
		"https://argus.dev/contracts/head-test-inventory/v1/schema.json", "head test inventory")
})

var manifestV4Schema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(mustReadSchema("execution-manifest/v4/execution-manifest.schema.json"),
		"https://argus.dev/contracts/execution-manifest/v4/schema.json", "reviewed execution manifest")
})

// ReviewedSelectionPolicyV1 is a validated owner policy document.
type ReviewedSelectionPolicyV1 struct {
	APIVersion          string                      `json:"apiVersion"`
	SourceRepository    RepositoryIdentityReference `json:"sourceRepository"`
	MustRunCapabilities []string                    `json:"mustRunCapabilities"`
	MustRunTests        []ImpactTestIdentity        `json:"mustRunTests"`
	Pins                []ImpactTestIdentity        `json:"pins"`
	Exclusions          []ImpactTestIdentity        `json:"exclusions"`
}

// HeadInventoryTest is one discovered functional API test.
type HeadInventoryTest struct {
	TestRepository RepositoryReference `json:"testRepository"`
	Revision       RevisionReference   `json:"revision"`
	SuiteKey       string              `json:"suiteKey"`
	Adapter        string              `json:"adapter"`
	TestKey        string              `json:"testKey"`
	Name           string              `json:"name"`
	Capabilities   []string            `json:"capabilities"`
	New            bool                `json:"new"`
	Modified       bool                `json:"modified"`
}

// HeadTestInventoryV1 is complete discovery at immutable test revisions.
type HeadTestInventoryV1 struct {
	APIVersion       string                      `json:"apiVersion"`
	SourceRepository RepositoryIdentityReference `json:"sourceRepository"`
	Complete         bool                        `json:"complete"`
	Tests            []HeadInventoryTest         `json:"tests"`
}

// ReviewedSelectionReason carries both the deciding rule and evidence keys.
type ReviewedSelectionReason struct {
	Code         string   `json:"code"`
	Capabilities []string `json:"capabilities"`
	EvidenceKeys []string `json:"evidenceKeys"`
}

// ReviewedTestDecision is one v4 inclusion or omission.
type ReviewedTestDecision struct {
	TestRepository     RepositoryReference       `json:"testRepository"`
	Suite              TestCatalogSuiteReference `json:"suite"`
	Test               TestCatalogTestReference  `json:"test"`
	Capabilities       []string                  `json:"capabilities"`
	Outcome            string                    `json:"outcome"`
	RemainingExecution string                    `json:"remainingExecution"`
	Reasons            []ReviewedSelectionReason `json:"reasons"`
}

// ExecutionManifestV4 binds selection to reviewed policy and head inventory.
type ExecutionManifestV4 struct {
	APIVersion            string                   `json:"apiVersion"`
	PolicyVersion         string                   `json:"policyVersion"`
	PolicyDigest          string                   `json:"policyDigest"`
	InventoryDigest       string                   `json:"inventoryDigest"`
	Impact                ManifestImpactReference  `json:"impact"`
	Change                ManifestChangeReference  `json:"change"`
	Catalog               CatalogSnapshotReference `json:"catalog"`
	Family                string                   `json:"family"`
	Mode                  string                   `json:"mode"`
	AffectedCapabilities  []string                 `json:"affectedCapabilities"`
	UncoveredCapabilities []string                 `json:"uncoveredCapabilities"`
	Decisions             []ReviewedTestDecision   `json:"decisions"`
	Warnings              []string                 `json:"warnings"`
}

// DecodeReviewedSelectionPolicyV1 validates and decodes untrusted policy JSON.
func DecodeReviewedSelectionPolicyV1(data []byte) (ReviewedSelectionPolicyV1, error) {
	var document ReviewedSelectionPolicyV1
	if err := validateDocument(data, reviewedPolicySchema, "reviewed selection policy"); err != nil {
		return document, err
	}
	err := json.Unmarshal(data, &document)

	return document, err
}

// DecodeHeadTestInventoryV1 validates and decodes observed inventory JSON.
func DecodeHeadTestInventoryV1(data []byte) (HeadTestInventoryV1, error) {
	var document HeadTestInventoryV1
	if err := validateDocument(data, headInventorySchema, "head test inventory"); err != nil {
		return document, err
	}
	err := json.Unmarshal(data, &document)

	return document, err
}

// ValidateExecutionManifestV4 checks the published v4 selection schema.
func ValidateExecutionManifestV4(document ExecutionManifestV4) error {
	data, err := json.Marshal(document)
	if err != nil {
		return fmt.Errorf("encode reviewed execution manifest: %w", err)
	}

	return validateDocument(data, manifestV4Schema, "reviewed execution manifest")
}

// DecodeExecutionManifestV4 validates and decodes a reviewed manifest.
func DecodeExecutionManifestV4(data []byte) (ExecutionManifestV4, error) {
	var document ExecutionManifestV4
	if err := validateDocument(data, manifestV4Schema, "reviewed execution manifest"); err != nil {
		return document, err
	}
	err := json.Unmarshal(data, &document)

	return document, err
}

func validateDocument(data []byte, load func() (*jsonschema.Schema, error), label string) error {
	schema, err := load()
	if err != nil {
		return err
	}
	value, err := decodeSingleJSONValue(data, label)
	if err != nil {
		return err
	}
	if err := schema.Validate(value); err != nil {
		return fmt.Errorf("validate %s: %w", label, err)
	}

	return nil
}
