package browservalidation

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/adaptation"
	"github.com/stanimirivanov/argus/internal/adaptation/browserlocator"
	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/execution/browser"
)

type memoryWorkspace struct {
	data       []byte
	mutate     bool
	restoreErr error
}

func (w *memoryWorkspace) Read(_ context.Context, _ string) ([]byte, error) {
	return append([]byte{}, w.data...), nil
}
func (w *memoryWorkspace) WriteIfUnchanged(_ context.Context, _ string, sha string, data []byte) error {
	if digest(w.data) != sha {
		return adaptation.ErrInvalid
	}
	w.data = append([]byte{}, data...)

	return nil
}
func (w *memoryWorkspace) Restore(_ context.Context, _ string, data []byte) error {
	if w.restoreErr != nil {
		return w.restoreErr
	}
	w.data = append([]byte{}, data...)

	return nil
}

type fakeAdapter struct {
	workspace   *memoryWorkspace
	proposal    browserlocator.Proposal
	outcomes    map[Phase][2]execution.TestOutcome
	failureCode string
	verifyErr   error
	runErr      error
	cancel      context.CancelFunc
	count       int
}

func (adapter *fakeAdapter) Execute(ctx context.Context, request browser.Request) (browser.Result, error) {
	adapter.count++
	if adapter.cancel != nil && adapter.count == 3 {
		adapter.cancel()
		return browser.Result{}, ctx.Err()
	}
	if adapter.runErr != nil && adapter.count == 3 {
		return browser.Result{}, adapter.runErr
	}
	phase := Original
	if strings.Contains(string(adapter.workspace.data), adapter.proposal.Edit.Replacement) {
		phase = Candidate
	}
	if strings.Contains(string(adapter.workspace.data), "__argus_negative_locator_") {
		phase = NegativeControl
	}
	repeat := (adapter.count - 1) % 2
	outcome := adapter.outcomes[phase][repeat]
	if adapter.workspace.mutate {
		adapter.workspace.data = append(adapter.workspace.data, '!')
	}
	start := time.Date(2026, 10, 18, 12, 4+adapter.count*2, 0, 0, time.UTC)
	var failure *execution.Failure
	if outcome == execution.TestFailed || outcome == execution.TestError {
		code := adapter.failureCode
		if code == "" {
			code = "locator-not-found"
		}
		failure = &execution.Failure{Code: code, Message: "locator did not find target"}
	}

	return browser.Result{APIVersion: browser.ResultVersion, AttemptID: request.AttemptID,
		AdapterID: request.Adapter, AdapterVersion: "1.58.2", Project: request.Project, Environment: request.Environment,
		StartedAt: start, CompletedAt: start.Add(time.Second), Results: []execution.TestResult{{
			SuiteKey: request.Tests[0].SuiteKey, TestKey: request.Tests[0].TestKey,
			Outcome: outcome, Duration: time.Second, Failure: failure}}, Artifacts: artifacts(request.Tests[0], strings.Repeat("c", 64))}, nil
}
func (adapter *fakeAdapter) VerifyArtifacts(_ context.Context, _ browser.Result) error {
	return adapter.verifyErr
}

func fixture(t *testing.T) (browserlocator.Proposal, browser.Attempt, browser.Attempt, *memoryWorkspace, *fakeAdapter) {
	t.Helper()
	source := []byte(`test("checkout [argus:shop:checkout]", async () => { await expect(page.getByRole("button", { name: "Checkout" })).toBeVisible(); });`)
	old := `getByRole("button", { name: "Checkout" })`
	start := strings.Index(string(source), old)
	proposal := browserlocator.Proposal{APIVersion: browserlocator.ProposalVersion, PolicyVersion: browserlocator.PolicyVersion,
		BaselineID: "baseline-1", FailedID: "failed-1", BaselineSHA256: strings.Repeat("a", 64), FailedSHA256: strings.Repeat("b", 64),
		SuiteKey: "shop", TestKey: "checkout", OriginalTarget: browserlocator.Node{Role: "button", Name: "Checkout", TestID: "action", Visible: true},
		CurrentTarget: browserlocator.Node{Role: "button", Name: "Place order", TestID: "action", Visible: true},
		Edit: adaptation.TextEdit{Path: "tests/shop.spec.mjs", BeforeSHA256: digest(source), StartByte: start, EndByte: start + len(old),
			Original: old, Replacement: `getByRole("button", { name: "Place order" })`, SemanticRole: "browser-locator"}}
	copy := proposal
	data, err := json.Marshal(copy)
	if err != nil {
		t.Fatal(err)
	}
	proposal.ProposalID = digest(data)
	if err := browserlocator.ValidateProposal(proposal); err != nil {
		t.Fatal(err)
	}
	repo := catalog.Repository{Identity: catalog.RepositoryIdentity{Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "tests-1"}, Owner: "example", Name: "browser-tests"}
	request := browser.Request{APIVersion: browser.RequestVersion, AttemptID: "baseline-1",
		Manifest: execution.ManifestReference{APIVersion: browser.ManifestVersion, SHA256: strings.Repeat("d", 64)},
		Stage:    execution.StageFullSuite, TestRepository: repo, TestRevision: catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: strings.Repeat("e", 40)},
		Adapter: "playwright", Project: "chromium-desktop", Environment: "staging",
		Tests: []browser.Test{{SuiteKey: "shop", TestKey: "checkout", Name: "checkout [argus:shop:checkout]"}}}
	baseline := fixtureAttempt(request, execution.TestPassed, "", proposal.BaselineSHA256, 0)
	request.AttemptID = "failed-1"
	failed := fixtureAttempt(request, execution.TestFailed, "locator-not-found", proposal.FailedSHA256, 2)
	workspace := &memoryWorkspace{data: source}
	adapter := &fakeAdapter{workspace: workspace, proposal: proposal, outcomes: map[Phase][2]execution.TestOutcome{
		Original: {execution.TestFailed, execution.TestFailed}, Candidate: {execution.TestPassed, execution.TestPassed},
		NegativeControl: {execution.TestFailed, execution.TestFailed}}}

	return proposal, baseline, failed, workspace, adapter
}

func fixtureAttempt(request browser.Request, outcome execution.TestOutcome, code, snapshotSHA string, minute int) browser.Attempt {
	start := time.Date(2026, 10, 18, 12, minute, 0, 0, time.UTC)
	var failure *execution.Failure
	if code != "" {
		failure = &execution.Failure{Code: code, Message: "missing locator"}
	}
	result := browser.Result{APIVersion: browser.ResultVersion, AttemptID: request.AttemptID, AdapterID: request.Adapter,
		AdapterVersion: "1.58.2", Project: request.Project, Environment: request.Environment,
		StartedAt: start, CompletedAt: start.Add(time.Second), Results: []execution.TestResult{{SuiteKey: "shop", TestKey: "checkout", Outcome: outcome, Duration: time.Second, Failure: failure}},
		Artifacts: artifacts(request.Tests[0], snapshotSHA)}

	return browser.Attempt{APIVersion: browser.AttemptVersion, Request: request, Result: result, Outcome: execution.DeriveAttemptOutcome(result.Results)}
}

func artifacts(test browser.Test, snapshotSHA string) []browser.Artifact {
	var result []browser.Artifact
	for _, kind := range []string{"trace", "screenshot", "accessibility"} {
		sha := strings.Repeat("c", 64)
		if kind == "accessibility" {
			sha = snapshotSHA
		}
		result = append(result, browser.Artifact{Key: kind, SuiteKey: test.SuiteKey, TestKey: test.TestKey,
			Kind: kind, URI: (&url.URL{Scheme: "file", Path: "/tmp/" + kind}).String(), SHA256: sha})
	}

	return result
}

func TestValidateBrowserRepairProvesSixRunsAndRestoresSource(t *testing.T) {
	proposal, baseline, failed, workspace, adapter := fixture(t)
	original := append([]byte{}, workspace.data...)
	evidence, err := NewService(workspace, adapter).Validate(t.Context(), proposal, baseline, failed)
	if err != nil {
		t.Fatal(err)
	}
	if evidence.Status != Validated || len(evidence.Runs) != 6 || adapter.count != 6 || string(workspace.data) != string(original) {
		t.Fatalf("incomplete proof or restoration: %+v", evidence)
	}
	if err := ValidateEvidence(evidence); err != nil {
		t.Fatal(err)
	}
	altered := evidence
	altered.Runs = append([]Run{}, evidence.Runs...)
	altered.Runs[4].SourceSHA256 = strings.Repeat("f", 64)
	if ValidateEvidence(altered) == nil {
		t.Fatal("accepted tampered run")
	}
}

func TestValidateBrowserRepairSeparatesPolicyRejectionFromUntrustedRuns(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*fakeAdapter)
		reason string
	}{
		{"original passes", func(a *fakeAdapter) {
			a.outcomes[Original] = [2]execution.TestOutcome{execution.TestPassed, execution.TestPassed}
		}, "original-passed"},
		{"candidate locator fails", func(a *fakeAdapter) {
			a.outcomes[Candidate] = [2]execution.TestOutcome{execution.TestFailed, execution.TestFailed}
		}, "candidate-locator-failed"},
		{"negative control passes", func(a *fakeAdapter) {
			a.outcomes[NegativeControl] = [2]execution.TestOutcome{execution.TestPassed, execution.TestPassed}
		}, "negative-control-passed"},
		{"flaky candidate", func(a *fakeAdapter) {
			a.outcomes[Candidate] = [2]execution.TestOutcome{execution.TestPassed, execution.TestFailed}
		}, ""},
		{"product failure", func(a *fakeAdapter) {
			a.outcomes[Candidate] = [2]execution.TestOutcome{execution.TestFailed, execution.TestFailed}
			a.failureCode = "assertion"
		}, ""},
		{"missing artifact", func(a *fakeAdapter) { a.verifyErr = errors.New("missing artifact") }, ""},
		{"adapter error", func(a *fakeAdapter) { a.runErr = errors.New("adapter failed") }, ""},
		{"source mutation", func(a *fakeAdapter) { a.workspace.mutate = true }, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			proposal, baseline, failed, workspace, adapter := fixture(t)
			original := append([]byte{}, workspace.data...)
			test.change(adapter)
			_, err := NewService(workspace, adapter).Validate(t.Context(), proposal, baseline, failed)
			if err == nil {
				t.Fatal("accepted invalid repair")
			}
			var rejected *RejectedError
			if test.reason == "" && errors.As(err, &rejected) {
				t.Fatalf("mislabelled execution failure: %v", err)
			}
			if test.reason != "" && (!errors.As(err, &rejected) || rejected.Evidence.Reason != test.reason || ValidateEvidence(rejected.Evidence) != nil) {
				t.Fatalf("missing trustworthy rejection: %v", err)
			}
			if string(workspace.data) != string(original) {
				t.Fatal("source not restored")
			}
		})
	}
}

func TestValidateBrowserRepairRestoresAfterCancellationAndRestoreFailure(t *testing.T) {
	proposal, baseline, failed, workspace, adapter := fixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	adapter.cancel = cancel
	_, err := NewService(workspace, adapter).Validate(ctx, proposal, baseline, failed)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation = %v", err)
	}
	if digest(workspace.data) != proposal.Edit.BeforeSHA256 {
		t.Fatal("source not restored after cancellation")
	}
	proposal, baseline, failed, workspace, adapter = fixture(t)
	workspace.restoreErr = errors.New("restore failed")
	_, err = NewService(workspace, adapter).Validate(t.Context(), proposal, baseline, failed)
	if err == nil || !strings.Contains(err.Error(), "restore failed") {
		t.Fatalf("restoration failure = %v", err)
	}
}
