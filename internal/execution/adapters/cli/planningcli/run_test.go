package planningcli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stanimirivanov/argus/internal/contracts"
)

func TestRunValidatesArguments(t *testing.T) {
	t.Parallel()
	if err := Run(t.Context(), nil, nil, &bytes.Buffer{}); err == nil ||
		!strings.Contains(err.Error(), "usage: plan-functional-api") {
		t.Fatalf("missing arguments error = %v", err)
	}
	if err := Run(
		t.Context(), []string{"-manifest", "-", "-bindings", "-"},
		strings.NewReader("{}"), &bytes.Buffer{},
	); err == nil || !strings.Contains(err.Error(), "usage: plan-functional-api") {
		t.Fatalf("two stdin documents error = %v", err)
	}
}

func TestRunProducesRunnableSelectedAndControlJobs(t *testing.T) {
	manifestData, err := os.ReadFile(filepath.Join(
		"..", "..", "..", "..", "..", "contracts", "fixtures", "execution-manifest", "v1", "valid", "targeted.json",
	))
	if err != nil {
		t.Fatalf("read execution manifest fixture: %v", err)
	}
	manifestDocument, err := contracts.DecodeExecutionManifestV1(manifestData)
	if err != nil {
		t.Fatalf("decode execution manifest fixture: %v", err)
	}
	bindings := contracts.FunctionalAPIExecutionBindingsV1{
		APIVersion: contracts.FunctionalAPIExecutionBindingsV1APIVersion,
		Groups: []contracts.FunctionalAPIExecutionGroupBinding{{
			GroupKey: "orders-playwright", TestRepository: manifestDocument.Decisions[0].TestRepository,
			TestRevision: contracts.RevisionReference{
				Algorithm: "git-sha1", Digest: strings.Repeat("b", 40),
			},
			Adapter: "playwright",
		}},
	}
	bindingsData, err := json.Marshal(bindings)
	if err != nil {
		t.Fatalf("encode execution bindings: %v", err)
	}
	temporary := t.TempDir()
	manifestPath := filepath.Join(temporary, "manifest.json")
	bindingsPath := filepath.Join(temporary, "bindings.json")
	if err := os.WriteFile(manifestPath, manifestData, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(bindingsPath, bindingsData, 0o600); err != nil {
		t.Fatalf("write bindings: %v", err)
	}

	var output bytes.Buffer
	if err := Run(
		t.Context(), []string{"-manifest", manifestPath, "-bindings", bindingsPath}, nil, &output,
	); err != nil {
		t.Fatalf("run planner: %v", err)
	}
	var plan contracts.FunctionalAPIExecutionPlanV1
	if err := json.Unmarshal(output.Bytes(), &plan); err != nil {
		t.Fatalf("decode execution plan: %v", err)
	}
	if err := contracts.ValidateFunctionalAPIExecutionPlanV1(plan); err != nil {
		t.Fatalf("validate execution plan: %v", err)
	}
	if len(plan.Jobs) != 2 || plan.Jobs[0].Stage != "selected" ||
		plan.Jobs[1].Stage != "full-suite" {
		t.Fatalf("execution jobs = %+v", plan.Jobs)
	}
}
