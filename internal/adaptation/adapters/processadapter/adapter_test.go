package processadapter

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/adaptation"
	"github.com/stanimirivanov/argus/internal/adaptation/endpointrepair"
	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
	"github.com/stanimirivanov/argus/internal/contracts"
)

const helperEnvironment = "ARGUS_ADAPTATION_PROCESS_TEST_HELPER"

func TestAdapterExchangesValidatedProposalDocuments(t *testing.T) {
	t.Setenv(helperEnvironment, "1")
	adapter, err := New([]string{os.Args[0], "-test.run=^TestAdaptationProcessHelper$"}, io.Discard)
	if err != nil {
		t.Fatalf("create process adapter: %v", err)
	}
	request := validRequest()
	result, err := adapter.Propose(context.Background(), request)
	if err != nil {
		t.Fatalf("request proposal: %v", err)
	}
	if result.ProposalID != request.ProposalID || result.Edit == nil ||
		result.Edit.Original != request.Rename.PreviousPath || result.Edit.Replacement != request.Rename.Path {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestAdaptationProcessHelper(t *testing.T) {
	if os.Getenv(helperEnvironment) != "1" {
		return
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(2)
	}
	var request contracts.FunctionalAPIAdaptationRequestV1
	if json.Unmarshal(data, &request) != nil || contracts.ValidateFunctionalAPIAdaptationRequestV1(request) != nil {
		os.Exit(3)
	}
	document := contracts.FunctionalAPIAdaptationResultV1{
		APIVersion: contracts.FunctionalAPIAdaptationResultV1APIVersion,
		ProposalID: request.ProposalID,
		Adapter:    contracts.AdapterIdentity{ID: request.Test.Adapter, Version: "test-v1"},
		Outcome:    "candidate",
		Edit: &contracts.AdaptationTextEdit{
			Path:         "tests/orders.spec.ts",
			BeforeSHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			StartByte:    10, EndByte: 20, Original: request.EndpointRename.PreviousPath,
			Replacement: request.EndpointRename.Path, SemanticRole: "request-target",
		},
	}
	if json.NewEncoder(os.Stdout).Encode(document) != nil {
		os.Exit(4)
	}
	os.Exit(0)
}

func validRequest() endpointrepair.AdapterRequest {
	return endpointrepair.AdapterRequest{
		APIVersion: endpointrepair.RequestAPIVersion,
		ProposalID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Change: change.Reference{
			SourceRepository: catalog.Repository{
				Identity: catalog.RepositoryIdentity{
					Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "source-1",
				},
				Owner: "example", Name: "orders",
			},
			PullRequestNumber: 42,
			BaseRevision: catalog.Revision{
				Algorithm: catalog.RevisionGitSHA1, Digest: "1111111111111111111111111111111111111111",
			},
			HeadRevision: catalog.Revision{
				Algorithm: catalog.RevisionGitSHA1, Digest: "2222222222222222222222222222222222222222",
			},
			ObservedAt: time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC),
			Trigger: change.Trigger{
				Provider: catalog.ProviderGitHub, DeliveryID: "delivery-42",
				Event: "pull_request", Action: "synchronize",
			},
		},
		Test: adaptation.TestReference{
			Repository: catalog.Repository{
				Identity: catalog.RepositoryIdentity{
					Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "tests-1",
				},
				Owner: "example", Name: "orders-tests",
			},
			Revision: catalog.Revision{
				Algorithm: catalog.RevisionGitSHA1, Digest: "cccccccccccccccccccccccccccccccccccccccc",
			},
			SuiteKey: "orders-api", TestKey: "list-orders", Name: "Lists orders",
			Adapter: "playwright", Capabilities: []string{"list-orders"},
		},
		Rename: endpointrepair.EndpointRename{
			Method: "GET", OperationID: "listOrders", PreviousPath: "/v1/orders",
			Path: "/v2/orders", Capabilities: []string{"list-orders"},
		},
	}
}
