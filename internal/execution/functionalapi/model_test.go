package functionalapi

import (
	"errors"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/selection"
)

func TestValidateRequestRejectsDuplicateTestsAndWrongProtocol(t *testing.T) {
	t.Parallel()
	request := Request{
		APIVersion: AdapterRequestAPIVersion, AttemptID: "attempt-1",
		Manifest: execution.ManifestReference{APIVersion: selection.ManifestAPIVersion,
			SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		Stage: execution.StageSelected,
		TestRepository: catalog.Repository{
			Identity: catalog.RepositoryIdentity{Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "tests-1"},
			Owner:    "example", Name: "api-tests",
		},
		TestRevision: catalog.Revision{Algorithm: catalog.RevisionGitSHA1,
			Digest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		Adapter: "api-runner", Tests: []Test{{SuiteKey: "orders", TestKey: "create", Name: "create order"}},
	}
	if err := ValidateRequest(request); err != nil {
		t.Fatalf("valid request: %v", err)
	}
	duplicate := request
	duplicate.Tests = append(append([]Test{}, request.Tests...), request.Tests[0])
	if err := ValidateRequest(duplicate); !errors.Is(err, execution.ErrInvalid) {
		t.Fatalf("duplicate request error = %v", err)
	}
	request.APIVersion = "argus.dev/browser-adapter-request/v1"
	if err := ValidateRequest(request); !errors.Is(err, execution.ErrInvalid) {
		t.Fatalf("foreign protocol error = %v", err)
	}
}

func TestValidateAdapterResultRejectsSubMicrosecondTimestamps(t *testing.T) {
	t.Parallel()
	result := AdapterResult{
		APIVersion: AdapterResultAPIVersion, AttemptID: "attempt-1",
		AdapterID: "api-runner", AdapterVersion: "1.0.0",
		StartedAt:   time.Date(2026, 9, 26, 8, 0, 0, 1, time.UTC),
		CompletedAt: time.Date(2026, 9, 26, 8, 0, 1, 0, time.UTC),
		Results: []execution.TestResult{{SuiteKey: "orders", TestKey: "create",
			Outcome: execution.TestPassed, Duration: time.Second}},
		Artifacts: []execution.ArtifactReference{},
	}
	if err := ValidateAdapterResult(result); !errors.Is(err, execution.ErrInvalid) {
		t.Fatalf("sub-microsecond timestamp error = %v", err)
	}
}
