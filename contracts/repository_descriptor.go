// Package contracts validates external Argus contract documents and exposes
// transport representations for conversion at application boundaries.
package contracts

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	// RepositoryDescriptorV1APIVersion identifies the first repository
	// descriptor contract.
	RepositoryDescriptorV1APIVersion = "argus.dev/repository-descriptor/v1"
	repositoryDescriptorV1SchemaID   = "https://argus.dev/contracts/repository-descriptor/v1/schema.json"
)

//go:embed generated/repository-descriptor/v1/repository-descriptor.schema.json
var repositoryDescriptorV1SchemaJSON []byte

var loadRepositoryDescriptorV1Schema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(
		repositoryDescriptorV1SchemaJSON,
		repositoryDescriptorV1SchemaID,
		"repository descriptor",
	)
})

// RepositoryDescriptorV1 is the transport representation of a validated v1
// repository descriptor. Domain invariants are enforced when this value is
// imported into the catalog model.
type RepositoryDescriptorV1 struct {
	APIVersion   string                  `json:"apiVersion"`
	Repository   RepositoryReference     `json:"repository"`
	Capabilities []CapabilityDeclaration `json:"capabilities"`
	Components   []ComponentDeclaration  `json:"components"`
	TestSuites   []TestSuiteDeclaration  `json:"testSuites"`
}

// RepositoryReference identifies a repository by a provider-assigned opaque
// identifier while retaining its current human-readable coordinates.
type RepositoryReference struct {
	Provider             string `json:"provider"`
	Host                 string `json:"host"`
	ProviderRepositoryID string `json:"providerRepositoryId"`
	Owner                string `json:"owner"`
	Name                 string `json:"name"`
}

// CapabilityDeclaration describes a stable capability within the source
// repository.
type CapabilityDeclaration struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// ComponentDeclaration maps a source tree root to capabilities.
type ComponentDeclaration struct {
	Key          string   `json:"key"`
	Root         string   `json:"root"`
	Capabilities []string `json:"capabilities"`
}

// TestSuiteDeclaration describes one test suite and the repository containing
// its tests.
type TestSuiteDeclaration struct {
	Key        string              `json:"key"`
	Repository RepositoryReference `json:"repository"`
	Family     string              `json:"family"`
	Adapter    string              `json:"adapter"`
	Tests      []TestDeclaration   `json:"tests"`
}

// TestDeclaration maps a stable test key to the capabilities it exercises.
type TestDeclaration struct {
	Key          string   `json:"key"`
	Name         string   `json:"name"`
	Capabilities []string `json:"capabilities"`
}

// DecodeRepositoryDescriptorV1 validates an external document against the
// JSON Schema generated from Effect Schema before decoding its transport DTO.
func DecodeRepositoryDescriptorV1(data []byte) (RepositoryDescriptorV1, error) {
	var document RepositoryDescriptorV1

	schema, err := loadRepositoryDescriptorV1Schema()
	if err != nil {
		return document, err
	}

	untyped, err := decodeSingleJSONValue(data, "repository descriptor")
	if err != nil {
		return document, err
	}
	if err := schema.Validate(untyped); err != nil {
		return document, fmt.Errorf("validate repository descriptor schema: %w", err)
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return document, fmt.Errorf("decode repository descriptor: %w", err)
	}

	return document, nil
}
