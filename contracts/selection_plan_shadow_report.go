package contracts

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	// ExecutionPlanAttemptBindingsV1APIVersion identifies explicit attempt bindings v1.
	ExecutionPlanAttemptBindingsV1APIVersion = "argus.dev/execution-plan-attempt-bindings/v1"
	// SelectionPlanShadowReportV1APIVersion identifies aggregate plan evidence v1.
	SelectionPlanShadowReportV1APIVersion = "argus.dev/selection-plan-shadow-report/v1"
)

//go:embed generated/execution-plan-attempt-bindings/v1/execution-plan-attempt-bindings.schema.json
var executionPlanAttemptBindingsV1SchemaJSON []byte

//go:embed generated/selection-plan-shadow-report/v1/selection-plan-shadow-report.schema.json
var selectionPlanShadowReportV1SchemaJSON []byte

var loadExecutionPlanAttemptBindingsV1Schema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(
		executionPlanAttemptBindingsV1SchemaJSON,
		"https://argus.dev/contracts/execution-plan-attempt-bindings/v1/schema.json",
		"execution plan attempt bindings",
	)
})

var loadSelectionPlanShadowReportV1Schema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(
		selectionPlanShadowReportV1SchemaJSON,
		"https://argus.dev/contracts/selection-plan-shadow-report/v1/schema.json",
		"selection plan shadow report",
	)
})

// ExecutionPlanAttemptGroupBinding names attempts for one planned group.
type ExecutionPlanAttemptGroupBinding struct {
	GroupKey           string  `json:"groupKey"`
	SelectedAttemptID  *string `json:"selectedAttemptId"`
	FullSuiteAttemptID string  `json:"fullSuiteAttemptId"`
}

// ExecutionPlanAttemptBindingsV1 supplies explicit attempt IDs for a plan.
type ExecutionPlanAttemptBindingsV1 struct {
	APIVersion string                             `json:"apiVersion"`
	Groups     []ExecutionPlanAttemptGroupBinding `json:"groups"`
}

// PlanDigestReference identifies canonical execution-plan JSON.
type PlanDigestReference struct {
	APIVersion string `json:"apiVersion"`
	SHA256     string `json:"sha256"`
}

// SelectionPlanShadowGroup contains one plan group's comparison evidence.
type SelectionPlanShadowGroup struct {
	GroupKey                    string              `json:"groupKey"`
	TestRepository              RepositoryReference `json:"testRepository"`
	TestRevision                RevisionReference   `json:"testRevision"`
	Adapter                     AdapterIdentity     `json:"adapter"`
	SelectedAttemptID           *string             `json:"selectedAttemptId"`
	FullSuiteAttemptID          string              `json:"fullSuiteAttemptId"`
	SelectedTestCount           int                 `json:"selectedTestCount"`
	FullSuiteTestCount          int                 `json:"fullSuiteTestCount"`
	SelectedDurationMS          int64               `json:"selectedDurationMs"`
	FullSuiteDurationMS         int64               `json:"fullSuiteDurationMs"`
	DurationReductionMS         int64               `json:"durationReductionMs"`
	SelectedFailureCount        int                 `json:"selectedFailureCount"`
	FullSuiteFailureCount       int                 `json:"fullSuiteFailureCount"`
	CaughtFullSuiteFailureCount int                 `json:"caughtFullSuiteFailureCount"`
	FailureRecallBasisPoints    *int                `json:"failureRecallBasisPoints"`
	MissedFailures              []ShadowFailureMiss `json:"missedFailures"`
}

// SelectionPlanShadowReportV1 aggregates shadow evidence for an entire plan.
type SelectionPlanShadowReportV1 struct {
	APIVersion                  string                     `json:"apiVersion"`
	Plan                        PlanDigestReference        `json:"plan"`
	Manifest                    ManifestDigestReference    `json:"manifest"`
	GroupCount                  int                        `json:"groupCount"`
	GroupsWithSelection         int                        `json:"groupsWithSelection"`
	FullOnlyGroupCount          int                        `json:"fullOnlyGroupCount"`
	SelectedTestCount           int                        `json:"selectedTestCount"`
	FullSuiteTestCount          int                        `json:"fullSuiteTestCount"`
	SelectedDurationMS          int64                      `json:"selectedDurationMs"`
	FullSuiteDurationMS         int64                      `json:"fullSuiteDurationMs"`
	DurationReductionMS         int64                      `json:"durationReductionMs"`
	SelectedFailureCount        int                        `json:"selectedFailureCount"`
	FullSuiteFailureCount       int                        `json:"fullSuiteFailureCount"`
	CaughtFullSuiteFailureCount int                        `json:"caughtFullSuiteFailureCount"`
	FailureRecallBasisPoints    *int                       `json:"failureRecallBasisPoints"`
	Groups                      []SelectionPlanShadowGroup `json:"groups"`
}

// DecodeExecutionPlanAttemptBindingsV1 validates and decodes attempt bindings.
func DecodeExecutionPlanAttemptBindingsV1(data []byte) (ExecutionPlanAttemptBindingsV1, error) {
	var document ExecutionPlanAttemptBindingsV1
	if err := validateExecutionJSON(
		data, loadExecutionPlanAttemptBindingsV1Schema, "execution plan attempt bindings",
	); err != nil {
		return document, err
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return document, fmt.Errorf("decode execution plan attempt bindings: %w", err)
	}

	return document, nil
}

// ValidateExecutionPlanAttemptBindingsV1 validates typed attempt bindings.
func ValidateExecutionPlanAttemptBindingsV1(document ExecutionPlanAttemptBindingsV1) error {
	return validateTypedExecutionDocument(
		document, loadExecutionPlanAttemptBindingsV1Schema, "execution plan attempt bindings",
	)
}

// ValidateSelectionPlanShadowReportV1 validates a typed aggregate report.
func ValidateSelectionPlanShadowReportV1(document SelectionPlanShadowReportV1) error {
	return validateTypedExecutionDocument(
		document, loadSelectionPlanShadowReportV1Schema, "selection plan shadow report",
	)
}
