package fieldrepair

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/adaptation"
	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
)

type fakeAdapter struct {
	request Request
	result  Result
}

func (adapter *fakeAdapter) ProposeField(_ context.Context, request Request) (Result, error) {
	adapter.request = request
	adapter.result.ProposalID = request.ProposalID

	return adapter.result, nil
}

func TestProposeFieldRetainsCompatibleProofAndRejectsOracleEdit(t *testing.T) {
	t.Parallel()
	rename := validRename()
	adapter := &fakeAdapter{result: Result{
		APIVersion: ResultAPIVersion, AdapterID: "playwright", AdapterVersion: "1.58.2",
		Outcome: "candidate", Edit: &adaptation.TextEdit{
			Path: "tests/orders.spec.mjs", BeforeSHA256: strings.Repeat("a", 64),
			StartByte: 40, EndByte: 50, Original: "customerId", Replacement: "clientId",
			SemanticRole: "request-body-key",
		},
	}}
	proposal, err := NewService(adapter).Propose(context.Background(), validImpact(), validTest(), rename)
	if err != nil {
		t.Fatalf("propose field: %v", err)
	}
	if proposal.ProposalID == "" || proposal.ProposalID != adapter.request.ProposalID ||
		proposal.Rename.BaseSHA256 != rename.BaseSHA256 || proposal.Decision != "PATCH_AND_VALIDATE" {
		t.Fatalf("unbound proposal: %+v", proposal)
	}
	adapter.result.Edit.Original = "404"
	adapter.result.Edit.EndByte = 43
	if _, err := NewService(adapter).Propose(context.Background(), validImpact(), validTest(), rename); !errors.Is(err, adaptation.ErrInvalid) {
		t.Fatalf("expected oracle edit rejection, got %v", err)
	}
	adapter.result.Edit.Original = "customerId"
	adapter.result.Edit.EndByte = 50
	adapter.result.Edit.SemanticRole = "response-field-access"
	if _, err := NewService(adapter).Propose(context.Background(), validImpact(), validTest(), rename); !errors.Is(err, adaptation.ErrInvalid) {
		t.Fatalf("expected direction rejection, got %v", err)
	}
}

func TestProposeFieldAbstainsForIncompleteOrUnmappedImpact(t *testing.T) {
	t.Parallel()
	rename := validRename()
	adapter := &fakeAdapter{result: Result{
		APIVersion: ResultAPIVersion, AdapterID: "playwright", AdapterVersion: "1.58.2",
		Outcome: "abstained", ReasonCode: "unsupported-source", Reason: "dynamic body",
	}}
	if _, err := NewService(adapter).Propose(context.Background(), validImpact(), validTest(), rename); !errors.Is(err, ErrAdapterAbstained) {
		t.Fatalf("expected adapter abstention, got %v", err)
	}
	impact := validImpact()
	impact.Status = change.ImpactPartial
	impact.Warnings = []string{"incomplete"}
	if _, err := NewService(adapter).Propose(context.Background(), impact, validTest(), rename); !errors.Is(err, ErrNoAuthoritativeRename) {
		t.Fatalf("expected incomplete impact abstention, got %v", err)
	}
	rename.OperationID = "unrelated"
	if _, err := NewService(adapter).Propose(context.Background(), validImpact(), validTest(), rename); !errors.Is(err, ErrNoAuthoritativeRename) {
		t.Fatalf("expected operation mismatch abstention, got %v", err)
	}
}

func validRename() change.FieldRename {
	return change.FieldRename{
		DocumentPath: "api/orders.yaml", BaseSHA256: strings.Repeat("1", 64),
		HeadSHA256: strings.Repeat("2", 64), Method: "POST", Path: "/orders",
		OperationID: "createOrder", Capabilities: []string{"create-order"},
		Direction: "request", PreviousName: "customerId", Name: "clientId",
	}
}

func validTest() adaptation.TestReference {
	return adaptation.TestReference{
		Repository: catalog.Repository{Identity: catalog.RepositoryIdentity{
			Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "R_tests_1",
		}, Owner: "example", Name: "orders-tests"},
		Revision: catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: strings.Repeat("c", 40)},
		SuiteKey: "orders-api", TestKey: "create-order", Name: "creates order",
		Adapter: "playwright", Capabilities: []string{"create-order"},
	}
}

func validImpact() change.CapabilityImpact {
	id := "createOrder"

	return change.CapabilityImpact{
		APIVersion: change.ImpactAPIVersion, AnalyzerVersion: change.OpenAPIAnalyzerVersion,
		Change: change.Reference{
			SourceRepository: catalog.Repository{Identity: catalog.RepositoryIdentity{
				Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "R_source_1",
			}, Owner: "example", Name: "orders"},
			PullRequestNumber: 42,
			BaseRevision:      catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: strings.Repeat("1", 40)},
			HeadRevision:      catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: strings.Repeat("2", 40)},
			ObservedAt:        time.Date(2026, 10, 17, 10, 0, 0, 0, time.UTC),
			Trigger: change.Trigger{Provider: catalog.ProviderGitHub, DeliveryID: "delivery-42",
				Event: "pull_request", Action: "synchronize"},
		},
		Status: change.ImpactComplete,
		Documents: []change.DocumentImpact{{
			Path: "api/orders.yaml", Kind: change.SemanticModified, TotalChanges: 1, BreakingChanges: 1,
			Operations: []change.OperationImpact{{Method: "POST", Path: "/orders", OperationID: &id,
				Kind: change.SemanticModified, Capabilities: []string{"create-order"}, PotentialBreak: true}},
		}},
	}
}
