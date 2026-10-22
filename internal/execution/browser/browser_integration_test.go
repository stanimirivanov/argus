//go:build browserintegration

package browser_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/execution/adapters/processadapter"
	"github.com/stanimirivanov/argus/internal/execution/browser"
	"github.com/stanimirivanov/argus/internal/execution/browserplanning"
	"github.com/stanimirivanov/argus/internal/execution/browsershadow"
	"github.com/stanimirivanov/argus/internal/selection"
)

type browserMemoryAttempts map[string]browser.Attempt

func (stored browserMemoryAttempts) FindBrowserAttempt(_ context.Context, id string) (browser.Attempt, error) {
	attempt, ok := stored[id]
	if !ok {
		return browser.Attempt{}, execution.ErrNotFound
	}
	return attempt, nil
}

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
	runStagedBrowserCampaign(t, manifest, adapter, &diagnostics)
	if _, err := os.Stat(filepath.Join(root, "examples", "functional-ui", "test-repository", "test-results")); err == nil {
		t.Fatal("Playwright left output in fixture checkout")
	}
}

func TestReferenceBrowserPlanAndKnownShadowMiss(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ARGUS_BROWSER_TEST_DIR", filepath.Join(root, "examples", "functional-ui", "test-repository"))
	t.Setenv("ARGUS_BROWSER_ARTIFACT_DIR", t.TempDir())
	t.Setenv("ARGUS_BROWSER_ENVIRONMENT", "ci")
	t.Setenv("ARGUS_BROWSER_FAIL_RECEIPT", "1")
	revisionOutput, err := exec.CommandContext(t.Context(), "git", "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	revision := catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: strings.TrimSpace(string(revisionOutput))}
	repo := catalog.Repository{Identity: catalog.RepositoryIdentity{Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "tests-1"}, Owner: "example", Name: "browser-tests"}
	manifest := browser.Manifest{APIVersion: browser.ManifestVersion, Decisions: []browser.Decision{
		{Repository: repo, SuiteKey: "shop", TestKey: "checkout", Name: "checkout shows total [argus:shop:checkout]", Revision: revision, Projects: []string{"chromium-desktop"}, Outcome: selection.OutcomeRunRequired},
		{Repository: repo, SuiteKey: "shop", TestKey: "receipt", Name: "receipt shows order [argus:shop:receipt]", Revision: revision, Projects: []string{"chromium-desktop", "chromium-mobile"}, Outcome: selection.OutcomeSkipForNow},
	}}
	bindings := browserplanning.Bindings{APIVersion: browserplanning.BindingsVersion, Groups: []browserplanning.Binding{
		{GroupKey: "desktop-ci", TestRepository: repo, TestRevision: revision, Adapter: "playwright", Project: "chromium-desktop", Environment: "ci"},
		{GroupKey: "mobile-ci", TestRepository: repo, TestRevision: revision, Adapter: "playwright", Project: "chromium-mobile", Environment: "ci"},
	}}
	plan, err := browserplanning.Build(manifest, strings.Repeat("a", 64), bindings)
	if err != nil {
		t.Fatal(err)
	}
	var diagnostics bytes.Buffer
	adapter, err := processadapter.NewBrowser([]string{"node", filepath.Join(root, "examples", "functional-ui", "playwright-adapter.mjs")}, &diagnostics)
	if err != nil {
		t.Fatal(err)
	}
	stored := browserMemoryAttempts{}
	selected := "desktop-selected"
	for _, job := range plan.Jobs {
		id := "desktop-full"
		if job.Project == "chromium-mobile" {
			id = "mobile-full"
		} else if job.Stage == execution.StageSelected {
			id = selected
		}
		attempt, runErr := browser.NewService(adapter).Execute(t.Context(), manifest, browser.Options{AttemptID: id, ManifestSHA256: plan.Manifest.SHA256, Stage: job.Stage, TestRepository: repo.Identity, TestRevision: revision, Adapter: job.Adapter, Project: job.Project, Environment: job.Environment})
		if runErr != nil {
			t.Fatalf("run %s: %v; stderr: %s", id, runErr, diagnostics.String())
		}
		if err := browser.ValidateAttempt(attempt); err != nil {
			t.Fatal(err)
		}
		stored[id] = attempt
	}
	groupBindings := browsershadow.Bindings{APIVersion: browsershadow.BindingsVersion, Groups: []browsershadow.GroupBinding{
		{GroupKey: "desktop-ci", SelectedAttemptID: &selected, FullSuiteAttemptID: "desktop-full"},
		{GroupKey: "mobile-ci", FullSuiteAttemptID: "mobile-full"},
	}}
	report, err := browsershadow.Compare(t.Context(), stored, plan, strings.Repeat("b", 64), groupBindings)
	if err != nil {
		t.Fatal(err)
	}
	if report.GroupCount != 2 || report.FullOnlyGroupCount != 1 || report.FullSuiteFailureCount != 2 || report.CaughtFullSuiteFailureCount != 0 || len(report.Groups[0].MissedFailures) != 1 || len(report.Groups[1].MissedFailures) != 1 {
		t.Fatalf("wrong real browser shadow report: %+v", report)
	}
}
