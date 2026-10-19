package contracts

import (
	"encoding/json"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// BrowserLocatorValidationV1 is the flattened public proof or rejection document.
type BrowserLocatorValidationV1 struct {
	APIVersion    string                    `json:"apiVersion"`
	PolicyVersion string                    `json:"policyVersion"`
	ValidationID  string                    `json:"validationId"`
	Status        string                    `json:"status"`
	Reason        string                    `json:"reason,omitempty"`
	Proposal      BrowserLocatorProposalV1  `json:"proposal"`
	Baseline      BrowserExecutionAttemptV1 `json:"baseline"`
	Failed        BrowserExecutionAttemptV1 `json:"failed"`
	Source        BrowserValidationSource   `json:"source"`
	Runs          []BrowserValidationRun    `json:"runs"`
}

// BrowserValidationSource identifies the three source variants and restoration.
type BrowserValidationSource struct {
	Path            string `json:"path"`
	OriginalSHA256  string `json:"originalSha256"`
	CandidateSHA256 string `json:"candidateSha256"`
	NegativeSHA256  string `json:"negativeSha256"`
	RestoredSHA256  string `json:"restoredSha256"`
	NegativeLocator string `json:"negativeLocator"`
}

// BrowserValidationRun binds one execution to its source variant.
type BrowserValidationRun struct {
	Phase        string                    `json:"phase"`
	Repeat       int                       `json:"repeat"`
	SourceSHA256 string                    `json:"sourceSha256"`
	Attempt      BrowserExecutionAttemptV1 `json:"attempt"`
}

var loadBrowserLocatorValidationSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(mustReadSchema("browser-locator-validation/v1/browser-locator-validation.schema.json"),
		"https://argus.dev/contracts/browser-locator-validation/v1/schema.json", "browser locator validation")
})

// ValidateBrowserLocatorValidationV1 checks the external structural contract.
// Domain policy validation remains owned by browservalidation.ValidateEvidence.
func ValidateBrowserLocatorValidationV1(data []byte) error {
	return validateDocument(data, loadBrowserLocatorValidationSchema, "browser locator validation")
}

// DecodeBrowserLocatorValidationV1 validates an uploaded public document.
func DecodeBrowserLocatorValidationV1(data []byte) (BrowserLocatorValidationV1, error) {
	var document BrowserLocatorValidationV1
	if err := ValidateBrowserLocatorValidationV1(data); err != nil {
		return document, err
	}
	err := json.Unmarshal(data, &document)

	return document, err
}
