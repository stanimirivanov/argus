package contracts

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	// FunctionalAPIAdaptationRequestV1APIVersion identifies adaptation input v1.
	FunctionalAPIAdaptationRequestV1APIVersion = "argus.dev/functional-api-adaptation-request/v1"
	// FunctionalAPIAdaptationResultV1APIVersion identifies adapter output v1.
	FunctionalAPIAdaptationResultV1APIVersion = "argus.dev/functional-api-adaptation-result/v1"
	// AdaptationProposalV1APIVersion identifies reviewable proposals v1.
	AdaptationProposalV1APIVersion = "argus.dev/adaptation-proposal/v1"
)

var functionalAPIAdaptationRequestV1SchemaJSON = mustReadSchema("functional-api-adaptation-request/v1/functional-api-adaptation-request.schema.json")

var functionalAPIAdaptationResultV1SchemaJSON = mustReadSchema("functional-api-adaptation-result/v1/functional-api-adaptation-result.schema.json")

var adaptationProposalV1SchemaJSON = mustReadSchema("adaptation-proposal/v1/adaptation-proposal.schema.json")

var loadFunctionalAPIAdaptationRequestV1Schema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(
		functionalAPIAdaptationRequestV1SchemaJSON,
		"https://argus.dev/contracts/functional-api-adaptation-request/v1/schema.json",
		"functional API adaptation request",
	)
})

var loadFunctionalAPIAdaptationResultV1Schema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(
		functionalAPIAdaptationResultV1SchemaJSON,
		"https://argus.dev/contracts/functional-api-adaptation-result/v1/schema.json",
		"functional API adaptation result",
	)
})

var loadAdaptationProposalV1Schema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(
		adaptationProposalV1SchemaJSON,
		"https://argus.dev/contracts/adaptation-proposal/v1/schema.json",
		"adaptation proposal",
	)
})

// AdaptationChangeReference binds a proposal to immutable source evidence.
type AdaptationChangeReference struct {
	SourceRepository RepositoryReference  `json:"sourceRepository"`
	PullRequest      PullRequestReference `json:"pullRequest"`
	BaseRevision     RevisionReference    `json:"baseRevision"`
	HeadRevision     RevisionReference    `json:"headRevision"`
	ObservedAt       string               `json:"observedAt"`
	Trigger          ChangeTrigger        `json:"trigger"`
}

// AdaptationTestReference identifies one immutable test source.
type AdaptationTestReference struct {
	Repository   RepositoryReference `json:"repository"`
	Revision     RevisionReference   `json:"revision"`
	SuiteKey     string              `json:"suiteKey"`
	TestKey      string              `json:"testKey"`
	Name         string              `json:"name"`
	Adapter      string              `json:"adapter"`
	Capabilities []string            `json:"capabilities"`
}

// EndpointRenameEvidence retains the exact authoritative mapping.
type EndpointRenameEvidence struct {
	Method       string   `json:"method"`
	OperationID  string   `json:"operationId"`
	PreviousPath string   `json:"previousPath"`
	Path         string   `json:"path"`
	Capabilities []string `json:"capabilities"`
}

// AdaptationTextEdit is one byte-addressed source replacement.
type AdaptationTextEdit struct {
	Path         string `json:"path"`
	BeforeSHA256 string `json:"beforeSha256"`
	StartByte    int    `json:"startByte"`
	EndByte      int    `json:"endByte"`
	Original     string `json:"original"`
	Replacement  string `json:"replacement"`
	SemanticRole string `json:"semanticRole"`
}

// FunctionalAPIAdaptationRequestV1 is sent to reviewed framework code.
type FunctionalAPIAdaptationRequestV1 struct {
	APIVersion     string                    `json:"apiVersion"`
	ProposalID     string                    `json:"proposalId"`
	Change         AdaptationChangeReference `json:"change"`
	Test           AdaptationTestReference   `json:"test"`
	EndpointRename EndpointRenameEvidence    `json:"endpointRename"`
}

// FunctionalAPIAdaptationResultV1 is untrusted adapter output.
type FunctionalAPIAdaptationResultV1 struct {
	APIVersion string              `json:"apiVersion"`
	ProposalID string              `json:"proposalId"`
	Adapter    AdapterIdentity     `json:"adapter"`
	Outcome    string              `json:"outcome"`
	ReasonCode *string             `json:"reasonCode"`
	Reason     *string             `json:"reason"`
	Edit       *AdaptationTextEdit `json:"edit"`
}

// AdaptationSourceImpact identifies the semantic analyzer contract.
type AdaptationSourceImpact struct {
	APIVersion      string `json:"apiVersion"`
	AnalyzerVersion string `json:"analyzerVersion"`
}

// AdaptationProposalV1 is a constrained candidate for later validation.
type AdaptationProposalV1 struct {
	APIVersion     string                    `json:"apiVersion"`
	PolicyVersion  string                    `json:"policyVersion"`
	ProposalID     string                    `json:"proposalId"`
	SourceImpact   AdaptationSourceImpact    `json:"sourceImpact"`
	Change         AdaptationChangeReference `json:"change"`
	Test           AdaptationTestReference   `json:"test"`
	Classification string                    `json:"classification"`
	Decision       string                    `json:"decision"`
	EndpointRename EndpointRenameEvidence    `json:"endpointRename"`
	Adapter        AdapterIdentity           `json:"adapter"`
	Edit           AdaptationTextEdit        `json:"edit"`
}

// ValidateFunctionalAPIAdaptationRequestV1 validates a typed request.
func ValidateFunctionalAPIAdaptationRequestV1(document FunctionalAPIAdaptationRequestV1) error {
	return validateTypedAdaptationDocument(
		document, loadFunctionalAPIAdaptationRequestV1Schema, "functional API adaptation request",
	)
}

// DecodeFunctionalAPIAdaptationResultV1 validates and decodes adapter output.
func DecodeFunctionalAPIAdaptationResultV1(data []byte) (FunctionalAPIAdaptationResultV1, error) {
	var document FunctionalAPIAdaptationResultV1
	if err := validateAdaptationJSON(data, loadFunctionalAPIAdaptationResultV1Schema, "functional API adaptation result"); err != nil {
		return document, err
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return document, fmt.Errorf("decode functional API adaptation result: %w", err)
	}

	return document, nil
}

// DecodeAdaptationProposalV1 validates and decodes a public proposal.
func DecodeAdaptationProposalV1(data []byte) (AdaptationProposalV1, error) {
	var document AdaptationProposalV1
	if err := validateAdaptationJSON(data, loadAdaptationProposalV1Schema, "adaptation proposal"); err != nil {
		return document, err
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return document, fmt.Errorf("decode adaptation proposal: %w", err)
	}

	return document, nil
}

// ValidateAdaptationProposalV1 validates a typed proposal.
func ValidateAdaptationProposalV1(document AdaptationProposalV1) error {
	return validateTypedAdaptationDocument(document, loadAdaptationProposalV1Schema, "adaptation proposal")
}

func validateTypedAdaptationDocument(
	document any,
	load func() (*jsonschema.Schema, error),
	name string,
) error {
	data, err := json.Marshal(document)
	if err != nil {
		return fmt.Errorf("encode %s for validation: %w", name, err)
	}

	return validateAdaptationJSON(data, load, name)
}

func validateAdaptationJSON(data []byte, load func() (*jsonschema.Schema, error), name string) error {
	schema, err := load()
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
