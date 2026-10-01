package functionalapi

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/selection"
)

func TestExecuteCorrelatesSelectedTestsAndDerivesFailure(t *testing.T) {
	t.Parallel()
	adapter := &stubAdapter{respond: func(request Request) AdapterResult {
		if len(request.Tests) != 1 || request.Tests[0].TestKey != "create-order-test" {
			t.Fatalf("adapter request tests = %+v", request.Tests)
		}

		return validResult(request, execution.TestFailed)
	}}
	attempt, err := NewService(adapter).Execute(t.Context(), validManifest(), validOptions(execution.StageSelected))
	if err != nil {
		t.Fatalf("execute selected stage: %v", err)
	}
	if attempt.Outcome != execution.AttemptFailed || attempt.Manifest.SHA256 != digest() {
		t.Fatalf("attempt = %+v", attempt)
	}
}

func TestExecuteFullSuiteIncludesEarlyOmissions(t *testing.T) {
	t.Parallel()
	adapter := &stubAdapter{respond: func(request Request) AdapterResult {
		if len(request.Tests) != 2 {
			t.Fatalf("full-suite tests = %+v", request.Tests)
		}

		return validResult(request, execution.TestPassed)
	}}
	if _, err := NewService(adapter).Execute(
		t.Context(), validManifest(), validOptions(execution.StageFullSuite),
	); err != nil {
		t.Fatalf("execute full suite: %v", err)
	}
}

func TestExecuteRejectsAdapterTestSetExpansion(t *testing.T) {
	t.Parallel()
	adapter := &stubAdapter{respond: func(request Request) AdapterResult {
		result := validResult(request, execution.TestPassed)
		result.Results[0].TestKey = "unrequested-test"

		return result
	}}
	_, err := NewService(adapter).Execute(t.Context(), validManifest(), validOptions(execution.StageSelected))
	if !errors.Is(err, execution.ErrInvalid) {
		t.Fatalf("expanded result error = %v", err)
	}
}

type stubAdapter struct {
	respond func(Request) AdapterResult
}

func (adapter *stubAdapter) Execute(
	_ context.Context,
	request Request,
) (AdapterResult, error) {
	return adapter.respond(request), nil
}

func validResult(request Request, outcome execution.TestOutcome) AdapterResult {
	results := make([]execution.TestResult, 0, len(request.Tests))
	for _, test := range request.Tests {
		result := execution.TestResult{
			SuiteKey: test.SuiteKey, TestKey: test.TestKey, Outcome: outcome, Duration: time.Second,
		}
		if outcome == execution.TestFailed || outcome == execution.TestError {
			result.Failure = &execution.Failure{Code: "assertion-failed", Message: "expected 201"}
		}
		results = append(results, result)
	}

	return AdapterResult{
		APIVersion: AdapterResultAPIVersion,
		AttemptID:  request.AttemptID, AdapterID: request.Adapter, AdapterVersion: "1.2.3",
		StartedAt:   time.Date(2026, 9, 19, 10, 0, 0, 0, time.UTC),
		CompletedAt: time.Date(2026, 9, 19, 10, 0, 2, 0, time.UTC), Results: results,
	}
}

func validOptions(stage execution.Stage) Options {
	return Options{
		AttemptID: "github-123-1", ManifestSHA256: digest(), Stage: stage,
		TestRepository: testRepository().Identity,
		TestRevision: catalog.Revision{
			Algorithm: catalog.RevisionGitSHA1, Digest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
		Adapter: "playwright",
	}
}

func validManifest() selection.Manifest {
	source := catalog.Repository{
		Identity: catalog.RepositoryIdentity{
			Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "source-42",
		},
		Owner: "example", Name: "orders",
	}
	base := catalog.Revision{
		Algorithm: catalog.RevisionGitSHA1, Digest: "0123456789abcdef0123456789abcdef01234567",
	}

	return selection.Manifest{
		APIVersion: selection.ManifestAPIVersion, PolicyVersion: selection.FunctionalAPIPolicyVersion,
		ImpactAPIVersion: change.ImpactAPIVersion, ImpactAnalyzerVersion: change.OpenAPIAnalyzerVersion,
		Change: change.Reference{
			SourceRepository: source, PullRequestNumber: 42, BaseRevision: base,
			HeadRevision: catalog.Revision{
				Algorithm: catalog.RevisionGitSHA1, Digest: "89abcdef0123456789abcdef0123456789abcdef",
			},
			ObservedAt: time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC),
			Trigger: change.Trigger{
				Provider: catalog.ProviderGitHub, DeliveryID: "delivery-42",
				Event: "pull_request", Action: "synchronize",
			},
		},
		Catalog: catalog.SnapshotReference{
			SourceRepository: source, Revision: base,
			DescriptorAPIVersion: "argus.dev/repository-descriptor/v1",
		},
		Family: catalog.TestFamilyFunctionalAPI, Mode: selection.ModeTargeted,
		AffectedCapabilities: []string{"create-order"},
		Decisions: []selection.Decision{
			decision("create-order-test", selection.OutcomeRunRequired, selection.RemainingNone,
				selection.Reason{Code: selection.ReasonDirectCapabilityImpact, Capabilities: []string{"create-order"}}),
			decision("list-orders-test", selection.OutcomeSkipForNow, selection.RemainingFullSuite,
				selection.Reason{Code: selection.ReasonNotAffected}),
		},
	}
}

func decision(
	key string,
	outcome selection.Outcome,
	remaining selection.RemainingExecution,
	reason selection.Reason,
) selection.Decision {
	return selection.Decision{
		Test: selection.TestReference{
			Repository: testRepository(), SuiteKey: "orders-api",
			Family: catalog.TestFamilyFunctionalAPI, Adapter: "playwright",
			TestKey: key, Name: key, Capabilities: []string{"create-order"},
		},
		Outcome: outcome, RemainingExecution: remaining, Reasons: []selection.Reason{reason},
	}
}

func testRepository() catalog.Repository {
	return catalog.Repository{
		Identity: catalog.RepositoryIdentity{
			Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "tests-1",
		},
		Owner: "example", Name: "orders-tests",
	}
}

func digest() string { return "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" }
