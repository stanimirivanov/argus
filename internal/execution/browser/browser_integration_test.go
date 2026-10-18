//go:build browserintegration

package browser_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/execution/adapters/processadapter"
	"github.com/stanimirivanov/argus/internal/execution/browser"
	"github.com/stanimirivanov/argus/internal/selection"
)

func TestReferencePlaywrightBrowserExecution(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	fixtureDir := filepath.Join(root, "examples", "functional-ui", "test-repository")
	t.Setenv("ARGUS_BROWSER_TEST_DIR", fixtureDir)
	t.Setenv("ARGUS_BROWSER_ARTIFACT_DIR", t.TempDir())
	t.Setenv("ARGUS_BROWSER_ENVIRONMENT", "ci")
	revisionOutput, err := exec.CommandContext(t.Context(), "git", "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	revision := catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: strings.TrimSpace(string(revisionOutput))}
	repo := catalog.Repository{Identity: catalog.RepositoryIdentity{Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "tests-1"}, Owner: "example", Name: "browser-tests"}
	manifest := browser.Manifest{APIVersion: browser.ManifestVersion, Decisions: []browser.Decision{
		{Repository: repo, SuiteKey: "shop", TestKey: "checkout", Name: "checkout shows total [argus:shop:checkout]", Revision: revision, Projects: []string{"chromium-desktop", "chromium-mobile"}, Outcome: selection.OutcomeRunRequired},
		{Repository: repo, SuiteKey: "shop", TestKey: "receipt", Name: "receipt shows order [argus:shop:receipt]", Revision: revision, Projects: []string{"chromium-desktop", "chromium-mobile"}, Outcome: selection.OutcomeSkipForNow},
	}}
	var diagnostics bytes.Buffer
	adapter, err := processadapter.NewBrowser([]string{"node", filepath.Join(root, "examples", "functional-ui", "playwright-adapter.mjs")}, &diagnostics)
	if err != nil {
		t.Fatal(err)
	}
	for _, stage := range []execution.Stage{execution.StageSelected, execution.StageFullSuite} {
		options := browser.Options{AttemptID: "browser-" + string(stage), ManifestSHA256: strings.Repeat("a", 64), Stage: stage, TestRepository: repo.Identity, TestRevision: revision, Adapter: "playwright", Project: "chromium-desktop", Environment: "ci"}
		attempt, err := browser.NewService(adapter).Execute(t.Context(), manifest, options)
		if err != nil {
			t.Fatalf("browser adapter: %v; stderr: %s", err, diagnostics.String())
		}
		expected := 1
		if stage == execution.StageFullSuite {
			expected = 2
		}
		if attempt.Outcome != execution.AttemptPassed || len(attempt.Result.Results) != expected || len(attempt.Result.Artifacts) != expected*3 {
			t.Fatalf("unexpected real browser attempt: %+v", attempt)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "examples", "functional-ui", "test-repository", "test-results")); err == nil {
		t.Fatal("Playwright left output in fixture checkout")
	}
}
