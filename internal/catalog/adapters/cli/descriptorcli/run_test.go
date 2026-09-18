package descriptorcli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
)

const testRevision = "0123456789abcdef0123456789abcdef01234567"

func TestRunReportsNormalizedDescriptorSummary(t *testing.T) {
	t.Parallel()

	fixture := filepath.Join(
		"..", "..", "..", "..", "..", "contracts", "fixtures", "repository-descriptor", "v1", "valid", "source-and-test-repositories.json",
	)
	var output bytes.Buffer
	if err := Run([]string{"-revision", testRevision, fixture}, &output); err != nil {
		t.Fatalf("run descriptor command: %v", err)
	}

	var result summary
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode command output: %v", err)
	}
	if result.CapabilityCount != 2 || result.ComponentCount != 1 || result.TestSuiteCount != 1 || result.TestCount != 1 {
		t.Fatalf("unexpected summary: %#v", result)
	}
	if result.SourceRepository.ProviderRepositoryID != "R_orders_source_01" {
		t.Fatalf("source repository id = %q", result.SourceRepository.ProviderRepositoryID)
	}

	var document map[string]any
	if err := json.Unmarshal(output.Bytes(), &document); err != nil {
		t.Fatalf("decode command output keys: %v", err)
	}
	for _, key := range []string{
		"apiVersion",
		"sourceRepository",
		"revision",
		"capabilityCount",
		"componentCount",
		"testSuiteCount",
		"testCount",
	} {
		if _, ok := document[key]; !ok {
			t.Fatalf("command output is missing JSON key %q", key)
		}
	}
	repository, ok := document["sourceRepository"].(map[string]any)
	if !ok {
		t.Fatal("sourceRepository is not a JSON object")
	}
	if _, ok := repository["providerRepositoryId"]; !ok {
		t.Fatal("sourceRepository is missing JSON key \"providerRepositoryId\"")
	}
}

func TestRunRejectsDomainInvalidDescriptor(t *testing.T) {
	t.Parallel()

	fixture := filepath.Join(
		"..", "..", "..", "..", "..", "contracts", "fixtures", "repository-descriptor", "v1", "invalid", "unknown-capability-reference.json",
	)
	var output bytes.Buffer
	err := Run([]string{"-revision", testRevision, fixture}, &output)
	if err == nil {
		t.Fatal("expected domain-invalid descriptor to fail")
	}
	if output.Len() != 0 {
		t.Fatalf("failed command wrote output: %q", output.String())
	}
}

func TestRunRequiresRevisionAndPath(t *testing.T) {
	t.Parallel()

	if err := Run(nil, &bytes.Buffer{}); err == nil {
		t.Fatal("expected missing arguments to fail")
	}
}
