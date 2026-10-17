package contracts

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var loadFieldValidationRequestSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(mustReadSchema("functional-api-field-validation-request/v1/functional-api-field-validation-request.schema.json"),
		"https://argus.dev/contracts/functional-api-field-validation-request/v1/schema.json", "field validation request")
})
var loadFieldValidationResultSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(mustReadSchema("functional-api-field-validation-result/v1/functional-api-field-validation-result.schema.json"),
		"https://argus.dev/contracts/functional-api-field-validation-result/v1/schema.json", "field validation result")
})
var loadFieldValidationEvidenceSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(mustReadSchema("field-validation-evidence/v1/field-validation-evidence.schema.json"),
		"https://argus.dev/contracts/field-validation-evidence/v1/schema.json", "field validation evidence")
})
var loadFieldValidationRejectionSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(mustReadSchema("field-validation-rejection/v1/field-validation-rejection.schema.json"),
		"https://argus.dev/contracts/field-validation-rejection/v1/schema.json", "field validation rejection")
})

// FieldValidationRequestV1 is one repeated adapter request.
type FieldValidationRequestV1 struct {
	APIVersion   string                  `json:"apiVersion"`
	ValidationID string                  `json:"validationId"`
	ProposalID   string                  `json:"proposalId"`
	Phase        string                  `json:"phase"`
	Repeat       int                     `json:"repeat"`
	Test         AdaptationTestReference `json:"test"`
	SourceSHA256 string                  `json:"sourceSha256"`
}

// FieldValidationResultV1 is one untrusted adapter observation.
type FieldValidationResultV1 struct {
	APIVersion   string             `json:"apiVersion"`
	ValidationID string             `json:"validationId"`
	ProposalID   string             `json:"proposalId"`
	Phase        string             `json:"phase"`
	Repeat       int                `json:"repeat"`
	SuiteKey     string             `json:"suiteKey"`
	TestKey      string             `json:"testKey"`
	Adapter      AdapterIdentity    `json:"adapter"`
	SourceSHA256 string             `json:"sourceSha256"`
	StartedAt    string             `json:"startedAt"`
	CompletedAt  string             `json:"completedAt"`
	Outcome      string             `json:"outcome"`
	Failure      *NormalizedFailure `json:"failure"`
}

// FieldValidationRunV1 is one normalized portable run.
type FieldValidationRunV1 struct {
	Phase        string             `json:"phase"`
	Repeat       int                `json:"repeat"`
	SourceSHA256 string             `json:"sourceSha256"`
	StartedAt    string             `json:"startedAt"`
	CompletedAt  string             `json:"completedAt"`
	Outcome      string             `json:"outcome"`
	Failure      *NormalizedFailure `json:"failure"`
}

// FieldValidationSourceV1 binds the materialized and restored file digests.
type FieldValidationSourceV1 struct {
	Path                 string `json:"path"`
	OriginalSHA256       string `json:"originalSha256"`
	CandidateSHA256      string `json:"candidateSha256"`
	NegativeSHA256       string `json:"negativeSha256"`
	RestoredSHA256       string `json:"restoredSha256"`
	NegativeControlField string `json:"negativeControlField"`
}

// FieldValidationEvidenceV1 is the six-run successful public proof.
type FieldValidationEvidenceV1 struct {
	APIVersion            string                  `json:"apiVersion"`
	PolicyVersion         string                  `json:"policyVersion"`
	ValidationID          string                  `json:"validationId"`
	ProposalID            string                  `json:"proposalId"`
	ProposalPolicyVersion string                  `json:"proposalPolicyVersion"`
	Test                  AdaptationTestReference `json:"test"`
	Adapter               AdapterIdentity         `json:"adapter"`
	Edit                  AdaptationTextEdit      `json:"edit"`
	Source                FieldValidationSourceV1 `json:"source"`
	Runs                  []FieldValidationRunV1  `json:"runs"`
}

// FieldValidationRejectionV1 is a trustworthy failed policy gate.
type FieldValidationRejectionV1 struct {
	FieldValidationEvidenceV1
	Rejection AdaptationValidationRejection `json:"rejection"`
}

// ValidateFieldValidationRequestV1 checks adapter request shape.
func ValidateFieldValidationRequestV1(document FieldValidationRequestV1) error {
	return validateTypedAdaptationDocument(document, loadFieldValidationRequestSchema, "field validation request")
}

// DecodeFieldValidationResultV1 rejects malformed or unknown adapter output.
func DecodeFieldValidationResultV1(data []byte) (FieldValidationResultV1, error) {
	var document FieldValidationResultV1
	if err := validateAdaptationJSON(data, loadFieldValidationResultSchema, "field validation result"); err != nil {
		return document, err
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return document, fmt.Errorf("decode field validation result: %w", err)
	}

	return document, nil
}

// ValidateFieldValidationEvidenceV1 checks public success shape.
func ValidateFieldValidationEvidenceV1(document FieldValidationEvidenceV1) error {
	return validateTypedAdaptationDocument(document, loadFieldValidationEvidenceSchema, "field validation evidence")
}

// DecodeFieldValidationEvidenceV1 checks and decodes public success evidence.
func DecodeFieldValidationEvidenceV1(data []byte) (FieldValidationEvidenceV1, error) {
	var document FieldValidationEvidenceV1
	if err := validateAdaptationJSON(data, loadFieldValidationEvidenceSchema, "field validation evidence"); err != nil {
		return document, err
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return document, err
	}

	return document, nil
}

// ValidateFieldValidationRejectionV1 checks public rejection shape.
func ValidateFieldValidationRejectionV1(document FieldValidationRejectionV1) error {
	return validateTypedAdaptationDocument(document, loadFieldValidationRejectionSchema, "field validation rejection")
}
