package contracts

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	// BrowserDiscoveryV1APIVersion identifies immutable Playwright catalog evidence.
	BrowserDiscoveryV1APIVersion = "argus.dev/browser-discovery/v1"
	browserDiscoverySchemaID     = "https://argus.dev/contracts/browser-discovery/v1/schema.json"
)

var browserDiscoverySchemaJSON = mustReadSchema("browser-discovery/v1/browser-discovery.schema.json")

var loadBrowserDiscoverySchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(browserDiscoverySchemaJSON, browserDiscoverySchemaID, "browser discovery")
})

// BrowserDiscoveryV1 is the portable wire representation of collected browser
// variants and reviewed UI source mappings. Semantic validation against the
// descriptor occurs at the catalog ingress.
type BrowserDiscoveryV1 struct {
	APIVersion       string                    `json:"apiVersion"`
	ObservationID    string                    `json:"observationId"`
	Catalog          CatalogSnapshotReference  `json:"catalog"`
	CollectorVersion string                    `json:"collectorVersion"`
	CollectedAt      string                    `json:"collectedAt"`
	ExpiresAt        string                    `json:"expiresAt"`
	Complete         bool                      `json:"complete"`
	MappingRevision  RevisionReference         `json:"mappingRevision"`
	MappingComplete  bool                      `json:"mappingComplete"`
	Suites           []BrowserSuiteObservation `json:"suites"`
	Mappings         []BrowserMapping          `json:"mappings"`
}

// BrowserSuiteObservation retains the test checkout and project variants.
type BrowserSuiteObservation struct {
	TestRepository RepositoryReference `json:"testRepository"`
	Revision       RevisionReference   `json:"revision"`
	SuiteKey       string              `json:"suiteKey"`
	Cases          []BrowserCase       `json:"cases"`
}

// BrowserCase describes one stable test in one Playwright project.
type BrowserCase struct {
	TestKey string   `json:"testKey"`
	Project string   `json:"project"`
	File    string   `json:"file"`
	Title   string   `json:"title"`
	Tags    []string `json:"tags"`
	Owners  []string `json:"owners"`
}

// BrowserMapping records one reviewed route/component relationship.
type BrowserMapping struct {
	Kind           string                      `json:"kind"`
	Key            string                      `json:"key"`
	RoutePattern   string                      `json:"routePattern"`
	SourceRoot     string                      `json:"sourceRoot"`
	TestRepository RepositoryIdentityReference `json:"testRepository"`
	SuiteKey       string                      `json:"suiteKey"`
	TestKey        string                      `json:"testKey"`
}

// DecodeBrowserDiscoveryV1 checks the generated schema before decoding.
func DecodeBrowserDiscoveryV1(data []byte) (BrowserDiscoveryV1, error) {
	var document BrowserDiscoveryV1
	if err := validateDocument(data, loadBrowserDiscoverySchema, "browser discovery"); err != nil {
		return document, fmt.Errorf("browser discovery contract: %w", err)
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return BrowserDiscoveryV1{}, fmt.Errorf("decode browser discovery: %w", err)
	}

	return document, nil
}

// ValidateBrowserDiscoveryV1 checks an outbound wire document.
func ValidateBrowserDiscoveryV1(document BrowserDiscoveryV1) error {
	data, err := json.Marshal(document)
	if err != nil {
		return fmt.Errorf("encode browser discovery: %w", err)
	}

	return validateDocument(data, loadBrowserDiscoverySchema, "browser discovery")
}
