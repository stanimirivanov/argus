package validationprocessadapter

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/contracts"
	"github.com/stanimirivanov/argus/internal/adaptation"
	"github.com/stanimirivanov/argus/internal/catalog"
)

const helperEnvironment = "ARGUS_VALIDATION_PROCESS_TEST_HELPER"

func TestAdapterExecutesValidatedPhaseInWorkspace(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "workspace.marker"), []byte("expected"), 0o600); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	t.Setenv(helperEnvironment, "1")
	adapter, err := New([]string{os.Args[0], "-test.run=^TestValidationProcessHelper$"}, root, io.Discard)
	if err != nil {
		t.Fatalf("create validation adapter: %v", err)
	}
	request := validRequest()
	result, err := adapter.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("execute validation adapter: %v", err)
	}
	if result.Phase != adaptation.ValidationOriginal || result.Outcome != adaptation.ValidationFailed {
		t.Fatalf("unexpected validation result: %+v", result)
	}
}

func TestValidationProcessHelper(t *testing.T) {
	if os.Getenv(helperEnvironment) != "1" {
		return
	}
	if _, err := os.Stat("workspace.marker"); err != nil {
		os.Exit(2)
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(3)
	}
	var request contracts.FunctionalAPIRepairValidationRequestV1
	if json.Unmarshal(data, &request) != nil ||
		contracts.ValidateFunctionalAPIRepairValidationRequestV1(request) != nil {
		os.Exit(4)
	}
	document := contracts.FunctionalAPIRepairValidationResultV1{
		APIVersion:   contracts.FunctionalAPIRepairValidationResultV1APIVersion,
		ValidationID: request.ValidationID, ProposalID: request.ProposalID, Phase: request.Phase,
		SuiteKey: request.Test.SuiteKey, TestKey: request.Test.TestKey,
		Adapter:      contracts.AdapterIdentity{ID: request.Test.Adapter, Version: "test-v1"},
		SourceSHA256: request.SourceSHA256,
		StartedAt:    time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
		CompletedAt:  time.Date(2026, 9, 27, 10, 0, 1, 0, time.UTC).Format(time.RFC3339Nano),
		Outcome:      "failed",
		Failure:      &contracts.NormalizedFailure{Code: "request-failed", Message: "request failed"},
	}
	if json.NewEncoder(os.Stdout).Encode(document) != nil {
		os.Exit(5)
	}
	os.Exit(0)
}

func validRequest() adaptation.ValidationRequest {
	return adaptation.ValidationRequest{
		APIVersion:   adaptation.ValidationRequestAPIVersion,
		ValidationID: "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
		ProposalID:   "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Phase:        adaptation.ValidationOriginal,
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
		SourceSHA256: "1111111111111111111111111111111111111111111111111111111111111111",
	}
}
