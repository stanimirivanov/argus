package processadapter

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/contracts"
	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/execution"
)

const helperEnvironment = "ARGUS_PROCESS_ADAPTER_TEST_HELPER"

func TestAdapterExchangesValidatedProtocolDocuments(t *testing.T) {
	t.Setenv(helperEnvironment, "1")
	adapter, err := New([]string{os.Args[0], "-test.run=^TestProcessAdapterHelper$"}, io.Discard)
	if err != nil {
		t.Fatalf("create process adapter: %v", err)
	}
	request := execution.Request{
		APIVersion: execution.FunctionalAPIAdapterRequestAPIVersion,
		AttemptID:  "attempt-42",
		Manifest: execution.ManifestReference{
			APIVersion: "argus.dev/execution-manifest/v1",
			SHA256:     "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		},
		Stage: execution.StageSelected,
		TestRepository: catalog.Repository{
			Identity: catalog.RepositoryIdentity{
				Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "tests-1",
			},
			Owner: "example", Name: "orders-tests",
		},
		TestRevision: catalog.Revision{
			Algorithm: catalog.RevisionGitSHA1, Digest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
		Adapter: "playwright",
		Tests:   []execution.Test{{SuiteKey: "orders-api", TestKey: "create-order", Name: "Create order"}},
	}
	result, err := adapter.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("execute adapter: %v", err)
	}
	if result.AttemptID != request.AttemptID || len(result.Results) != 1 ||
		result.Results[0].Outcome != execution.TestPassed {
		t.Fatalf("adapter result = %+v", result)
	}
}

func TestProcessAdapterHelper(t *testing.T) {
	if os.Getenv(helperEnvironment) != "1" {
		return
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(2)
	}
	var request contracts.FunctionalAPIAdapterRequestV1
	if json.Unmarshal(data, &request) != nil ||
		contracts.ValidateFunctionalAPIAdapterRequestV1(request) != nil {
		os.Exit(3)
	}
	results := make([]contracts.NormalizedFunctionalAPITestResult, 0, len(request.Tests))
	for _, test := range request.Tests {
		results = append(results, contracts.NormalizedFunctionalAPITestResult{
			SuiteKey: test.SuiteKey, TestKey: test.TestKey,
			Outcome: "passed", DurationMS: 25,
		})
	}
	document := contracts.FunctionalAPIAdapterResultV1{
		APIVersion:  contracts.FunctionalAPIAdapterResultV1APIVersion,
		AttemptID:   request.AttemptID,
		Adapter:     contracts.AdapterIdentity{ID: request.Adapter, Version: "test-v1"},
		StartedAt:   time.Date(2026, 9, 19, 10, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
		CompletedAt: time.Date(2026, 9, 19, 10, 0, 1, 0, time.UTC).Format(time.RFC3339Nano),
		Results:     results, Artifacts: []contracts.ExecutionArtifactReference{},
	}
	if json.NewEncoder(os.Stdout).Encode(document) != nil {
		os.Exit(4)
	}
	os.Exit(0)
}
