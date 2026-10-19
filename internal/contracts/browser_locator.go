package contracts

import (
	"encoding/json"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var loadBrowserStructureSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(mustReadSchema("browser-structure/v1/browser-structure.schema.json"), "https://argus.dev/contracts/browser-structure/v1/schema.json", "browser structure")
})
var loadBrowserLocatorDiagnosisSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(mustReadSchema("browser-locator-diagnosis/v1/browser-locator-diagnosis.schema.json"), "https://argus.dev/contracts/browser-locator-diagnosis/v1/schema.json", "browser locator diagnosis")
})
var loadBrowserLocatorProposalSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(mustReadSchema("browser-locator-proposal/v1/browser-locator-proposal.schema.json"), "https://argus.dev/contracts/browser-locator-proposal/v1/schema.json", "browser locator proposal")
})

// BrowserTarget is one structural element used to prove target equivalence.
type BrowserTarget struct {
	Role    string `json:"role"`
	Name    string `json:"name"`
	TestID  string `json:"testId"`
	Visible bool   `json:"visible"`
}

// BrowserLocatorProposalV1 carries an unapplied, exact-preimage browser edit.
type BrowserLocatorProposalV1 struct {
	APIVersion     string             `json:"apiVersion"`
	PolicyVersion  string             `json:"policyVersion"`
	ProposalID     string             `json:"proposalId"`
	BaselineID     string             `json:"baselineAttemptId"`
	FailedID       string             `json:"failedAttemptId"`
	BaselineSHA256 string             `json:"baselineSnapshotSha256"`
	FailedSHA256   string             `json:"failedSnapshotSha256"`
	SuiteKey       string             `json:"suiteKey"`
	TestKey        string             `json:"testKey"`
	OriginalTarget BrowserTarget      `json:"originalTarget"`
	CurrentTarget  BrowserTarget      `json:"currentTarget"`
	Edit           AdaptationTextEdit `json:"edit"`
}

// BrowserLocatorDiagnosisV1 records one proposal or a reasoned abstention.
type BrowserLocatorDiagnosisV1 struct {
	APIVersion string                    `json:"apiVersion"`
	ReasonCode string                    `json:"reasonCode"`
	BaselineID string                    `json:"baselineAttemptId"`
	FailedID   string                    `json:"failedAttemptId"`
	SuiteKey   string                    `json:"suiteKey"`
	TestKey    string                    `json:"testKey"`
	Proposal   *BrowserLocatorProposalV1 `json:"proposal"`
}

// ValidateBrowserStructureV1 checks the Playwright structural artifact schema.
func ValidateBrowserStructureV1(data []byte) error {
	return validateDocument(data, loadBrowserStructureSchema, "browser structure")
}

// ValidateBrowserLocatorDiagnosisV1 checks a reviewable decision envelope.
func ValidateBrowserLocatorDiagnosisV1(value BrowserLocatorDiagnosisV1) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}

	return validateDocument(data, loadBrowserLocatorDiagnosisSchema, "browser locator diagnosis")
}

// ValidateBrowserLocatorProposalV1 checks an unapplied browser edit envelope.
func ValidateBrowserLocatorProposalV1(value BrowserLocatorProposalV1) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}

	return validateDocument(data, loadBrowserLocatorProposalSchema, "browser locator proposal")
}
