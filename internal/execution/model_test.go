package execution

import (
	"errors"
	"testing"
	"time"
)

func TestValidateAdapterResultRejectsSubMicrosecondTimestamps(t *testing.T) {
	t.Parallel()
	result := AdapterResult{
		APIVersion: FunctionalAPIAdapterResultAPIVersion,
		AttemptID:  "attempt-1", AdapterID: "generic-http", AdapterVersion: "1.0.0",
		StartedAt:   time.Date(2026, 9, 26, 8, 0, 0, 1, time.UTC),
		CompletedAt: time.Date(2026, 9, 26, 8, 0, 1, 0, time.UTC),
		Results: []TestResult{{
			SuiteKey: "orders", TestKey: "create", Outcome: TestPassed,
			Duration: time.Second,
		}},
		Artifacts: []ArtifactReference{},
	}

	if err := ValidateAdapterResult(result); !errors.Is(err, ErrInvalid) {
		t.Fatalf("sub-microsecond timestamp error = %v, want ErrInvalid", err)
	}
}
