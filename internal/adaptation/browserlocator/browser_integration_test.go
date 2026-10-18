//go:build browserintegration

package browserlocator_test

import (
	"bytes"
	"encoding/json"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stanimirivanov/argus/internal/adaptation/adapters/cli/browserlocatorcli"
	adaptationcontract "github.com/stanimirivanov/argus/internal/adaptation/adapters/contract"
	"github.com/stanimirivanov/argus/internal/adaptation/browserlocator"
	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/execution"
	executioncontract "github.com/stanimirivanov/argus/internal/execution/adapters/contract"
	"github.com/stanimirivanov/argus/internal/execution/adapters/processadapter"
	"github.com/stanimirivanov/argus/internal/execution/browser"
	"github.com/stanimirivanov/argus/internal/selection"
)

func TestRealPlaywrightLocatorRenameProposal(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	fixtureDir := filepath.Join(root, "examples", "functional-ui", "test-repository")
	t.Setenv("ARGUS_BROWSER_TEST_DIR", fixtureDir)
	artifactRoot := t.TempDir()
	t.Setenv("ARGUS_BROWSER_ARTIFACT_DIR", artifactRoot)
	t.Setenv("ARGUS_BROWSER_ENVIRONMENT", "ci")
	revisionBytes, err := exec.CommandContext(t.Context(), "git", "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	revision := catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: strings.TrimSpace(string(revisionBytes))}
	repo := catalog.Repository{Identity: catalog.RepositoryIdentity{Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "tests-1"}, Owner: "example", Name: "browser-tests"}
	manifest := browser.Manifest{APIVersion: browser.ManifestVersion, Decisions: []browser.Decision{{
		Repository: repo, SuiteKey: "shop", TestKey: "checkout", Name: "checkout shows total [argus:shop:checkout]",
		Revision: revision, Projects: []string{"chromium-desktop"}, Outcome: selection.OutcomeRunRequired,
	}}}
	var diagnostics bytes.Buffer
	adapter, err := processadapter.NewBrowser([]string{"node", filepath.Join(root, "examples", "functional-ui", "playwright-adapter.mjs")}, &diagnostics)
	if err != nil {
		t.Fatal(err)
	}
	run := func(id string) browser.Attempt {
		attempt, runErr := browser.NewService(adapter).Execute(t.Context(), manifest, browser.Options{
			AttemptID: id, ManifestSHA256: strings.Repeat("a", 64), Stage: execution.StageSelected,
			TestRepository: repo.Identity, TestRevision: revision, Adapter: "playwright",
			Project: "chromium-desktop", Environment: "ci",
		})
		if runErr != nil {
			t.Fatalf("execute %s: %v; stderr: %s", id, runErr, diagnostics.String())
		}
		return attempt
	}
	baseline := run("locator-baseline")
	t.Setenv("ARGUS_BROWSER_RENAME_CHECKOUT", "1")
	failed := run("locator-failed")
	if failed.Result.Results[0].Failure == nil || failed.Result.Results[0].Failure.Code != "locator-not-found" {
		t.Fatalf("unexpected failure classification: %+v", failed.Result.Results[0])
	}
	source, err := os.ReadFile(filepath.Join(fixtureDir, "tests", "shop.spec.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	request := browserlocator.Request{Baseline: baseline, Failed: failed, SuiteKey: "shop", TestKey: "checkout",
		SourcePath: "tests/shop.spec.mjs", Source: source,
		BaselineData: snapshotBytes(t, baseline), FailedData: snapshotBytes(t, failed)}
	diagnosis, err := browserlocator.Diagnose(request)
	if err != nil {
		t.Fatal(err)
	}
	if diagnosis.ReasonCode != "candidate" || diagnosis.Proposal == nil ||
		diagnosis.Proposal.Edit.Replacement != `getByRole("button", { name: "Place order" })` {
		t.Fatalf("real browser diagnosis: %+v", diagnosis)
	}
	writeAttempt := func(name string, attempt browser.Attempt) string {
		document, exportErr := executioncontract.ExportBrowserAttempt(attempt)
		if exportErr != nil {
			t.Fatal(exportErr)
		}
		data, marshalErr := json.Marshal(document)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		path := filepath.Join(t.TempDir(), name+".json")
		if writeErr := os.WriteFile(path, data, 0600); writeErr != nil {
			t.Fatal(writeErr)
		}
		return path
	}
	var output bytes.Buffer
	err = browserlocatorcli.Run(t.Context(), []string{
		"-baseline", writeAttempt("baseline", baseline), "-failed", writeAttempt("failed", failed),
		"-test-checkout", root, "-artifact-root", artifactRoot,
		"-registry", "examples/functional-ui/test-repository/argus-tests.json",
		"-source-path", "examples/functional-ui/test-repository/tests/shop.spec.mjs",
		"-suite-key", "shop", "-test-key", "checkout",
	}, &output, func(data []byte) (browser.Attempt, error) {
		document, decodeErr := contracts.DecodeBrowserExecutionAttemptV1(data)
		if decodeErr != nil {
			return browser.Attempt{}, decodeErr
		}
		return executioncontract.ImportBrowserAttempt(document)
	}, func(value browserlocator.Diagnosis) ([]byte, error) {
		document, exportErr := adaptationcontract.ExportBrowserLocatorDiagnosisV1(value)
		if exportErr != nil {
			return nil, exportErr
		}
		return json.Marshal(document)
	})
	if err != nil {
		t.Fatal(err)
	}
	var exported contracts.BrowserLocatorDiagnosisV1
	if err := json.Unmarshal(output.Bytes(), &exported); err != nil {
		t.Fatal(err)
	}
	if exported.Proposal == nil || exported.ReasonCode != "candidate" {
		t.Fatalf("CLI diagnosis: %+v", exported)
	}
}

func snapshotBytes(t *testing.T, attempt browser.Attempt) []byte {
	t.Helper()
	for _, artifact := range attempt.Result.Artifacts {
		if artifact.Kind != "accessibility" {
			continue
		}
		uri, err := url.Parse(artifact.URI)
		if err != nil {
			t.Fatal(err)
		}
		path := uri.Path
		if len(path) > 2 && path[0] == '/' && path[2] == ':' {
			path = path[1:]
		}
		data, err := os.ReadFile(filepath.FromSlash(path))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	t.Fatal("missing accessibility artifact")
	return nil
}
