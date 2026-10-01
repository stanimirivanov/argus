package executioncli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/contracts"
	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/execution/functionalapi"
)

func TestRunConsumesManifestAndEmitsValidatedAttempt(t *testing.T) {
	t.Parallel()
	manifestPath := filepath.Join(
		"..", "..", "..", "..", "..", "contracts", "fixtures",
		"execution-manifest", "v1", "valid", "targeted.json",
	)
	var output bytes.Buffer
	err := Run(
		t.Context(),
		[]string{
			"-manifest", manifestPath,
			"-attempt-id", "github-123-1",
			"-test-repository-id", "tests-1",
			"-test-revision", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"-adapter", "playwright",
			"--", "fake-adapter", "--reporter=argus",
		},
		bytes.NewReader(nil), &output, io.Discard,
		func(command []string, _ io.Writer) (functionalapi.Adapter, error) {
			if len(command) != 2 || command[0] != "fake-adapter" {
				t.Fatalf("adapter command = %v", command)
			}

			return successfulAdapter{}, nil
		},
	)
	if err != nil {
		t.Fatalf("run CLI: %v", err)
	}
	var attempt contracts.ExecutionAttemptV1
	if err := json.Unmarshal(output.Bytes(), &attempt); err != nil {
		t.Fatalf("decode attempt: %v", err)
	}
	if err := contracts.ValidateExecutionAttemptV1(attempt); err != nil {
		t.Fatalf("validate attempt: %v", err)
	}
	if attempt.Outcome != "passed" || len(attempt.Results) != 1 ||
		attempt.Results[0].TestKey != "create-order-test" {
		t.Fatalf("attempt = %+v", attempt)
	}
}

func TestRunValidatesArgumentsBeforeOpeningAdapter(t *testing.T) {
	t.Parallel()
	if err := Run(t.Context(), nil, nil, io.Discard, io.Discard, nil); err == nil {
		t.Fatal("expected usage error")
	}
}

func TestRunEmitsFailedAttemptBeforeReturningNonPassingStatus(t *testing.T) {
	t.Parallel()
	manifestPath := filepath.Join(
		"..", "..", "..", "..", "..", "contracts", "fixtures",
		"execution-manifest", "v1", "valid", "targeted.json",
	)
	var output bytes.Buffer
	err := Run(
		t.Context(),
		[]string{
			"-manifest", manifestPath, "-attempt-id", "github-123-2",
			"-test-repository-id", "tests-1",
			"-test-revision", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"-adapter", "playwright", "--", "fake-adapter",
		},
		nil, &output, io.Discard,
		func([]string, io.Writer) (functionalapi.Adapter, error) {
			return outcomeAdapter{outcome: execution.TestFailed}, nil
		},
	)
	if !errors.Is(err, execution.ErrAttemptNotPassed) {
		t.Fatalf("failed attempt error = %v", err)
	}
	var attempt contracts.ExecutionAttemptV1
	if json.Unmarshal(output.Bytes(), &attempt) != nil || attempt.Outcome != "failed" {
		t.Fatalf("failed attempt output = %s", output.String())
	}
}

type successfulAdapter struct{}

func (successfulAdapter) Execute(
	_ context.Context,
	request functionalapi.Request,
) (functionalapi.AdapterResult, error) {
	return outcomeAdapter{outcome: execution.TestPassed}.Execute(context.Background(), request)
}

type outcomeAdapter struct {
	outcome execution.TestOutcome
}

func (adapter outcomeAdapter) Execute(
	_ context.Context,
	request functionalapi.Request,
) (functionalapi.AdapterResult, error) {
	results := make([]execution.TestResult, 0, len(request.Tests))
	for _, test := range request.Tests {
		result := execution.TestResult{
			SuiteKey: test.SuiteKey, TestKey: test.TestKey,
			Outcome: adapter.outcome, Duration: 25 * time.Millisecond,
		}
		if adapter.outcome == execution.TestFailed || adapter.outcome == execution.TestError {
			result.Failure = &execution.Failure{Code: "assertion-failed", Message: "expected 201"}
		}
		results = append(results, result)
	}

	return functionalapi.AdapterResult{
		APIVersion: functionalapi.AdapterResultAPIVersion,
		AttemptID:  request.AttemptID, AdapterID: request.Adapter, AdapterVersion: "test-v1",
		StartedAt:   time.Date(2026, 9, 19, 10, 0, 0, 0, time.UTC),
		CompletedAt: time.Date(2026, 9, 19, 10, 0, 1, 0, time.UTC),
		Results:     results, Artifacts: []execution.ArtifactReference{},
	}, nil
}
