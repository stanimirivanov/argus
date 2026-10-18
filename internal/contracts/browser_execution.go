package contracts

import (
	"encoding/json"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var loadBrowserAdapterRequestSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(mustReadSchema("browser-adapter-request/v1/browser-adapter-request.schema.json"), "https://argus.dev/contracts/browser-adapter-request/v1/schema.json", "browser adapter request")
})
var loadBrowserAdapterResultSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(mustReadSchema("browser-adapter-result/v1/browser-adapter-result.schema.json"), "https://argus.dev/contracts/browser-adapter-result/v1/schema.json", "browser adapter result")
})
var loadBrowserAttemptSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(mustReadSchema("browser-execution-attempt/v1/browser-execution-attempt.schema.json"), "https://argus.dev/contracts/browser-execution-attempt/v1/schema.json", "browser attempt")
})

// BrowserArtifactReference binds one browser test to immutable artifact metadata.
type BrowserArtifactReference struct {
	Key      string `json:"key"`
	SuiteKey string `json:"suiteKey"`
	TestKey  string `json:"testKey"`
	Kind     string `json:"kind"`
	URI      string `json:"uri"`
	SHA256   string `json:"sha256"`
}

// BrowserAdapterRequestV1 is the exact browser adapter input.
type BrowserAdapterRequestV1 struct {
	APIVersion     string                       `json:"apiVersion"`
	AttemptID      string                       `json:"attemptId"`
	Manifest       ManifestDigestReference      `json:"manifest"`
	Stage          string                       `json:"stage"`
	TestRepository RepositoryReference          `json:"testRepository"`
	TestRevision   RevisionReference            `json:"testRevision"`
	Adapter        string                       `json:"adapter"`
	Project        string                       `json:"project"`
	Environment    string                       `json:"environment"`
	Tests          []RequestedFunctionalAPITest `json:"tests"`
}

// BrowserAdapterResultV1 is untrusted output from a CI-local browser adapter.
type BrowserAdapterResultV1 struct {
	APIVersion  string                              `json:"apiVersion"`
	AttemptID   string                              `json:"attemptId"`
	Adapter     AdapterIdentity                     `json:"adapter"`
	Project     string                              `json:"project"`
	Environment string                              `json:"environment"`
	StartedAt   string                              `json:"startedAt"`
	CompletedAt string                              `json:"completedAt"`
	Results     []NormalizedFunctionalAPITestResult `json:"results"`
	Artifacts   []BrowserArtifactReference          `json:"artifacts"`
}

// BrowserExecutionAttemptV1 is correlated browser evidence for ingestion.
type BrowserExecutionAttemptV1 struct {
	APIVersion     string                              `json:"apiVersion"`
	AttemptID      string                              `json:"attemptId"`
	Manifest       ManifestDigestReference             `json:"manifest"`
	Stage          string                              `json:"stage"`
	TestRepository RepositoryReference                 `json:"testRepository"`
	TestRevision   RevisionReference                   `json:"testRevision"`
	Adapter        AdapterIdentity                     `json:"adapter"`
	Tests          []RequestedFunctionalAPITest        `json:"tests"`
	Project        string                              `json:"project"`
	Environment    string                              `json:"environment"`
	StartedAt      string                              `json:"startedAt"`
	CompletedAt    string                              `json:"completedAt"`
	Outcome        string                              `json:"outcome"`
	Results        []NormalizedFunctionalAPITestResult `json:"results"`
	Artifacts      []BrowserArtifactReference          `json:"artifacts"`
}

// ValidateBrowserAdapterRequestV1 checks the portable request schema.
func ValidateBrowserAdapterRequestV1(document BrowserAdapterRequestV1) error {
	return validateTypedExecutionDocument(document, loadBrowserAdapterRequestSchema, "browser adapter request")
}

// DecodeBrowserAdapterResultV1 validates and decodes adapter output.
func DecodeBrowserAdapterResultV1(data []byte) (BrowserAdapterResultV1, error) {
	var document BrowserAdapterResultV1
	if err := validateExecutionJSON(data, loadBrowserAdapterResultSchema, "browser adapter result"); err != nil {
		return document, err
	}
	err := json.Unmarshal(data, &document)

	return document, err
}

// ValidateBrowserExecutionAttemptV1 checks the portable attempt schema.
func ValidateBrowserExecutionAttemptV1(document BrowserExecutionAttemptV1) error {
	return validateTypedExecutionDocument(document, loadBrowserAttemptSchema, "browser execution attempt")
}

// DecodeBrowserExecutionAttemptV1 validates and decodes uploaded evidence.
func DecodeBrowserExecutionAttemptV1(data []byte) (BrowserExecutionAttemptV1, error) {
	var document BrowserExecutionAttemptV1
	if err := validateExecutionJSON(data, loadBrowserAttemptSchema, "browser execution attempt"); err != nil {
		return document, err
	}
	err := json.Unmarshal(data, &document)

	return document, err
}
