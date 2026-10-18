package browsershadow_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/execution"
	executioncontract "github.com/stanimirivanov/argus/internal/execution/adapters/contract"
	"github.com/stanimirivanov/argus/internal/execution/browser"
	"github.com/stanimirivanov/argus/internal/execution/browserplanning"
	"github.com/stanimirivanov/argus/internal/execution/browsershadow"
	"github.com/stanimirivanov/argus/internal/selection"
)

type memoryAttempts map[string]browser.Attempt

func (m memoryAttempts) FindBrowserAttempt(_ context.Context, id string) (browser.Attempt, error) {
	a, ok := m[id]
	if !ok {
		return browser.Attempt{}, execution.ErrNotFound
	}

	return a, nil
}

func fixture(t *testing.T) (browserplanning.Plan, memoryAttempts, browsershadow.Bindings) {
	t.Helper()
	repo := catalog.Repository{Identity: catalog.RepositoryIdentity{Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "tests-1"}, Owner: "example", Name: "browser-tests"}
	revision := catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: strings.Repeat("a", 40)}
	manifest := browser.Manifest{APIVersion: browser.ManifestVersion, Decisions: []browser.Decision{
		{Repository: repo, SuiteKey: "shop", TestKey: "checkout", Name: "checkout", Revision: revision, Projects: []string{"desktop"}, Outcome: selection.OutcomeRunRequired},
		{Repository: repo, SuiteKey: "shop", TestKey: "receipt", Name: "receipt", Revision: revision, Projects: []string{"desktop", "mobile"}, Outcome: selection.OutcomeSkipForNow},
	}}
	groups := browserplanning.Bindings{APIVersion: browserplanning.BindingsVersion, Groups: []browserplanning.Binding{
		{GroupKey: "desktop-ci", TestRepository: repo, TestRevision: revision, Adapter: "playwright", Project: "desktop", Environment: "ci"},
		{GroupKey: "mobile-ci", TestRepository: repo, TestRevision: revision, Adapter: "playwright", Project: "mobile", Environment: "ci"},
	}}
	plan, err := browserplanning.Build(manifest, strings.Repeat("b", 64), groups)
	if err != nil {
		t.Fatal(err)
	}
	stored := memoryAttempts{}
	selected := "desktop-selected"
	bindings := browsershadow.Bindings{APIVersion: browsershadow.BindingsVersion, Groups: []browsershadow.GroupBinding{{GroupKey: "desktop-ci", SelectedAttemptID: &selected, FullSuiteAttemptID: "desktop-full"}, {GroupKey: "mobile-ci", FullSuiteAttemptID: "mobile-full"}}}
	for _, job := range plan.Jobs {
		id := "desktop-full"
		if job.Project == "mobile" {
			id = "mobile-full"
		} else if job.Stage == execution.StageSelected {
			id = selected
		}
		stored[id] = attempt(t, job, plan.Manifest, id)
	}

	return plan, stored, bindings
}

func attempt(t *testing.T, job browserplanning.Job, manifest execution.ManifestReference, id string) browser.Attempt {
	t.Helper()
	tests := []browser.Test{}
	if job.Project == "desktop" {
		tests = append(tests, browser.Test{SuiteKey: "shop", TestKey: "checkout", Name: "checkout"})
	}
	if job.Stage == execution.StageFullSuite {
		tests = append(tests, browser.Test{SuiteKey: "shop", TestKey: "receipt", Name: "receipt"})
	}
	request := browser.Request{APIVersion: browser.RequestVersion, AttemptID: id, Manifest: manifest, Stage: job.Stage, TestRepository: job.TestRepository, TestRevision: job.TestRevision, Adapter: job.Adapter, Project: job.Project, Environment: job.Environment, Tests: tests}
	now := time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC)
	result := browser.Result{APIVersion: browser.ResultVersion, AttemptID: id, AdapterID: job.Adapter, AdapterVersion: "1.58.2", Project: job.Project, Environment: job.Environment, StartedAt: now, CompletedAt: now.Add(time.Second), Results: []execution.TestResult{}, Artifacts: []browser.Artifact{}}
	for _, test := range tests {
		outcome := execution.TestPassed
		var failure *execution.Failure
		if test.TestKey == "receipt" {
			outcome = execution.TestFailed
			failure = &execution.Failure{Code: "assertion", Message: "receipt unavailable"}
		}
		result.Results = append(result.Results, execution.TestResult{SuiteKey: test.SuiteKey, TestKey: test.TestKey, Outcome: outcome, Duration: time.Second, Failure: failure})
		for _, kind := range []string{"trace", "screenshot", "accessibility"} {
			result.Artifacts = append(result.Artifacts, browser.Artifact{Key: test.TestKey + "-" + kind, SuiteKey: test.SuiteKey, TestKey: test.TestKey, Kind: kind, URI: "file:///evidence/" + id + "/" + test.TestKey + "/" + kind, SHA256: strings.Repeat("c", 64)})
		}
	}
	a := browser.Attempt{APIVersion: browser.AttemptVersion, Request: request, Result: result, Outcome: execution.DeriveAttemptOutcome(result.Results)}
	if err := browser.ValidateAttempt(a); err != nil {
		t.Fatal(err)
	}

	return a
}

func TestCompareBrowserPlanPublishesFullOnlyMisses(t *testing.T) {
	plan, stored, bindings := fixture(t)
	report, err := browsershadow.Compare(t.Context(), stored, plan, strings.Repeat("d", 64), bindings)
	if err != nil {
		t.Fatal(err)
	}
	if report.GroupCount != 2 || report.FullOnlyGroupCount != 1 || report.GroupsWithSelection != 1 || report.FullSuiteFailureCount != 2 || report.CaughtFullSuiteFailureCount != 0 || report.FailureRecallBasisPoints == nil || *report.FailureRecallBasisPoints != 0 || len(report.Groups[0].MissedFailures) != 1 || len(report.Groups[1].MissedFailures) != 1 {
		t.Fatalf("incomplete browser shadow report: %+v", report)
	}
	if _, err := executioncontract.ExportBrowserPlanShadowReport(report); err != nil {
		t.Fatalf("browser shadow contract: %v", err)
	}
}
func TestCompareBrowserPlanRejectsMissingAndCrossProjectControls(t *testing.T) {
	plan, stored, bindings := fixture(t)
	delete(stored, "mobile-full")
	if _, err := browsershadow.Compare(t.Context(), stored, plan, strings.Repeat("d", 64), bindings); err == nil {
		t.Fatal("missing control accepted")
	}
	plan, stored, bindings = fixture(t)
	mobile := stored["mobile-full"]
	mobile.Request.Project = "desktop"
	mobile.Result.Project = "desktop"
	stored["mobile-full"] = mobile
	if _, err := browsershadow.Compare(t.Context(), stored, plan, strings.Repeat("d", 64), bindings); err == nil {
		t.Fatal("cross-project control accepted")
	}
	plan, stored, bindings = fixture(t)
	bindings.Groups[1].FullSuiteAttemptID = "desktop-full"
	if _, err := browsershadow.Compare(t.Context(), stored, plan, strings.Repeat("d", 64), bindings); err == nil {
		t.Fatal("reused attempt accepted")
	}
}
