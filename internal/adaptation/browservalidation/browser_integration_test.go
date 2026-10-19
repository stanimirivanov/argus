//go:build browserintegration

package browservalidation_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	adaptationcontract "github.com/stanimirivanov/argus/internal/adaptation/adapters/contract"
	"github.com/stanimirivanov/argus/internal/adaptation/adapters/fileworkspace"
	"github.com/stanimirivanov/argus/internal/adaptation/browserlocator"
	"github.com/stanimirivanov/argus/internal/adaptation/browservalidation"
	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/execution"
	executioncontract "github.com/stanimirivanov/argus/internal/execution/adapters/contract"
	"github.com/stanimirivanov/argus/internal/execution/adapters/processadapter"
	"github.com/stanimirivanov/argus/internal/execution/browser"
	"github.com/stanimirivanov/argus/internal/selection"
)

func TestRealPlaywrightBrowserLocatorValidation(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	fixture := disposableCheckout(t, root)
	artifactRoot := t.TempDir()
	t.Setenv("ARGUS_BROWSER_TEST_DIR", fixture)
	t.Setenv("ARGUS_BROWSER_ARTIFACT_DIR", artifactRoot)
	t.Setenv("ARGUS_BROWSER_ENVIRONMENT", "ci")
	revision := catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: gitOutput(t, fixture, "rev-parse", "HEAD")}
	repo := catalog.Repository{Identity: catalog.RepositoryIdentity{Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "tests-1"}, Owner: "example", Name: "browser-tests"}
	manifest := browser.Manifest{APIVersion: browser.ManifestVersion, Decisions: []browser.Decision{{Repository: repo,
		SuiteKey: "shop", TestKey: "checkout", Name: "checkout shows total [argus:shop:checkout]",
		Revision: revision, Projects: []string{"chromium-desktop"}, Outcome: selection.OutcomeRunRequired}}}
	var diagnostics bytes.Buffer
	adapter, err := processadapter.NewBrowser([]string{"node", filepath.Join(root, "examples", "functional-ui", "playwright-adapter.mjs")}, &diagnostics)
	if err != nil {
		t.Fatal(err)
	}
	run := func(id string) browser.Attempt {
		attempt, err := browser.NewService(adapter).Execute(t.Context(), manifest, browser.Options{AttemptID: id,
			ManifestSHA256: strings.Repeat("a", 64), Stage: execution.StageSelected,
			TestRepository: repo.Identity, TestRevision: revision, Adapter: "playwright",
			Project: "chromium-desktop", Environment: "ci"})
		if err != nil {
			t.Fatalf("run %s: %v; stderr: %s", id, err, diagnostics.String())
		}
		return attempt
	}
	baseline := run("validation-baseline")
	t.Setenv("ARGUS_BROWSER_RENAME_CHECKOUT", "1")
	failed := run("validation-failed")
	sourcePath := filepath.Join(fixture, "tests", "shop.spec.mjs")
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	diagnosis, err := browserlocator.Diagnose(browserlocator.Request{Baseline: baseline, Failed: failed,
		SuiteKey: "shop", TestKey: "checkout", SourcePath: "tests/shop.spec.mjs", Source: source,
		BaselineData: snapshotBytes(t, baseline), FailedData: snapshotBytes(t, failed)})
	if err != nil {
		t.Fatal(err)
	}
	if diagnosis.Proposal == nil {
		t.Fatalf("missing real browser proposal: %+v", diagnosis)
	}
	workspace, err := fileworkspace.Open(fixture)
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := browservalidation.NewService(workspace, adapter).Validate(t.Context(), *diagnosis.Proposal, baseline, failed)
	if err != nil {
		t.Fatalf("validate real browser repair: %v; stderr: %s", err, diagnostics.String())
	}
	if len(evidence.Runs) != 6 || evidence.Status != browservalidation.Validated {
		t.Fatalf("incomplete browser proof: %+v", evidence)
	}
	document, err := adaptationcontract.ExportBrowserLocatorValidationV1(evidence, executioncontract.ExportBrowserAttempt)
	if err != nil {
		t.Fatal(err)
	}
	if document.ValidationID != evidence.ValidationID {
		t.Fatal("public contract lost validation identity")
	}
	restored, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(source, restored) {
		t.Fatal("disposable browser source was not restored")
	}
	t.Setenv("ARGUS_BROWSER_FALSE_HEAL", "1")
	_, err = browservalidation.NewService(workspace, adapter).Validate(t.Context(), *diagnosis.Proposal, baseline, failed)
	var rejected *browservalidation.RejectedError
	if !errors.As(err, &rejected) || rejected.Evidence.Reason != "negative-control-passed" {
		t.Fatalf("false healing was not rejected: %v; stderr: %s", err, diagnostics.String())
	}
	restored, err = os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(source, restored) {
		t.Fatal("false-healing run did not restore source")
	}
}

func disposableCheckout(t *testing.T, root string) string {
	t.Helper()
	checkout, err := os.MkdirTemp(filepath.Join(root, "examples", "functional-ui"), "argus-validation-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(checkout); err != nil {
			t.Errorf("remove disposable checkout: %v", err)
		}
	})
	base := filepath.Join(root, "examples", "functional-ui", "test-repository")
	for _, name := range []string{"package.json", "playwright.config.mjs", "argus-tests.json", filepath.Join("tests", "shop.spec.mjs")} {
		target := filepath.Join(checkout, name)
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(base, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	gitOutput(t, checkout, "init", "-q")
	gitOutput(t, checkout, "config", "user.name", "Argus fixture")
	gitOutput(t, checkout, "config", "user.email", "argus@example.invalid")
	gitOutput(t, checkout, "add", ".")
	gitOutput(t, checkout, "commit", "-q", "-m", "browser validation fixture")
	return checkout
}

func gitOutput(t *testing.T, root string, args ...string) string {
	t.Helper()
	command := exec.CommandContext(t.Context(), "git", append([]string{"-C", root}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
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
		var snapshot browserlocator.Snapshot
		if err := json.Unmarshal(data, &snapshot); err != nil {
			t.Fatal(err)
		}
		return data
	}
	t.Fatal("missing accessibility artifact")
	return nil
}
