package contracts

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var fieldRequestSchemaJSON = mustReadSchema("functional-api-field-adaptation-request/v1/functional-api-field-adaptation-request.schema.json")
var fieldResultSchemaJSON = mustReadSchema("functional-api-field-adaptation-result/v1/functional-api-field-adaptation-result.schema.json")
var fieldProposalSchemaJSON = mustReadSchema("field-adaptation-proposal/v1/field-adaptation-proposal.schema.json")

var loadFieldRequestSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(fieldRequestSchemaJSON,
		"https://argus.dev/contracts/functional-api-field-adaptation-request/v1/schema.json", "field adaptation request")
})
var loadFieldResultSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(fieldResultSchemaJSON,
		"https://argus.dev/contracts/functional-api-field-adaptation-result/v1/schema.json", "field adaptation result")
})
var loadFieldProposalSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(fieldProposalSchemaJSON,
		"https://argus.dev/contracts/field-adaptation-proposal/v1/schema.json", "field adaptation proposal")
})

// FieldRenameEvidenceV1 binds an explicitly declared property rename to exact
// immutable OpenAPI bytes and one operation.
type FieldRenameEvidenceV1 struct {
	DocumentPath string   `json:"documentPath"`
	BaseSHA256   string   `json:"baseSha256"`
	HeadSHA256   string   `json:"headSha256"`
	Method       string   `json:"method"`
	Path         string   `json:"path"`
	OperationID  string   `json:"operationId"`
	Capabilities []string `json:"capabilities"`
	Direction    string   `json:"direction"`
	StatusCode   string   `json:"statusCode"`
	PreviousName string   `json:"previousName"`
	Name         string   `json:"name"`
}

// FieldAdaptationRequestV1 is sent to explicitly configured source adapters.
type FieldAdaptationRequestV1 struct {
	APIVersion  string                    `json:"apiVersion"`
	ProposalID  string                    `json:"proposalId"`
	Change      AdaptationChangeReference `json:"change"`
	Test        AdaptationTestReference   `json:"test"`
	FieldRename FieldRenameEvidenceV1     `json:"fieldRename"`
}

// FieldAdaptationResultV1 is untrusted adapter output.
type FieldAdaptationResultV1 struct {
	APIVersion string              `json:"apiVersion"`
	ProposalID string              `json:"proposalId"`
	Adapter    AdapterIdentity     `json:"adapter"`
	Outcome    string              `json:"outcome"`
	ReasonCode *string             `json:"reasonCode"`
	Reason     *string             `json:"reason"`
	Edit       *AdaptationTextEdit `json:"edit"`
}

// FieldAdaptationProposalV1 is proposal-only evidence for class-specific
// validation in the next workflow stage.
type FieldAdaptationProposalV1 struct {
	APIVersion     string                    `json:"apiVersion"`
	PolicyVersion  string                    `json:"policyVersion"`
	ProposalID     string                    `json:"proposalId"`
	SourceImpact   AdaptationSourceImpact    `json:"sourceImpact"`
	Change         AdaptationChangeReference `json:"change"`
	Test           AdaptationTestReference   `json:"test"`
	Classification string                    `json:"classification"`
	Decision       string                    `json:"decision"`
	FieldRename    FieldRenameEvidenceV1     `json:"fieldRename"`
	Adapter        AdapterIdentity           `json:"adapter"`
	Edit           AdaptationTextEdit        `json:"edit"`
}

// ValidateFieldAdaptationRequestV1 verifies typed request structure.
func ValidateFieldAdaptationRequestV1(document FieldAdaptationRequestV1) error {
	return validateTypedAdaptationDocument(document, loadFieldRequestSchema, "field adaptation request")
}

// DecodeFieldAdaptationResultV1 rejects unknown and malformed adapter output.
func DecodeFieldAdaptationResultV1(data []byte) (FieldAdaptationResultV1, error) {
	var document FieldAdaptationResultV1
	if err := validateAdaptationJSON(data, loadFieldResultSchema, "field adaptation result"); err != nil {
		return document, err
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return document, fmt.Errorf("decode field adaptation result: %w", err)
	}

	return document, nil
}

// ValidateFieldAdaptationProposalV1 verifies typed public proposal structure.
func ValidateFieldAdaptationProposalV1(document FieldAdaptationProposalV1) error {
	return validateTypedAdaptationDocument(document, loadFieldProposalSchema, "field adaptation proposal")
}

// DecodeFieldAdaptationProposalV1 validates and decodes a public proposal.
func DecodeFieldAdaptationProposalV1(data []byte) (FieldAdaptationProposalV1, error) {
	var document FieldAdaptationProposalV1
	if err := validateAdaptationJSON(data, loadFieldProposalSchema, "field adaptation proposal"); err != nil {
		return document, err
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return document, fmt.Errorf("decode field adaptation proposal: %w", err)
	}

	return document, nil
}
