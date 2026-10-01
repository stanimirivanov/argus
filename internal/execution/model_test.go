package execution_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/execution"
)

func TestValidateResultEvidenceRejectsSubMicrosecondTimestamps(t *testing.T) {
	t.Parallel()
	err := execution.ValidateResultEvidence(
		time.Date(2026, 9, 26, 8, 0, 0, 1, time.UTC),
		time.Date(2026, 9, 26, 8, 0, 1, 0, time.UTC),
		[]execution.TestResult{{SuiteKey: "orders", TestKey: "create", Outcome: execution.TestPassed, Duration: time.Second}},
		[]execution.ArtifactReference{},
	)
	if !errors.Is(err, execution.ErrInvalid) {
		t.Fatalf("sub-microsecond timestamp error = %v, want ErrInvalid", err)
	}
}

func TestCanonicalEvidenceCopiesFailureBeforeSorting(t *testing.T) {
	t.Parallel()
	failure := &execution.Failure{Code: "assertion-failed", Message: "old"}
	results := []execution.TestResult{
		{SuiteKey: "z", TestKey: "z", Outcome: execution.TestFailed, Failure: failure},
		{SuiteKey: "a", TestKey: "a", Outcome: execution.TestPassed},
	}
	canonical, _ := execution.CanonicalEvidence(results, nil)
	failure.Message = "changed"
	if canonical[0].SuiteKey != "a" || canonical[1].Failure.Message != "old" {
		t.Fatalf("canonical evidence aliases input or is unordered: %+v", canonical)
	}
}
