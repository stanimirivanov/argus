package contracts

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	// TestCatalogPageV1APIVersion identifies the first paginated test catalog
	// result contract.
	TestCatalogPageV1APIVersion = "argus.dev/test-catalog-page/v1"
	testCatalogPageV1SchemaID   = "https://argus.dev/contracts/test-catalog-page/v1/schema.json"
)

//go:embed generated/test-catalog-page/v1/test-catalog-page.schema.json
var testCatalogPageV1SchemaJSON []byte

var loadTestCatalogPageV1Schema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(
		testCatalogPageV1SchemaJSON,
		testCatalogPageV1SchemaID,
		"test catalog page",
	)
})

// TestCatalogPageV1 is the versioned transport representation of a
// deterministic page from one immutable catalog snapshot.
type TestCatalogPageV1 struct {
	APIVersion string                   `json:"apiVersion"`
	Snapshot   CatalogSnapshotReference `json:"snapshot"`
	Items      []TestCatalogEntry       `json:"items"`
	NextCursor *string                  `json:"nextCursor"`
}

// CatalogSnapshotReference identifies the immutable descriptor observation
// from which a test page was read.
type CatalogSnapshotReference struct {
	SourceRepository     RepositoryReference `json:"sourceRepository"`
	Revision             RevisionReference   `json:"revision"`
	DescriptorAPIVersion string              `json:"descriptorApiVersion"`
}

// RevisionReference is the transport representation of an immutable Git
// revision.
type RevisionReference struct {
	Algorithm string `json:"algorithm"`
	Digest    string `json:"digest"`
}

// TestCatalogEntry describes one stable test and its catalog mappings.
// Identity is the test repository identity, suite key, and test key.
type TestCatalogEntry struct {
	TestRepository RepositoryReference       `json:"testRepository"`
	Suite          TestCatalogSuiteReference `json:"suite"`
	Test           TestCatalogTestReference  `json:"test"`
	Capabilities   []CapabilityReference     `json:"capabilities"`
}

// TestCatalogSuiteReference contains suite metadata associated with a test.
type TestCatalogSuiteReference struct {
	Key     string `json:"key"`
	Family  string `json:"family"`
	Adapter string `json:"adapter"`
}

// TestCatalogTestReference contains the stable suite-scoped key and display
// name of a test.
type TestCatalogTestReference struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// CapabilityReference identifies a source-repository capability mapped to a
// catalog test.
type CapabilityReference struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// DecodeTestCatalogPageV1 validates and decodes an external v1 catalog page.
func DecodeTestCatalogPageV1(data []byte) (TestCatalogPageV1, error) {
	var document TestCatalogPageV1

	if err := validateTestCatalogPageV1JSON(data); err != nil {
		return document, err
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return document, fmt.Errorf("decode test catalog page: %w", err)
	}

	return document, nil
}

// ValidateTestCatalogPageV1 verifies that a typed result still conforms to the
// Effect-authored contract before it crosses a transport boundary.
func ValidateTestCatalogPageV1(document TestCatalogPageV1) error {
	data, err := json.Marshal(document)
	if err != nil {
		return fmt.Errorf("encode test catalog page for validation: %w", err)
	}

	return validateTestCatalogPageV1JSON(data)
}

func validateTestCatalogPageV1JSON(data []byte) error {
	schema, err := loadTestCatalogPageV1Schema()
	if err != nil {
		return err
	}
	untyped, err := decodeSingleJSONValue(data, "test catalog page")
	if err != nil {
		return err
	}
	if err := schema.Validate(untyped); err != nil {
		return fmt.Errorf("validate test catalog page schema: %w", err)
	}

	return nil
}
