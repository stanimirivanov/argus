package browservalidationcli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/adaptation/browserlocator"
	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/execution/browser"
)

type fixedWorkspace struct{ source []byte }

func (w fixedWorkspace) Read(context.Context, string) ([]byte, error)                   { return w.source, nil }
func (w fixedWorkspace) WriteIfUnchanged(context.Context, string, string, []byte) error { return nil }
func (w fixedWorkspace) Restore(context.Context, string, []byte) error                  { return nil }

func TestVerifyProposalRequiresReviewedBindingAndCorrelatedSnapshots(t *testing.T) {
	checkout, artifacts := t.TempDir(), t.TempDir()
	source := []byte(`test("checkout [argus:shop:checkout]", async () => { await expect(page.getByRole("button", { name: "Checkout" })).toBeVisible(); });`)
	baseTime := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	baselineData := snapshot(t, baseTime.Add(time.Second), "Checkout")
	failedData := snapshot(t, baseTime.Add(3*time.Minute+time.Second), "Place order")
	repo := catalog.Repository{Identity: catalog.RepositoryIdentity{Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "tests-1"}, Owner: "example", Name: "browser-tests"}
	request := browser.Request{APIVersion: browser.RequestVersion, AttemptID: "baseline-1",
		Manifest: execution.ManifestReference{APIVersion: browser.ManifestVersion, SHA256: strings.Repeat("a", 64)},
		Stage:    execution.StageFullSuite, TestRepository: repo, TestRevision: catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: strings.Repeat("b", 40)},
		Adapter: "playwright", Project: "chromium-desktop", Environment: "ci",
		Tests: []browser.Test{{SuiteKey: "shop", TestKey: "checkout", Name: "checkout [argus:shop:checkout]"}}}
	baseline := makeAttempt(t, request, artifacts, baselineData, baseTime, execution.TestPassed)
	request.AttemptID = "failed-1"
	failed := makeAttempt(t, request, artifacts, failedData, baseTime.Add(3*time.Minute), execution.TestFailed)
	diagnosis, err := browserlocator.Diagnose(browserlocator.Request{Baseline: baseline, Failed: failed,
		SuiteKey: "shop", TestKey: "checkout", SourcePath: "tests/shop.spec.mjs", Source: source,
		BaselineData: baselineData, FailedData: failedData})
	if err != nil || diagnosis.Proposal == nil {
		t.Fatalf("diagnose fixture: %+v %v", diagnosis, err)
	}
	registryPath := filepath.Join(checkout, "argus-tests.json")
	writeRegistry := func(file string) {
		value := map[string]any{"apiVersion": "argus.dev/reference-browser-test-map/v1", "tests": []map[string]string{{
			"suiteKey": "shop", "testKey": "checkout", "name": request.Tests[0].Name, "file": file}}}
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(registryPath, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeRegistry("tests/shop.spec.mjs")
	workspace := fixedWorkspace{source}
	if err := verifyProposal(t.Context(), workspace, checkout, artifacts, "argus-tests.json", *diagnosis.Proposal, baseline, failed); err != nil {
		t.Fatal(err)
	}
	writeRegistry("tests/other.spec.mjs")
	if err := verifyProposal(t.Context(), workspace, checkout, artifacts, "argus-tests.json", *diagnosis.Proposal, baseline, failed); err == nil {
		t.Fatal("accepted unreviewed source binding")
	}
	writeRegistry("tests/shop.spec.mjs")
	forged := *diagnosis.Proposal
	forged.CurrentTarget.Name = "Different target"
	if err := verifyProposal(t.Context(), workspace, checkout, artifacts, "argus-tests.json", forged, baseline, failed); err == nil {
		t.Fatal("accepted proposal not derived from snapshots")
	}
}

func snapshot(t *testing.T, captured time.Time, name string) []byte {
	t.Helper()
	data, err := json.Marshal(browserlocator.Snapshot{APIVersion: browserlocator.SnapshotVersion, CapturedAt: captured,
		URL: "https://example.test/checkout", Complete: true, ARIA: "- button: " + name,
		Nodes: []browserlocator.Node{{Role: "button", Name: name, TestID: "checkout-action", Visible: true}}})
	if err != nil {
		t.Fatal(err)
	}

	return data
}

func makeAttempt(t *testing.T, request browser.Request, root string, snapshotData []byte, started time.Time, outcome execution.TestOutcome) browser.Attempt {
	t.Helper()
	var failure *execution.Failure
	if outcome == execution.TestFailed {
		failure = &execution.Failure{Code: "locator-not-found", Message: "Locator: getByRole('button', { name: 'Checkout' })\nReceived: <element(s) not found>"}
	}
	artifacts := make([]browser.Artifact, 0, 3)
	for _, kind := range []string{"trace", "screenshot", "accessibility"} {
		data := []byte(kind)
		if kind == "accessibility" {
			data = snapshotData
		}
		path := filepath.Join(root, request.AttemptID+"-"+kind)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		uriPath := filepath.ToSlash(path)
		if filepath.VolumeName(path) != "" {
			uriPath = "/" + uriPath
		}
		sum := sha256.Sum256(data)
		artifacts = append(artifacts, browser.Artifact{Key: kind, SuiteKey: "shop", TestKey: "checkout", Kind: kind,
			URI: (&url.URL{Scheme: "file", Path: uriPath}).String(), SHA256: hex.EncodeToString(sum[:])})
	}
	result := browser.Result{APIVersion: browser.ResultVersion, AttemptID: request.AttemptID, AdapterID: "playwright", AdapterVersion: "1.58.2",
		Project: request.Project, Environment: request.Environment, StartedAt: started, CompletedAt: started.Add(time.Minute),
		Results: []execution.TestResult{{SuiteKey: "shop", TestKey: "checkout", Outcome: outcome, Duration: time.Second, Failure: failure}}, Artifacts: artifacts}

	return browser.Attempt{APIVersion: browser.AttemptVersion, Request: request, Result: result, Outcome: execution.DeriveAttemptOutcome(result.Results)}
}
