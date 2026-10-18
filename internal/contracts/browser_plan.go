package contracts

import (
	"encoding/json"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var loadBrowserBindingsSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(mustReadSchema("browser-execution-bindings/v1/browser-execution-bindings.schema.json"), "https://argus.dev/contracts/browser-execution-bindings/v1/schema.json", "browser execution bindings")
})
var loadBrowserPlanSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(mustReadSchema("browser-execution-plan/v1/browser-execution-plan.schema.json"), "https://argus.dev/contracts/browser-execution-plan/v1/schema.json", "browser execution plan")
})
var loadBrowserAttemptBindingsSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(mustReadSchema("browser-plan-attempt-bindings/v1/browser-plan-attempt-bindings.schema.json"), "https://argus.dev/contracts/browser-plan-attempt-bindings/v1/schema.json", "browser plan attempt bindings")
})
var loadBrowserReportSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(mustReadSchema("browser-plan-shadow-report/v1/browser-plan-shadow-report.schema.json"), "https://argus.dev/contracts/browser-plan-shadow-report/v1/schema.json", "browser plan shadow report")
})

// BrowserExecutionGroupBinding is one reviewed CI checkout and execution dimension.
type BrowserExecutionGroupBinding struct {
	GroupKey       string              `json:"groupKey"`
	TestRepository RepositoryReference `json:"testRepository"`
	TestRevision   RevisionReference   `json:"testRevision"`
	Adapter        string              `json:"adapter"`
	Project        string              `json:"project"`
	Environment    string              `json:"environment"`
}

// BrowserExecutionBindingsV1 is reviewed browser checkout configuration.
type BrowserExecutionBindingsV1 struct {
	APIVersion string                         `json:"apiVersion"`
	Groups     []BrowserExecutionGroupBinding `json:"groups"`
}

// BrowserExecutionJob is one selected or full-suite CI matrix entry.
type BrowserExecutionJob struct {
	BrowserExecutionGroupBinding
	Stage     string `json:"stage"`
	TestCount int    `json:"testCount"`
}

// BrowserExecutionPlanV1 binds flattened jobs to a browser manifest digest.
type BrowserExecutionPlanV1 struct {
	APIVersion string                  `json:"apiVersion"`
	Manifest   ManifestDigestReference `json:"manifest"`
	Jobs       []BrowserExecutionJob   `json:"jobs"`
}

// BrowserPlanAttemptGroupBinding names explicit stage attempts for one group.
type BrowserPlanAttemptGroupBinding struct {
	GroupKey           string  `json:"groupKey"`
	SelectedAttemptID  *string `json:"selectedAttemptId"`
	FullSuiteAttemptID string  `json:"fullSuiteAttemptId"`
}

// BrowserPlanAttemptBindingsV1 covers every group in a browser plan.
type BrowserPlanAttemptBindingsV1 struct {
	APIVersion string                           `json:"apiVersion"`
	Groups     []BrowserPlanAttemptGroupBinding `json:"groups"`
}

// BrowserPlanMetrics contains bounded test counts, durations, and recall.
type BrowserPlanMetrics struct {
	SelectedTestCount           int   `json:"selectedTestCount"`
	FullSuiteTestCount          int   `json:"fullSuiteTestCount"`
	SelectedDurationMS          int64 `json:"selectedDurationMs"`
	FullSuiteDurationMS         int64 `json:"fullSuiteDurationMs"`
	DurationReductionMS         int64 `json:"durationReductionMs"`
	SelectedFailureCount        int   `json:"selectedFailureCount"`
	FullSuiteFailureCount       int   `json:"fullSuiteFailureCount"`
	CaughtFullSuiteFailureCount int   `json:"caughtFullSuiteFailureCount"`
	FailureRecallBasisPoints    *int  `json:"failureRecallBasisPoints"`
}

// BrowserFailureMiss identifies a full-suite failure missed by selection.
type BrowserFailureMiss struct {
	SuiteKey         string  `json:"suiteKey"`
	TestKey          string  `json:"testKey"`
	FullSuiteOutcome string  `json:"fullSuiteOutcome"`
	SelectedOutcome  *string `json:"selectedOutcome"`
	Reason           string  `json:"reason"`
}

// BrowserPlanShadowGroup compares one reviewed browser execution group.
type BrowserPlanShadowGroup struct {
	BrowserExecutionGroupBinding
	AdapterVersion     string  `json:"adapterVersion"`
	SelectedAttemptID  *string `json:"selectedAttemptId"`
	FullSuiteAttemptID string  `json:"fullSuiteAttemptId"`
	BrowserPlanMetrics
	MissedFailures []BrowserFailureMiss `json:"missedFailures"`
}

// BrowserPlanShadowReportV1 summarizes all explicit group comparisons.
type BrowserPlanShadowReportV1 struct {
	APIVersion          string                  `json:"apiVersion"`
	Plan                PlanDigestReference     `json:"plan"`
	Manifest            ManifestDigestReference `json:"manifest"`
	GroupCount          int                     `json:"groupCount"`
	GroupsWithSelection int                     `json:"groupsWithSelection"`
	FullOnlyGroupCount  int                     `json:"fullOnlyGroupCount"`
	BrowserPlanMetrics
	Groups []BrowserPlanShadowGroup `json:"groups"`
}

// DecodeBrowserExecutionBindingsV1 validates reviewed browser bindings.
func DecodeBrowserExecutionBindingsV1(data []byte) (BrowserExecutionBindingsV1, error) {
	var v BrowserExecutionBindingsV1
	err := validateDocument(data, loadBrowserBindingsSchema, "browser execution bindings")
	if err == nil {
		err = json.Unmarshal(data, &v)
	}

	return v, err
}

// DecodeBrowserExecutionPlanV1 validates a flattened browser job plan.
func DecodeBrowserExecutionPlanV1(data []byte) (BrowserExecutionPlanV1, error) {
	var v BrowserExecutionPlanV1
	err := validateDocument(data, loadBrowserPlanSchema, "browser execution plan")
	if err == nil {
		err = json.Unmarshal(data, &v)
	}

	return v, err
}

// DecodeBrowserPlanAttemptBindingsV1 validates explicit attempt IDs.
func DecodeBrowserPlanAttemptBindingsV1(data []byte) (BrowserPlanAttemptBindingsV1, error) {
	var v BrowserPlanAttemptBindingsV1
	err := validateDocument(data, loadBrowserAttemptBindingsSchema, "browser plan attempt bindings")
	if err == nil {
		err = json.Unmarshal(data, &v)
	}

	return v, err
}

// ValidateBrowserExecutionPlanV1 checks a typed browser plan against its schema.
func ValidateBrowserExecutionPlanV1(v BrowserExecutionPlanV1) error {
	return validateTypedExecutionDocument(v, loadBrowserPlanSchema, "browser execution plan")
}

// ValidateBrowserPlanShadowReportV1 checks a typed browser report against its schema.
func ValidateBrowserPlanShadowReportV1(v BrowserPlanShadowReportV1) error {
	return validateTypedExecutionDocument(v, loadBrowserReportSchema, "browser plan shadow report")
}
