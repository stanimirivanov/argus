package contracts

import (
	"encoding/json"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// BrowserSuiteBindingsV1APIVersion identifies reviewed browser checkout pins.
const BrowserSuiteBindingsV1APIVersion = "argus.dev/browser-suite-bindings/v1"

var browserSuiteBindingsSchemaJSON = mustReadSchema("browser-suite-bindings/v1/browser-suite-bindings.schema.json")

var loadBrowserSuiteBindingsSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(browserSuiteBindingsSchemaJSON,
		"https://argus.dev/contracts/browser-suite-bindings/v1/schema.json", "browser suite bindings")
})

// BrowserSuiteBinding pins a browser suite's intended checkout revision.
type BrowserSuiteBinding struct {
	TestRepository RepositoryIdentityReference `json:"testRepository"`
	SuiteKey       string                      `json:"suiteKey"`
	Revision       RevisionReference           `json:"revision"`
}

// BrowserSuiteBindingsV1 is reviewed CI input, independent of collector output.
type BrowserSuiteBindingsV1 struct {
	APIVersion       string                      `json:"apiVersion"`
	SourceRepository RepositoryIdentityReference `json:"sourceRepository"`
	Suites           []BrowserSuiteBinding       `json:"suites"`
}

// DecodeBrowserSuiteBindingsV1 checks the portable schema before use.
func DecodeBrowserSuiteBindingsV1(data []byte) (BrowserSuiteBindingsV1, error) {
	var document BrowserSuiteBindingsV1
	if err := validateDocument(data, loadBrowserSuiteBindingsSchema, "browser suite bindings"); err != nil {
		return document, err
	}
	err := json.Unmarshal(data, &document)

	return document, err
}
