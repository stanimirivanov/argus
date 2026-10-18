// Package browsershadow compares explicit browser attempts against every planned control.
package browsershadow

import (
	"context"
	"fmt"
	"regexp"

	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/execution/browser"
	"github.com/stanimirivanov/argus/internal/execution/browserplanning"
)

const (
	// BindingsVersion identifies explicit attempt IDs for a browser plan.
	BindingsVersion = "argus.dev/browser-plan-attempt-bindings/v1"
	// ReportVersion identifies complete-plan browser shadow evidence.
	ReportVersion = "argus.dev/browser-plan-shadow-report/v1"
)

var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// GroupBinding names the immutable attempts for one planned browser group.
type GroupBinding struct {
	GroupKey           string
	SelectedAttemptID  *string
	FullSuiteAttemptID string
}

// Bindings covers every group in one plan without implicit attempt lookup.
type Bindings struct {
	APIVersion string
	Groups     []GroupBinding
}

// Miss is a full-suite failure absent from the selected-stage failures.
type Miss struct {
	SuiteKey         string
	TestKey          string
	FullSuiteOutcome execution.TestOutcome
	SelectedOutcome  *execution.TestOutcome
	Reason           string
}

// Metrics uses summed test durations, not CI wall-clock time.
type Metrics struct {
	SelectedTestCount           int
	FullSuiteTestCount          int
	SelectedDurationMS          int64
	FullSuiteDurationMS         int64
	DurationReductionMS         int64
	SelectedFailureCount        int
	FullSuiteFailureCount       int
	CaughtFullSuiteFailureCount int
	FailureRecallBasisPoints    *int
}

// GroupReport compares one explicit repository/project/environment group.
type GroupReport struct {
	browserplanning.Binding
	AdapterVersion     string
	SelectedAttemptID  *string
	FullSuiteAttemptID string
	Metrics
	MissedFailures []Miss
}

// Report aggregates all planned groups; a missing group prevents its creation.
type Report struct {
	APIVersion          string
	PlanSHA256          string
	Manifest            execution.ManifestReference
	GroupCount          int
	GroupsWithSelection int
	FullOnlyGroupCount  int
	Metrics
	Groups []GroupReport
}

// AttemptReader retrieves immutable browser attempts by explicit ID.
type AttemptReader interface {
	FindBrowserAttempt(context.Context, string) (browser.Attempt, error)
}

func load(ctx context.Context, reader AttemptReader, id string, manifest execution.ManifestReference, job browserplanning.Job) (browser.Attempt, error) {
	attempt, err := reader.FindBrowserAttempt(ctx, id)
	if err != nil {
		return browser.Attempt{}, err
	}
	attempt = browser.CanonicalAttempt(attempt)
	if browser.ValidateAttempt(attempt) != nil {
		return browser.Attempt{}, execution.ErrUnavailable
	}
	r := attempt.Request
	if r.AttemptID != id || r.Manifest != manifest || r.Stage != job.Stage || r.TestRepository != job.TestRepository || r.TestRevision != job.TestRevision || r.Adapter != job.Adapter || r.Project != job.Project || r.Environment != job.Environment || len(r.Tests) != job.TestCount {
		return browser.Attempt{}, fmt.Errorf("%w: attempt does not satisfy browser plan job", browser.ErrInvalid)
	}

	return attempt, nil
}
func duration(results []execution.TestResult) int64 {
	var n int64
	for _, r := range results {
		n += r.Duration.Milliseconds()
	}

	return n
}
func failed(o execution.TestOutcome) bool {
	return o == execution.TestFailed || o == execution.TestError
}
