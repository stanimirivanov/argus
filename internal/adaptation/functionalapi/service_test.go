package functionalapi

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/adaptation"
	"github.com/stanimirivanov/argus/internal/adaptation/endpointrepair"
	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
)

func TestProposeProducesConstrainedEndpointEdit(t *testing.T) {
	t.Parallel()

	adapter := &fakeAdapter{result: endpointrepair.AdapterResult{
		APIVersion: endpointrepair.ResultAPIVersion, AdapterID: "playwright", AdapterVersion: "1.2.3",
		Outcome: "candidate", Edit: &adaptation.TextEdit{
			Path: "tests/orders.spec.ts", BeforeSHA256: repeated("a", 64), StartByte: 120, EndByte: 130,
			Original: "/v1/orders", Replacement: "/v2/orders", SemanticRole: "request-target",
		},
	}}
	service := NewService(adapter)
	proposal, err := service.Propose(context.Background(), validImpact(), validTest())
	if err != nil {
		t.Fatalf("propose repair: %v", err)
	}
	if adapter.request.ProposalID == "" || proposal.ProposalID != adapter.request.ProposalID {
		t.Fatal("proposal identity was not correlated")
	}
	if proposal.Classification != endpointrepair.ClassificationInvalidated ||
		proposal.Decision != endpointrepair.DecisionPatchAndValidate {
		t.Fatalf("unexpected proposal decision: %+v", proposal)
	}
	if proposal.Rename.OperationID != "listOrders" || proposal.Edit.Original != "/v1/orders" ||
		proposal.Edit.Replacement != "/v2/orders" {
		t.Fatalf("unexpected constrained edit: %+v", proposal)
	}
}

func TestDetectEndpointRenameAbstainsOnIncompleteOrAmbiguousEvidence(t *testing.T) {
	t.Parallel()

	tests := map[string]func(change.CapabilityImpact) change.CapabilityImpact{
		"partial impact": func(impact change.CapabilityImpact) change.CapabilityImpact {
			impact.Status = change.ImpactPartial
			impact.Warnings = []string{"source document unavailable"}

			return impact
		},
		"ambiguous target": func(impact change.CapabilityImpact) change.CapabilityImpact {
			id := "listOrders"
			impact.Documents[0].Operations = append(impact.Documents[0].Operations, change.OperationImpact{
				Method: "GET", Path: "/v3/orders", OperationID: &id, Kind: change.SemanticAdded,
				Capabilities: []string{"list-orders"},
			})

			return impact
		},
		"changed operation identity": func(impact change.CapabilityImpact) change.CapabilityImpact {
			id := "searchOrders"
			impact.Documents[0].Operations[1].OperationID = &id

			return impact
		},
	}
	for name, mutate := range tests {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := DetectEndpointRename(mutate(validImpact()), []string{"list-orders"})
			if !errors.Is(err, endpointrepair.ErrNoAuthoritativeRename) {
				t.Fatalf("expected abstention, got %v", err)
			}
		})
	}
}

func TestProposeRejectsIntentChangingAdapterEdit(t *testing.T) {
	t.Parallel()

	adapter := &fakeAdapter{result: endpointrepair.AdapterResult{
		APIVersion: endpointrepair.ResultAPIVersion, AdapterID: "playwright", AdapterVersion: "1.2.3",
		Outcome: "candidate", Edit: &adaptation.TextEdit{
			Path: "tests/orders.spec.ts", BeforeSHA256: repeated("b", 64), StartByte: 20, EndByte: 23,
			Original: "404", Replacement: "200", SemanticRole: "request-target",
		},
	}}
	_, err := NewService(adapter).Propose(context.Background(), validImpact(), validTest())
	if !errors.Is(err, adaptation.ErrInvalid) {
		t.Fatalf("expected policy rejection, got %v", err)
	}
}

func TestProposePreservesAdapterAbstention(t *testing.T) {
	t.Parallel()

	adapter := &fakeAdapter{result: endpointrepair.AdapterResult{
		APIVersion: endpointrepair.ResultAPIVersion, AdapterID: "playwright", AdapterVersion: "1.2.3",
		Outcome: "abstained", ReasonCode: "ambiguous-reference", Reason: "two request targets matched",
	}}
	_, err := NewService(adapter).Propose(context.Background(), validImpact(), validTest())
	if !errors.Is(err, endpointrepair.ErrAdapterAbstained) {
		t.Fatalf("expected abstention, got %v", err)
	}
}

type fakeAdapter struct {
	request endpointrepair.AdapterRequest
	result  endpointrepair.AdapterResult
}

func (adapter *fakeAdapter) Propose(
	_ context.Context,
	request endpointrepair.AdapterRequest,
) (endpointrepair.AdapterResult, error) {
	adapter.request = request
	adapter.result.ProposalID = request.ProposalID
	return adapter.result, nil
}

func validTest() adaptation.TestReference {
	return adaptation.TestReference{
		Repository: catalog.Repository{
			Identity: catalog.RepositoryIdentity{
				Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "R_tests_1",
			},
			Owner: "example", Name: "orders-tests",
		},
		Revision: catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: repeated("c", 40)},
		SuiteKey: "orders-api", TestKey: "list-orders", Name: "lists orders",
		Adapter: "playwright", Capabilities: []string{"list-orders"},
	}
}

func validImpact() change.CapabilityImpact {
	id := "listOrders"

	return change.CapabilityImpact{
		APIVersion: change.ImpactAPIVersion, AnalyzerVersion: change.OpenAPIAnalyzerVersion,
		Change: change.Reference{
			SourceRepository: catalog.Repository{
				Identity: catalog.RepositoryIdentity{
					Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "R_source_1",
				},
				Owner: "example", Name: "orders",
			},
			PullRequestNumber: 42,
			BaseRevision:      catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: repeated("1", 40)},
			HeadRevision:      catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: repeated("2", 40)},
			ObservedAt:        time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC),
			Trigger: change.Trigger{
				Provider: catalog.ProviderGitHub, DeliveryID: "delivery-42",
				Event: "pull_request", Action: "synchronize",
			},
		},
		Status: change.ImpactComplete,
		Documents: []change.DocumentImpact{{
			Path: "api/openapi.yaml", Kind: change.SemanticModified,
			TotalChanges: 2, BreakingChanges: 1,
			Operations: []change.OperationImpact{
				{Method: "GET", Path: "/v1/orders", OperationID: &id, Kind: change.SemanticRemoved,
					Capabilities: []string{"list-orders"}, PotentialBreak: true},
				{Method: "GET", Path: "/v2/orders", OperationID: &id, Kind: change.SemanticAdded,
					Capabilities: []string{"list-orders"}},
			},
		}},
	}
}

func repeated(value string, count int) string {
	result := ""
	for range count {
		result += value
	}

	return result
}
