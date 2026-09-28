package contracts

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	// FunctionalAPIRepairValidationRequestV1APIVersion identifies phase requests v1.
	FunctionalAPIRepairValidationRequestV1APIVersion = "argus.dev/functional-api-repair-validation-request/v1"
	// FunctionalAPIRepairValidationResultV1APIVersion identifies phase results v1.
	FunctionalAPIRepairValidationResultV1APIVersion = "argus.dev/functional-api-repair-validation-result/v1"
	// ValidationEvidenceV1APIVersion identifies successful validation evidence v1.
	ValidationEvidenceV1APIVersion = "argus.dev/validation-evidence/v1"
	// ValidationRejectionV1APIVersion identifies trustworthy rejected validation v1.
	ValidationRejectionV1APIVersion = "argus.dev/validation-rejection/v1"
)

//go:embed generated/functional-api-repair-validation-request/v1/functional-api-repair-validation-request.schema.json
var functionalAPIRepairValidationRequestV1SchemaJSON []byte

//go:embed generated/functional-api-repair-validation-result/v1/functional-api-repair-validation-result.schema.json
var functionalAPIRepairValidationResultV1SchemaJSON []byte

//go:embed generated/validation-evidence/v1/validation-evidence.schema.json
var validationEvidenceV1SchemaJSON []byte

//go:embed generated/validation-rejection/v1/validation-rejection.schema.json
var validationRejectionV1SchemaJSON []byte

var loadFunctionalAPIRepairValidationRequestV1Schema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(
		functionalAPIRepairValidationRequestV1SchemaJSON,
		"https://argus.dev/contracts/functional-api-repair-validation-request/v1/schema.json",
		"functional API repair validation request",
	)
})

var loadFunctionalAPIRepairValidationResultV1Schema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(
		functionalAPIRepairValidationResultV1SchemaJSON,
		"https://argus.dev/contracts/functional-api-repair-validation-result/v1/schema.json",
		"functional API repair validation result",
	)
})

var loadValidationEvidenceV1Schema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(
		validationEvidenceV1SchemaJSON,
		"https://argus.dev/contracts/validation-evidence/v1/schema.json",
		"validation evidence",
	)
})

var loadValidationRejectionV1Schema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(
		validationRejectionV1SchemaJSON,
		"https://argus.dev/contracts/validation-rejection/v1/schema.json",
		"validation rejection",
	)
})

// FunctionalAPIRepairValidationRequestV1 asks an adapter to execute one phase.
type FunctionalAPIRepairValidationRequestV1 struct {
	APIVersion   string                  `json:"apiVersion"`
	ValidationID string                  `json:"validationId"`
	ProposalID   string                  `json:"proposalId"`
	Phase        string                  `json:"phase"`
	Test         AdaptationTestReference `json:"test"`
	SourceSHA256 string                  `json:"sourceSha256"`
}

// FunctionalAPIRepairValidationResultV1 is untrusted single-phase output.
type FunctionalAPIRepairValidationResultV1 struct {
	APIVersion   string             `json:"apiVersion"`
	ValidationID string             `json:"validationId"`
	ProposalID   string             `json:"proposalId"`
	Phase        string             `json:"phase"`
	SuiteKey     string             `json:"suiteKey"`
	TestKey      string             `json:"testKey"`
	Adapter      AdapterIdentity    `json:"adapter"`
	SourceSHA256 string             `json:"sourceSha256"`
	StartedAt    string             `json:"startedAt"`
	CompletedAt  string             `json:"completedAt"`
	Outcome      string             `json:"outcome"`
	Failure      *NormalizedFailure `json:"failure"`
}

// AdaptationValidationRun is one normalized phase in successful evidence.
type AdaptationValidationRun struct {
	Phase        string             `json:"phase"`
	SourceSHA256 string             `json:"sourceSha256"`
	StartedAt    string             `json:"startedAt"`
	CompletedAt  string             `json:"completedAt"`
	Outcome      string             `json:"outcome"`
	Failure      *NormalizedFailure `json:"failure"`
}

// AdaptationValidationSource proves the exact executed and restored bytes.
type AdaptationValidationSource struct {
	Path                string `json:"path"`
	OriginalSHA256      string `json:"originalSha256"`
	CandidateSHA256     string `json:"candidateSha256"`
	NegativeSHA256      string `json:"negativeSha256"`
	RestoredSHA256      string `json:"restoredSha256"`
	NegativeControlPath string `json:"negativeControlPath"`
}

// ValidationEvidenceV1 is the successful three-phase public proof.
type ValidationEvidenceV1 struct {
	APIVersion            string                     `json:"apiVersion"`
	PolicyVersion         string                     `json:"policyVersion"`
	ValidationID          string                     `json:"validationId"`
	ProposalID            string                     `json:"proposalId"`
	ProposalPolicyVersion string                     `json:"proposalPolicyVersion"`
	Test                  AdaptationTestReference    `json:"test"`
	Adapter               AdapterIdentity            `json:"adapter"`
	Edit                  AdaptationTextEdit         `json:"edit"`
	Source                AdaptationValidationSource `json:"source"`
	Runs                  []AdaptationValidationRun  `json:"runs"`
}

// AdaptationValidationRejection identifies the policy gate and observed mismatch.
type AdaptationValidationRejection struct {
	Phase           string `json:"phase"`
	Reason          string `json:"reason"`
	ExpectedOutcome string `json:"expectedOutcome"`
	ActualOutcome   string `json:"actualOutcome"`
}

// ValidationRejectionV1 is a trustworthy completed prefix that disproves a candidate.
type ValidationRejectionV1 struct {
	APIVersion            string                        `json:"apiVersion"`
	PolicyVersion         string                        `json:"policyVersion"`
	ValidationID          string                        `json:"validationId"`
	ProposalID            string                        `json:"proposalId"`
	ProposalPolicyVersion string                        `json:"proposalPolicyVersion"`
	Test                  AdaptationTestReference       `json:"test"`
	Adapter               AdapterIdentity               `json:"adapter"`
	Edit                  AdaptationTextEdit            `json:"edit"`
	Source                AdaptationValidationSource    `json:"source"`
	Rejection             AdaptationValidationRejection `json:"rejection"`
	Runs                  []AdaptationValidationRun     `json:"runs"`
}

// ValidateFunctionalAPIRepairValidationRequestV1 validates a typed request.
func ValidateFunctionalAPIRepairValidationRequestV1(document FunctionalAPIRepairValidationRequestV1) error {
	return validateTypedAdaptationDocument(
		document, loadFunctionalAPIRepairValidationRequestV1Schema, "functional API repair validation request",
	)
}

// DecodeFunctionalAPIRepairValidationResultV1 validates and decodes adapter output.
func DecodeFunctionalAPIRepairValidationResultV1(
	data []byte,
) (FunctionalAPIRepairValidationResultV1, error) {
	var document FunctionalAPIRepairValidationResultV1
	if err := validateAdaptationJSON(
		data, loadFunctionalAPIRepairValidationResultV1Schema, "functional API repair validation result",
	); err != nil {
		return document, err
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return document, fmt.Errorf("decode functional API repair validation result: %w", err)
	}

	return document, nil
}

// ValidateValidationEvidenceV1 validates typed successful evidence.
func ValidateValidationEvidenceV1(document ValidationEvidenceV1) error {
	return validateTypedAdaptationDocument(document, loadValidationEvidenceV1Schema, "validation evidence")
}

// DecodeValidationEvidenceV1 validates and decodes public validation evidence.
func DecodeValidationEvidenceV1(data []byte) (ValidationEvidenceV1, error) {
	var document ValidationEvidenceV1
	if err := validateAdaptationJSON(data, loadValidationEvidenceV1Schema, "validation evidence"); err != nil {
		return document, err
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return document, fmt.Errorf("decode validation evidence: %w", err)
	}

	return document, nil
}

// ValidateValidationRejectionV1 validates typed rejection evidence.
func ValidateValidationRejectionV1(document ValidationRejectionV1) error {
	return validateTypedAdaptationDocument(document, loadValidationRejectionV1Schema, "validation rejection")
}

// DecodeValidationRejectionV1 validates and decodes public rejection evidence.
func DecodeValidationRejectionV1(data []byte) (ValidationRejectionV1, error) {
	var document ValidationRejectionV1
	if err := validateAdaptationJSON(data, loadValidationRejectionV1Schema, "validation rejection"); err != nil {
		return document, err
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return document, fmt.Errorf("decode validation rejection: %w", err)
	}

	return document, nil
}
