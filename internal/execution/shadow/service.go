// Package shadow compares selected execution with its full-suite control.
package shadow

import (
	"context"
	"fmt"
	"slices"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/execution"
)

const (
	// ReportAPIVersion identifies the first selection shadow-report contract.
	ReportAPIVersion = "argus.dev/selection-shadow-report/v1"
)

// MissReason explains why a full-suite failure was not caught in selection.
type MissReason string

const (
	// MissNotSelected means the failing full-suite test was absent from the selected attempt.
	MissNotSelected MissReason = "not-selected"
	// MissNotReproduced means the selected attempt ran the test without the same failure signal.
	MissNotReproduced MissReason = "not-reproduced"
)

// FailureMiss records one full-suite failure not caught by selected execution.
type FailureMiss struct {
	SuiteKey         string
	TestKey          string
	FullSuiteOutcome execution.TestOutcome
	SelectedOutcome  *execution.TestOutcome
	Reason           MissReason
}

// Report is a deterministic comparison of two explicit immutable attempts.
type Report struct {
	APIVersion                  string
	Manifest                    execution.ManifestReference
	TestRepository              catalog.Repository
	TestRevision                catalog.Revision
	AdapterID                   string
	AdapterVersion              string
	SelectedAttemptID           string
	FullSuiteAttemptID          string
	SelectedTestCount           int
	FullSuiteTestCount          int
	SelectedDurationMS          int64
	FullSuiteDurationMS         int64
	DurationReductionMS         int64
	SelectedFailureCount        int
	FullSuiteFailureCount       int
	CaughtFullSuiteFailureCount int
	FailureRecallBasisPoints    *int
	MissedFailures              []FailureMiss
}

// AttemptReader loads immutable attempts by external identity.
type AttemptReader interface {
	FindExecutionAttempt(context.Context, string) (execution.Attempt, error)
}

// Service derives shadow evidence without mutating stored attempts.
type Service struct {
	attempts AttemptReader
}

// NewService constructs the selected-versus-full-suite comparison use case.
func NewService(attempts AttemptReader) *Service {
	return &Service{attempts: attempts}
}

// Compare loads two explicit attempts and derives duration and failure recall.
func (service *Service) Compare(
	ctx context.Context,
	selectedAttemptID string,
	fullSuiteAttemptID string,
) (Report, error) {
	if service == nil || service.attempts == nil ||
		selectedAttemptID == "" || fullSuiteAttemptID == "" ||
		selectedAttemptID == fullSuiteAttemptID {
		return Report{}, execution.ErrInvalid
	}
	selected, err := service.attempts.FindExecutionAttempt(ctx, selectedAttemptID)
	if err != nil {
		return Report{}, err
	}
	fullSuite, err := service.attempts.FindExecutionAttempt(ctx, fullSuiteAttemptID)
	if err != nil {
		return Report{}, err
	}
	selected = execution.CanonicalAttempt(selected)
	fullSuite = execution.CanonicalAttempt(fullSuite)
	if err := validatePair(selected, fullSuite); err != nil {
		return Report{}, err
	}

	return buildReport(selected, fullSuite), nil
}

func validatePair(selected, fullSuite execution.Attempt) error {
	if execution.ValidateAttempt(selected) != nil || execution.ValidateAttempt(fullSuite) != nil {
		return execution.ErrUnavailable
	}
	if selected.Stage != execution.StageSelected || fullSuite.Stage != execution.StageFullSuite ||
		selected.Manifest != fullSuite.Manifest ||
		selected.TestRepository.Identity != fullSuite.TestRepository.Identity ||
		selected.TestRevision != fullSuite.TestRevision ||
		selected.AdapterID != fullSuite.AdapterID || selected.AdapterVersion != fullSuite.AdapterVersion {
		return fmt.Errorf("%w: incompatible shadow attempts", execution.ErrInvalid)
	}
	fullIdentities := make(map[catalog.TestIdentity]struct{}, len(fullSuite.Results))
	for _, result := range fullSuite.Results {
		fullIdentities[result.Identity(fullSuite.TestRepository.Identity)] = struct{}{}
	}
	for _, result := range selected.Results {
		if _, exists := fullIdentities[result.Identity(selected.TestRepository.Identity)]; !exists {
			return fmt.Errorf("%w: full-suite attempt is not a superset", execution.ErrInvalid)
		}
	}

	return nil
}

func buildReport(selected, fullSuite execution.Attempt) Report {
	report := Report{
		APIVersion: ReportAPIVersion, Manifest: selected.Manifest,
		TestRepository: fullSuite.TestRepository, TestRevision: selected.TestRevision,
		AdapterID: selected.AdapterID, AdapterVersion: selected.AdapterVersion,
		SelectedAttemptID: selected.AttemptID, FullSuiteAttemptID: fullSuite.AttemptID,
		SelectedTestCount: len(selected.Results), FullSuiteTestCount: len(fullSuite.Results),
		SelectedDurationMS:  sumDurationMilliseconds(selected.Results),
		FullSuiteDurationMS: sumDurationMilliseconds(fullSuite.Results),
		MissedFailures:      make([]FailureMiss, 0),
	}
	report.DurationReductionMS = report.FullSuiteDurationMS - report.SelectedDurationMS
	selectedByIdentity := make(map[catalog.TestIdentity]execution.TestResult, len(selected.Results))
	for _, result := range selected.Results {
		selectedByIdentity[result.Identity(selected.TestRepository.Identity)] = result
		if isFailure(result.Outcome) {
			report.SelectedFailureCount++
		}
	}
	for _, result := range fullSuite.Results {
		if !isFailure(result.Outcome) {
			continue
		}
		report.FullSuiteFailureCount++
		selectedResult, exists := selectedByIdentity[result.Identity(fullSuite.TestRepository.Identity)]
		if exists && isFailure(selectedResult.Outcome) {
			report.CaughtFullSuiteFailureCount++

			continue
		}
		report.MissedFailures = append(report.MissedFailures, newFailureMiss(result, selectedResult, exists))
	}
	if report.FullSuiteFailureCount != 0 {
		recall := report.CaughtFullSuiteFailureCount * 10_000 / report.FullSuiteFailureCount
		report.FailureRecallBasisPoints = &recall
	}
	slices.SortFunc(report.MissedFailures, func(left, right FailureMiss) int {
		if left.SuiteKey < right.SuiteKey {
			return -1
		}
		if left.SuiteKey > right.SuiteKey {
			return 1
		}
		if left.TestKey < right.TestKey {
			return -1
		}
		if left.TestKey > right.TestKey {
			return 1
		}

		return 0
	})

	return report
}

func newFailureMiss(
	fullSuite execution.TestResult,
	selected execution.TestResult,
	selectedExists bool,
) FailureMiss {
	miss := FailureMiss{
		SuiteKey: fullSuite.SuiteKey, TestKey: fullSuite.TestKey,
		FullSuiteOutcome: fullSuite.Outcome, Reason: MissNotSelected,
	}
	if selectedExists {
		outcome := selected.Outcome
		miss.SelectedOutcome = &outcome
		miss.Reason = MissNotReproduced
	}

	return miss
}

func sumDurationMilliseconds(results []execution.TestResult) int64 {
	var total int64
	for _, result := range results {
		total += result.Duration.Milliseconds()
	}

	return total
}

func isFailure(outcome execution.TestOutcome) bool {
	return outcome == execution.TestFailed || outcome == execution.TestError
}
