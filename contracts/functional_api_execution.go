package contracts

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	// FunctionalAPIAdapterRequestV1APIVersion identifies adapter request v1.
	FunctionalAPIAdapterRequestV1APIVersion = "argus.dev/functional-api-adapter-request/v1"
	// FunctionalAPIAdapterResultV1APIVersion identifies adapter result v1.
	FunctionalAPIAdapterResultV1APIVersion = "argus.dev/functional-api-adapter-result/v1"
	// ExecutionAttemptV1APIVersion identifies normalized execution attempt v1.
	ExecutionAttemptV1APIVersion = "argus.dev/execution-attempt/v1"
)

//go:embed generated/functional-api-adapter-request/v1/functional-api-adapter-request.schema.json
var functionalAPIAdapterRequestV1SchemaJSON []byte

//go:embed generated/functional-api-adapter-result/v1/functional-api-adapter-result.schema.json
var functionalAPIAdapterResultV1SchemaJSON []byte

//go:embed generated/execution-attempt/v1/execution-attempt.schema.json
var executionAttemptV1SchemaJSON []byte

var loadFunctionalAPIAdapterRequestV1Schema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(
		functionalAPIAdapterRequestV1SchemaJSON,
		"https://argus.dev/contracts/functional-api-adapter-request/v1/schema.json",
		"functional API adapter request",
	)
})

var loadFunctionalAPIAdapterResultV1Schema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(
		functionalAPIAdapterResultV1SchemaJSON,
		"https://argus.dev/contracts/functional-api-adapter-result/v1/schema.json",
		"functional API adapter result",
	)
})

var loadExecutionAttemptV1Schema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(
		executionAttemptV1SchemaJSON,
		"https://argus.dev/contracts/execution-attempt/v1/schema.json",
		"execution attempt",
	)
})

// ManifestDigestReference identifies canonical execution-manifest bytes.
type ManifestDigestReference struct {
	APIVersion string `json:"apiVersion"`
	SHA256     string `json:"sha256"`
}

// RequestedFunctionalAPITest identifies one exact adapter request member.
type RequestedFunctionalAPITest struct {
	SuiteKey string `json:"suiteKey"`
	TestKey  string `json:"testKey"`
	Name     string `json:"name"`
}

// FunctionalAPIAdapterRequestV1 is the CI-local adapter input contract.
type FunctionalAPIAdapterRequestV1 struct {
	APIVersion     string                       `json:"apiVersion"`
	AttemptID      string                       `json:"attemptId"`
	Manifest       ManifestDigestReference      `json:"manifest"`
	Stage          string                       `json:"stage"`
	TestRepository RepositoryReference          `json:"testRepository"`
	TestRevision   RevisionReference            `json:"testRevision"`
	Adapter        string                       `json:"adapter"`
	Tests          []RequestedFunctionalAPITest `json:"tests"`
}

// NormalizedFailure is bounded adapter diagnostic metadata.
type NormalizedFailure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// NormalizedFunctionalAPITestResult is one framework-neutral test result.
type NormalizedFunctionalAPITestResult struct {
	SuiteKey   string             `json:"suiteKey"`
	TestKey    string             `json:"testKey"`
	Outcome    string             `json:"outcome"`
	DurationMS int64              `json:"durationMs"`
	Failure    *NormalizedFailure `json:"failure"`
}

// ExecutionArtifactReference identifies immutable attempt evidence.
type ExecutionArtifactReference struct {
	Key    string `json:"key"`
	Kind   string `json:"kind"`
	URI    string `json:"uri"`
	SHA256 string `json:"sha256"`
}

// AdapterIdentity identifies the implementation that produced results.
type AdapterIdentity struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

// FunctionalAPIAdapterResultV1 is untrusted output from an adapter process.
type FunctionalAPIAdapterResultV1 struct {
	APIVersion  string                              `json:"apiVersion"`
	AttemptID   string                              `json:"attemptId"`
	Adapter     AdapterIdentity                     `json:"adapter"`
	StartedAt   string                              `json:"startedAt"`
	CompletedAt string                              `json:"completedAt"`
	Results     []NormalizedFunctionalAPITestResult `json:"results"`
	Artifacts   []ExecutionArtifactReference        `json:"artifacts"`
}

// ExecutionAttemptV1 is normalized and exactly correlated execution evidence.
type ExecutionAttemptV1 struct {
	APIVersion     string                              `json:"apiVersion"`
	AttemptID      string                              `json:"attemptId"`
	Manifest       ManifestDigestReference             `json:"manifest"`
	Stage          string                              `json:"stage"`
	TestRepository RepositoryReference                 `json:"testRepository"`
	TestRevision   RevisionReference                   `json:"testRevision"`
	Adapter        AdapterIdentity                     `json:"adapter"`
	StartedAt      string                              `json:"startedAt"`
	CompletedAt    string                              `json:"completedAt"`
	Outcome        string                              `json:"outcome"`
	Results        []NormalizedFunctionalAPITestResult `json:"results"`
	Artifacts      []ExecutionArtifactReference        `json:"artifacts"`
}

// ValidateFunctionalAPIAdapterRequestV1 validates a typed adapter request.
func ValidateFunctionalAPIAdapterRequestV1(document FunctionalAPIAdapterRequestV1) error {
	return validateTypedExecutionDocument(
		document, loadFunctionalAPIAdapterRequestV1Schema, "functional API adapter request",
	)
}

// DecodeFunctionalAPIAdapterResultV1 validates and decodes external adapter output.
func DecodeFunctionalAPIAdapterResultV1(data []byte) (FunctionalAPIAdapterResultV1, error) {
	var document FunctionalAPIAdapterResultV1
	if err := validateExecutionJSON(
		data, loadFunctionalAPIAdapterResultV1Schema, "functional API adapter result",
	); err != nil {
		return document, err
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return document, fmt.Errorf("decode functional API adapter result: %w", err)
	}

	return document, nil
}

// ValidateFunctionalAPIAdapterResultV1 validates a typed adapter result.
func ValidateFunctionalAPIAdapterResultV1(document FunctionalAPIAdapterResultV1) error {
	return validateTypedExecutionDocument(
		document, loadFunctionalAPIAdapterResultV1Schema, "functional API adapter result",
	)
}

// ValidateExecutionAttemptV1 validates normalized attempt evidence.
func ValidateExecutionAttemptV1(document ExecutionAttemptV1) error {
	return validateTypedExecutionDocument(document, loadExecutionAttemptV1Schema, "execution attempt")
}

// DecodeExecutionAttemptV1 validates and decodes externally supplied attempt evidence.
func DecodeExecutionAttemptV1(data []byte) (ExecutionAttemptV1, error) {
	var document ExecutionAttemptV1
	if err := validateExecutionJSON(data, loadExecutionAttemptV1Schema, "execution attempt"); err != nil {
		return document, err
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return document, fmt.Errorf("decode execution attempt: %w", err)
	}

	return document, nil
}

func validateTypedExecutionDocument[T any](
	document T,
	loader func() (*jsonschema.Schema, error),
	name string,
) error {
	data, err := json.Marshal(document)
	if err != nil {
		return fmt.Errorf("encode %s for validation: %w", name, err)
	}

	return validateExecutionJSON(data, loader, name)
}

func validateExecutionJSON(
	data []byte,
	loader func() (*jsonschema.Schema, error),
	name string,
) error {
	schema, err := loader()
	if err != nil {
		return err
	}
	untyped, err := decodeSingleJSONValue(data, name)
	if err != nil {
		return err
	}
	if err := schema.Validate(untyped); err != nil {
		return fmt.Errorf("validate %s schema: %w", name, err)
	}

	return nil
}
