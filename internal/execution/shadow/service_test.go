package shadow

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/execution"
)

func TestCompareReportsOmittedAndNonReproducedFailures(t *testing.T) {
	t.Parallel()
	selected := shadowAttempt(execution.StageSelected, "selected-1", []execution.TestResult{
		result("orders", "create", execution.TestFailed, time.Second),
		result("orders", "update", execution.TestPassed, time.Second),
	})
	fullSuite := shadowAttempt(execution.StageFullSuite, "full-1", []execution.TestResult{
		result("orders", "create", execution.TestFailed, time.Second),
		result("orders", "update", execution.TestFailed, 2*time.Second),
		result("orders", "delete", execution.TestError, 3*time.Second),
	})
	report, err := NewService(stubReader{attempts: map[string]execution.Attempt{
		selected.AttemptID: selected, fullSuite.AttemptID: fullSuite,
	}}).Compare(t.Context(), selected.AttemptID, fullSuite.AttemptID)
	if err != nil {
		t.Fatalf("compare attempts: %v", err)
	}
	if report.FullSuiteFailureCount != 3 || report.CaughtFullSuiteFailureCount != 1 ||
		report.FailureRecallBasisPoints == nil || *report.FailureRecallBasisPoints != 3333 ||
		len(report.MissedFailures) != 2 || report.DurationReductionMS != 4000 {
		t.Fatalf("report = %+v", report)
	}
	if report.MissedFailures[0].Reason != MissNotSelected ||
		report.MissedFailures[1].Reason != MissNotReproduced {
		t.Fatalf("misses = %+v", report.MissedFailures)
	}
}

func TestCompareRejectsIncompatibleAttemptPairs(t *testing.T) {
	t.Parallel()
	selected := shadowAttempt(execution.StageSelected, "selected-1", []execution.TestResult{
		result("orders", "create", execution.TestPassed, time.Second),
	})
	fullSuite := shadowAttempt(execution.StageFullSuite, "full-1", []execution.TestResult{
		result("orders", "create", execution.TestPassed, time.Second),
	})
	fullSuite.AdapterVersion = "different"
	_, err := NewService(stubReader{attempts: map[string]execution.Attempt{
		selected.AttemptID: selected, fullSuite.AttemptID: fullSuite,
	}}).Compare(t.Context(), selected.AttemptID, fullSuite.AttemptID)
	if !errors.Is(err, execution.ErrInvalid) {
		t.Fatalf("incompatible pair error = %v", err)
	}
}

func TestCompareReportsNullRecallAndEmptyMissesWithoutControlFailures(t *testing.T) {
	t.Parallel()
	selected := shadowAttempt(execution.StageSelected, "selected-1", []execution.TestResult{
		result("orders", "create", execution.TestPassed, time.Second),
	})
	fullSuite := shadowAttempt(execution.StageFullSuite, "full-1", []execution.TestResult{
		result("orders", "create", execution.TestPassed, time.Second),
	})
	report, err := NewService(stubReader{attempts: map[string]execution.Attempt{
		selected.AttemptID: selected, fullSuite.AttemptID: fullSuite,
	}}).Compare(t.Context(), selected.AttemptID, fullSuite.AttemptID)
	if err != nil {
		t.Fatalf("compare passing attempts: %v", err)
	}
	if report.FailureRecallBasisPoints != nil || report.MissedFailures == nil ||
		len(report.MissedFailures) != 0 {
		t.Fatalf("passing shadow report = %+v", report)
	}
}

type stubReader struct {
	attempts map[string]execution.Attempt
}

func (reader stubReader) FindExecutionAttempt(
	_ context.Context,
	attemptID string,
) (execution.Attempt, error) {
	attempt, exists := reader.attempts[attemptID]
	if !exists {
		return execution.Attempt{}, execution.ErrNotFound
	}

	return attempt, nil
}

func shadowAttempt(
	stage execution.Stage,
	attemptID string,
	results []execution.TestResult,
) execution.Attempt {
	repository := catalog.Repository{
		Identity: catalog.RepositoryIdentity{
			Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "tests-1",
		},
		Owner: "example", Name: "orders-tests",
	}

	return execution.Attempt{
		APIVersion: execution.AttemptAPIVersion, AttemptID: attemptID,
		Manifest: execution.ManifestReference{
			APIVersion: "argus.dev/execution-manifest/v1",
			SHA256:     "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		},
		Stage: stage, TestRepository: repository,
		TestRevision: catalog.Revision{
			Algorithm: catalog.RevisionGitSHA1, Digest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
		AdapterID: "playwright", AdapterVersion: "1.58.2",
		StartedAt:   time.Date(2026, 9, 26, 9, 0, 0, 0, time.UTC),
		CompletedAt: time.Date(2026, 9, 26, 9, 1, 0, 0, time.UTC),
		Outcome:     execution.DeriveAttemptOutcome(results), Results: results,
		Artifacts: []execution.ArtifactReference{},
	}
}

func result(
	suiteKey, testKey string,
	outcome execution.TestOutcome,
	duration time.Duration,
) execution.TestResult {
	value := execution.TestResult{
		SuiteKey: suiteKey, TestKey: testKey, Outcome: outcome, Duration: duration,
	}
	if outcome == execution.TestFailed || outcome == execution.TestError {
		value.Failure = &execution.Failure{Code: "test-failure", Message: "failure"}
	}

	return value
}
