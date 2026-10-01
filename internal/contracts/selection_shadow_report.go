package contracts

import (
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	// SelectionShadowReportV1APIVersion identifies shadow comparison report v1.
	SelectionShadowReportV1APIVersion = "argus.dev/selection-shadow-report/v1"
	selectionShadowReportV1SchemaID   = "https://argus.dev/contracts/selection-shadow-report/v1/schema.json"
)

var selectionShadowReportV1SchemaJSON = mustReadSchema("selection-shadow-report/v1/selection-shadow-report.schema.json")

var loadSelectionShadowReportV1Schema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(
		selectionShadowReportV1SchemaJSON,
		selectionShadowReportV1SchemaID,
		"selection shadow report",
	)
})

// ShadowFailureMiss records one full-suite failure not caught by selection.
type ShadowFailureMiss struct {
	SuiteKey         string  `json:"suiteKey"`
	TestKey          string  `json:"testKey"`
	FullSuiteOutcome string  `json:"fullSuiteOutcome"`
	SelectedOutcome  *string `json:"selectedOutcome"`
	Reason           string  `json:"reason"`
}

// SelectionShadowReportV1 compares two explicit immutable execution attempts.
type SelectionShadowReportV1 struct {
	APIVersion                  string                  `json:"apiVersion"`
	Manifest                    ManifestDigestReference `json:"manifest"`
	TestRepository              RepositoryReference     `json:"testRepository"`
	TestRevision                RevisionReference       `json:"testRevision"`
	Adapter                     AdapterIdentity         `json:"adapter"`
	SelectedAttemptID           string                  `json:"selectedAttemptId"`
	FullSuiteAttemptID          string                  `json:"fullSuiteAttemptId"`
	SelectedTestCount           int                     `json:"selectedTestCount"`
	FullSuiteTestCount          int                     `json:"fullSuiteTestCount"`
	SelectedDurationMS          int64                   `json:"selectedDurationMs"`
	FullSuiteDurationMS         int64                   `json:"fullSuiteDurationMs"`
	DurationReductionMS         int64                   `json:"durationReductionMs"`
	SelectedFailureCount        int                     `json:"selectedFailureCount"`
	FullSuiteFailureCount       int                     `json:"fullSuiteFailureCount"`
	CaughtFullSuiteFailureCount int                     `json:"caughtFullSuiteFailureCount"`
	FailureRecallBasisPoints    *int                    `json:"failureRecallBasisPoints"`
	MissedFailures              []ShadowFailureMiss     `json:"missedFailures"`
}

// ValidateSelectionShadowReportV1 validates a typed shadow report.
func ValidateSelectionShadowReportV1(document SelectionShadowReportV1) error {
	return validateTypedExecutionDocument(document, loadSelectionShadowReportV1Schema, "selection shadow report")
}
