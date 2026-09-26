package contracts

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	// FunctionalAPIExecutionBindingsV1APIVersion identifies reviewed group bindings v1.
	FunctionalAPIExecutionBindingsV1APIVersion = "argus.dev/functional-api-execution-bindings/v1"
	// FunctionalAPIExecutionPlanV1APIVersion identifies flattened CI jobs v1.
	FunctionalAPIExecutionPlanV1APIVersion = "argus.dev/functional-api-execution-plan/v1"
)

//go:embed generated/functional-api-execution-bindings/v1/functional-api-execution-bindings.schema.json
var functionalAPIExecutionBindingsV1SchemaJSON []byte

//go:embed generated/functional-api-execution-plan/v1/functional-api-execution-plan.schema.json
var functionalAPIExecutionPlanV1SchemaJSON []byte

var loadFunctionalAPIExecutionBindingsV1Schema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(
		functionalAPIExecutionBindingsV1SchemaJSON,
		"https://argus.dev/contracts/functional-api-execution-bindings/v1/schema.json",
		"functional API execution bindings",
	)
})

var loadFunctionalAPIExecutionPlanV1Schema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(
		functionalAPIExecutionPlanV1SchemaJSON,
		"https://argus.dev/contracts/functional-api-execution-plan/v1/schema.json",
		"functional API execution plan",
	)
})

// FunctionalAPIExecutionGroupBinding supplies one reviewed immutable checkout.
type FunctionalAPIExecutionGroupBinding struct {
	GroupKey       string              `json:"groupKey"`
	TestRepository RepositoryReference `json:"testRepository"`
	TestRevision   RevisionReference   `json:"testRevision"`
	Adapter        string              `json:"adapter"`
}

// FunctionalAPIExecutionBindingsV1 binds exact manifest groups to revisions.
type FunctionalAPIExecutionBindingsV1 struct {
	APIVersion string                               `json:"apiVersion"`
	Groups     []FunctionalAPIExecutionGroupBinding `json:"groups"`
}

// FunctionalAPIExecutionJob is one runnable CI matrix entry.
type FunctionalAPIExecutionJob struct {
	GroupKey       string              `json:"groupKey"`
	Stage          string              `json:"stage"`
	TestRepository RepositoryReference `json:"testRepository"`
	TestRevision   RevisionReference   `json:"testRevision"`
	Adapter        string              `json:"adapter"`
	TestCount      int                 `json:"testCount"`
}

// FunctionalAPIExecutionPlanV1 is a deterministic flattened CI matrix.
type FunctionalAPIExecutionPlanV1 struct {
	APIVersion string                      `json:"apiVersion"`
	Manifest   ManifestDigestReference     `json:"manifest"`
	Jobs       []FunctionalAPIExecutionJob `json:"jobs"`
}

// DecodeFunctionalAPIExecutionBindingsV1 validates and decodes reviewed bindings.
func DecodeFunctionalAPIExecutionBindingsV1(data []byte) (FunctionalAPIExecutionBindingsV1, error) {
	var document FunctionalAPIExecutionBindingsV1
	if err := validateExecutionJSON(
		data, loadFunctionalAPIExecutionBindingsV1Schema, "functional API execution bindings",
	); err != nil {
		return document, err
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return document, fmt.Errorf("decode functional API execution bindings: %w", err)
	}

	return document, nil
}

// DecodeFunctionalAPIExecutionPlanV1 validates and decodes a CI execution plan.
func DecodeFunctionalAPIExecutionPlanV1(data []byte) (FunctionalAPIExecutionPlanV1, error) {
	var document FunctionalAPIExecutionPlanV1
	if err := validateExecutionJSON(
		data, loadFunctionalAPIExecutionPlanV1Schema, "functional API execution plan",
	); err != nil {
		return document, err
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return document, fmt.Errorf("decode functional API execution plan: %w", err)
	}

	return document, nil
}

// ValidateFunctionalAPIExecutionBindingsV1 validates typed reviewed bindings.
func ValidateFunctionalAPIExecutionBindingsV1(document FunctionalAPIExecutionBindingsV1) error {
	return validateTypedExecutionDocument(
		document, loadFunctionalAPIExecutionBindingsV1Schema, "functional API execution bindings",
	)
}

// ValidateFunctionalAPIExecutionPlanV1 validates a typed CI execution plan.
func ValidateFunctionalAPIExecutionPlanV1(document FunctionalAPIExecutionPlanV1) error {
	return validateTypedExecutionDocument(
		document, loadFunctionalAPIExecutionPlanV1Schema, "functional API execution plan",
	)
}
