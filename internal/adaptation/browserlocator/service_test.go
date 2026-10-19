package browserlocator

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/execution/browser"
)

func fixture(t *testing.T) Request {
	t.Helper()
	start := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	repo := catalog.Repository{Identity: catalog.RepositoryIdentity{Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "tests-1"}, Owner: "example", Name: "browser-tests"}
	revision := catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: strings.Repeat("a", 40)}
	makeAttempt := func(id string, started time.Time, outcome execution.TestOutcome, failure *execution.Failure, data []byte) browser.Attempt {
		request := browser.Request{APIVersion: browser.RequestVersion, AttemptID: id,
			Manifest: execution.ManifestReference{APIVersion: browser.ManifestVersion, SHA256: strings.Repeat("b", 64)},
			Stage:    execution.StageFullSuite, TestRepository: repo, TestRevision: revision,
			Adapter: "playwright", Project: "chromium-desktop", Environment: "staging",
			Tests: []browser.Test{{SuiteKey: "shop", TestKey: "checkout", Name: "checkout"}}}
		artifacts := []browser.Artifact{}
		for _, kind := range []string{"trace", "screenshot", "accessibility"} {
			sha := strings.Repeat("c", 64)
			if kind == "accessibility" {
				sha = digest(data)
			}
			artifacts = append(artifacts, browser.Artifact{Key: kind, SuiteKey: "shop", TestKey: "checkout", Kind: kind,
				URI: (&url.URL{Scheme: "file", Path: "/tmp/" + id + "/" + kind}).String(), SHA256: sha})
		}
		result := browser.Result{APIVersion: browser.ResultVersion, AttemptID: id, AdapterID: "playwright", AdapterVersion: "1.58.2",
			Project: "chromium-desktop", Environment: "staging", StartedAt: started, CompletedAt: started.Add(time.Minute),
			Results: []execution.TestResult{{SuiteKey: "shop", TestKey: "checkout", Outcome: outcome, Duration: time.Second, Failure: failure}}, Artifacts: artifacts}

		return browser.Attempt{APIVersion: browser.AttemptVersion, Request: request, Result: result, Outcome: execution.DeriveAttemptOutcome(result.Results)}
	}
	snapshot := func(captured time.Time, nodes []Node) []byte {
		data, err := json.Marshal(Snapshot{APIVersion: SnapshotVersion, CapturedAt: captured, URL: "https://example.test/checkout", Complete: true, Nodes: nodes, ARIA: "- button: Checkout"})
		if err != nil {
			t.Fatal(err)
		}

		return data
	}
	baseline := snapshot(start.Add(30*time.Second), []Node{{Role: "button", Name: "Checkout", TestID: "checkout-action", Visible: true}})
	failed := snapshot(start.Add(3*time.Minute), []Node{{Role: "button", Name: "Place order", TestID: "checkout-action", Visible: true}})

	return Request{Baseline: makeAttempt("baseline-1", start, execution.TestPassed, nil, baseline),
		Failed: makeAttempt("failed-1", start.Add(2*time.Minute), execution.TestFailed,
			&execution.Failure{Code: "locator-not-found", Message: "Locator: getByRole('button', { name: 'Checkout' })\nReceived: <element(s) not found>"}, failed),
		SuiteKey: "shop", TestKey: "checkout", SourcePath: "tests/shop.spec.mjs",
		Source:       []byte(`test("checkout [argus:shop:checkout]", async () => { await expect(page.getByRole("button", { name: "Checkout" })).toBeVisible(); });`),
		BaselineData: baseline, FailedData: failed}
}

func TestUniqueRoleRenameProducesBoundedProposal(t *testing.T) {
	request := fixture(t)
	diagnosis, err := Diagnose(request)
	if err != nil {
		t.Fatal(err)
	}
	if diagnosis.ReasonCode != "candidate" || diagnosis.Proposal == nil {
		t.Fatalf("diagnosis: %+v", diagnosis)
	}
	edit := diagnosis.Proposal.Edit
	if edit.Replacement != `getByRole("button", { name: "Place order" })` ||
		string(request.Source[edit.StartByte:edit.EndByte]) != edit.Original || edit.BeforeSHA256 != digest(request.Source) {
		t.Fatalf("unsafe edit: %+v", edit)
	}
	if err := ValidateProposal(*diagnosis.Proposal); err != nil {
		t.Fatal(err)
	}
}

func TestUniqueTestIDRenameUsesStableRoleAndName(t *testing.T) {
	request := fixture(t)
	request.Source = []byte(`test("checkout [argus:shop:checkout]", async () => { await expect(page.getByTestId("old-action")).toBeVisible(); });`)
	request.Failed.Result.Results[0].Failure.Message = "Locator: getByTestId('old-action')\nReceived: <element(s) not found>"
	baseline := parseFor(t, request.BaselineData)
	baseline.Nodes[0].TestID = "old-action"
	request.BaselineData = marshalFor(t, baseline)
	request.Baseline.Result.Artifacts[2].SHA256 = digest(request.BaselineData)
	current := parseFor(t, request.FailedData)
	current.Nodes[0].Name = "Checkout"
	request.FailedData = marshalFor(t, current)
	bindFailed(&request)
	diagnosis, err := Diagnose(request)
	if err != nil {
		t.Fatal(err)
	}
	if diagnosis.Proposal == nil || diagnosis.Proposal.Edit.Replacement != `getByTestId("checkout-action")` {
		t.Fatalf("test ID diagnosis: %+v", diagnosis)
	}
}

func TestAbstainsOnNonLocatorAndUnsafeTargets(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Request)
		reason string
	}{
		{"assertion", func(r *Request) { r.Failed.Result.Results[0].Failure.Code = "assertion" }, "non-locator-failure"},
		{"product regression", func(r *Request) {
			r.FailedData = snapshotFor(t, r.Failed.Result.StartedAt.Add(time.Second), nil)
			bindFailed(r)
		}, "ambiguous-or-hidden-target"},
		{"duplicate target", func(r *Request) {
			s := parseFor(t, r.FailedData)
			s.Nodes = append(s.Nodes, s.Nodes[0])
			r.FailedData = marshalFor(t, s)
			bindFailed(r)
		}, "ambiguous-or-hidden-target"},
		{"hidden target", func(r *Request) {
			s := parseFor(t, r.FailedData)
			s.Nodes[0].Visible = false
			r.FailedData = marshalFor(t, s)
			bindFailed(r)
		}, "ambiguous-or-hidden-target"},
		{"old target remains", func(r *Request) {
			s := parseFor(t, r.FailedData)
			s.Nodes = append(s.Nodes, Node{Role: "button", Name: "Checkout", TestID: "other", Visible: true})
			r.FailedData = marshalFor(t, s)
			bindFailed(r)
		}, "unproved-original-target"},
		{"navigation", func(r *Request) {
			s := parseFor(t, r.FailedData)
			s.URL = "https://example.test/other"
			r.FailedData = marshalFor(t, s)
			bindFailed(r)
		}, "cross-flow-navigation"},
		{"positional", func(r *Request) {
			r.Source = []byte(`await expect(page.getByRole("button", { name: "Checkout" }).first()).toBeVisible();`)
		}, "positional-locator"},
		{"unsupported", func(r *Request) { r.Source = []byte(`await expect(page.locator("button")).toBeVisible();`) }, "ambiguous-or-unsupported-source"},
		{"cross flow", func(r *Request) {
			r.Source = []byte(`test("receipt [argus:shop:receipt]", async () => { await expect(page.getByRole("button", { name: "Checkout" })).toBeVisible(); });`)
		}, "cross-flow-source"},
		{"assertion edit", func(r *Request) {
			r.Source = []byte(`await expect(page.getByRole("button", { name: "Checkout" })).toHaveText("Place order");`)
		}, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := fixture(t)
			test.change(&request)
			if !strings.Contains(string(request.Source), "test(") {
				request.Source = []byte(`test("checkout [argus:shop:checkout]", async () => { ` + string(request.Source) + ` });`)
			}
			got, err := Diagnose(request)
			if err != nil {
				t.Fatal(err)
			}
			if test.reason != "" && (got.ReasonCode != test.reason || got.Proposal != nil) {
				t.Fatalf("diagnosis: %+v", got)
			}
			if test.reason == "" && (got.Proposal == nil || got.Proposal.Edit.Original == `"Place order"`) {
				t.Fatalf("oracle changed: %+v", got)
			}
		})
	}
}

func TestRejectsStaleOrUnboundEvidence(t *testing.T) {
	for _, change := range []func(*Request){
		func(r *Request) {
			s := parseFor(t, r.FailedData)
			s.CapturedAt = r.Failed.Result.CompletedAt.Add(time.Second)
			r.FailedData = marshalFor(t, s)
			bindFailed(r)
		},
		func(r *Request) { r.Failed.Result.Artifacts[2].SHA256 = strings.Repeat("0", 64) },
		func(r *Request) { r.Failed.Request.Project = "chromium-mobile" },
		func(r *Request) { r.Failed.Request.TestRevision.Digest = strings.Repeat("d", 40) },
	} {
		request := fixture(t)
		change(&request)
		if _, err := Diagnose(request); err == nil {
			t.Fatal("accepted stale or unbound evidence")
		}
	}
}

func parseFor(t *testing.T, data []byte) Snapshot {
	t.Helper()
	var value Snapshot
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}

	return value
}
func marshalFor(t *testing.T, value Snapshot) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}

	return data
}
func snapshotFor(t *testing.T, at time.Time, nodes []Node) []byte {
	t.Helper()

	return marshalFor(t, Snapshot{APIVersion: SnapshotVersion, CapturedAt: at, URL: "https://example.test/checkout", Complete: true, Nodes: nodes})
}
func bindFailed(r *Request) { r.Failed.Result.Artifacts[2].SHA256 = digest(r.FailedData) }
