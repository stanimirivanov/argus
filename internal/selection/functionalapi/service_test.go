package functionalapi

import (
	"context"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
	"github.com/stanimirivanov/argus/internal/selection"
)

func TestSelectTargetsMappedTestsAndExplainsOmissions(t *testing.T) {
	t.Parallel()
	impact := validImpact(change.ImpactComplete, []change.OperationImpact{
		{Method: "POST", Path: "/orders", Kind: change.SemanticModified, Capabilities: []string{"create-order"}},
		{Method: "DELETE", Path: "/orders/{id}", Kind: change.SemanticAdded, Capabilities: []string{"cancel-order"}},
	})
	service := NewService(
		&stubImpactReader{impact: impact},
		&stubCatalogReader{catalog: candidateCatalog(impact.Change, []selection.TestReference{
			testCandidate("create-order-test", "create-order"),
			testCandidate("list-orders-test", "list-orders"),
		})},
	)

	manifest, err := service.Select(t.Context(), validRequest())
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if manifest.Mode != selection.ModeTargeted || len(manifest.Decisions) != 2 {
		t.Fatalf("mode/decisions = %s/%d", manifest.Mode, len(manifest.Decisions))
	}
	if manifest.Decisions[0].Outcome != selection.OutcomeRunRequired ||
		manifest.Decisions[0].Reasons[0].Code != selection.ReasonDirectCapabilityImpact {
		t.Fatalf("impacted decision = %+v", manifest.Decisions[0])
	}
	if manifest.Decisions[1].Outcome != selection.OutcomeSkipForNow ||
		manifest.Decisions[1].RemainingExecution != selection.RemainingFullSuite {
		t.Fatalf("omitted decision = %+v", manifest.Decisions[1])
	}
	if len(manifest.UncoveredCapabilities) != 1 || manifest.UncoveredCapabilities[0] != "cancel-order" {
		t.Fatalf("uncovered capabilities = %v", manifest.UncoveredCapabilities)
	}
}

func TestSelectFallsBackToEveryCandidateForUnmappedImpact(t *testing.T) {
	t.Parallel()
	impact := validImpact(change.ImpactComplete, []change.OperationImpact{
		{Method: "POST", Path: "/orders", Kind: change.SemanticModified},
	})
	service := NewService(
		&stubImpactReader{impact: impact},
		&stubCatalogReader{catalog: candidateCatalog(impact.Change, []selection.TestReference{
			testCandidate("create-order-test", "create-order"),
			testCandidate("list-orders-test", "list-orders"),
		})},
	)

	manifest, err := service.Select(t.Context(), validRequest())
	if err != nil {
		t.Fatalf("select fallback: %v", err)
	}
	if manifest.Mode != selection.ModeFallback {
		t.Fatalf("mode = %s, want fallback", manifest.Mode)
	}
	for _, decision := range manifest.Decisions {
		if decision.Outcome != selection.OutcomeRunRequired ||
			decision.Reasons[0].Code != selection.ReasonConservativeFallback {
			t.Fatalf("fallback decision = %+v", decision)
		}
	}
}

type stubImpactReader struct {
	impact change.CapabilityImpact
}

func (reader *stubImpactReader) FindCapabilityImpact(
	context.Context,
	catalog.Provider,
	string,
) (change.CapabilityImpact, error) {
	return reader.impact, nil
}

type stubCatalogReader struct {
	catalog Catalog
}

func (reader *stubCatalogReader) ReadFunctionalAPICatalog(
	context.Context,
	catalog.SnapshotKey,
) (Catalog, error) {
	return reader.catalog, nil
}

func validRequest() Request {
	return Request{
		Provider: catalog.ProviderGitHub, DeliveryID: "delivery-42",
		DescriptorAPIVersion: "argus.dev/repository-descriptor/v1",
	}
}

func validImpact(status change.ImpactStatus, operations []change.OperationImpact) change.CapabilityImpact {
	repository := catalog.Repository{
		Identity: catalog.RepositoryIdentity{
			Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "source-42",
		},
		Owner: "example", Name: "orders",
	}

	return change.CapabilityImpact{
		APIVersion: change.ImpactAPIVersion, AnalyzerVersion: change.OpenAPIAnalyzerVersion,
		Change: change.Reference{
			SourceRepository: repository, PullRequestNumber: 42,
			BaseRevision: catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: "0123456789abcdef0123456789abcdef01234567"},
			HeadRevision: catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: "89abcdef0123456789abcdef0123456789abcdef"},
			ObservedAt:   time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC),
			Trigger: change.Trigger{
				Provider: catalog.ProviderGitHub, DeliveryID: "delivery-42",
				Event: "pull_request", Action: "synchronize",
			},
		},
		Status: status,
		Documents: []change.DocumentImpact{{
			Path: "api/openapi.yaml", Kind: change.SemanticModified,
			TotalChanges: max(1, len(operations)), Operations: operations,
		}},
	}
}

func candidateCatalog(reference change.Reference, tests []selection.TestReference) Catalog {
	return Catalog{
		Snapshot: catalog.SnapshotReference{
			SourceRepository: reference.SourceRepository, Revision: reference.BaseRevision,
			DescriptorAPIVersion: "argus.dev/repository-descriptor/v1",
		},
		Tests: tests,
	}
}

func testCandidate(key, capability string) selection.TestReference {
	return selection.TestReference{
		Repository: catalog.Repository{
			Identity: catalog.RepositoryIdentity{
				Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "tests-1",
			},
			Owner: "example", Name: "orders-tests",
		},
		SuiteKey: "orders-api", Family: catalog.TestFamilyFunctionalAPI, Adapter: "playwright",
		TestKey: key, Name: key, Capabilities: []string{capability},
	}
}
