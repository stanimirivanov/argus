package contract

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/execution/browser"
)

func TestBrowserAttemptContractRoundTrip(t *testing.T) {
	now := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
	repository := catalog.Repository{Identity: catalog.RepositoryIdentity{Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "tests-1"}, Owner: "example", Name: "browser-tests"}
	request := browser.Request{APIVersion: browser.RequestVersion, AttemptID: "browser-1", Manifest: execution.ManifestReference{APIVersion: browser.ManifestVersion, SHA256: strings.Repeat("a", 64)},
		Stage: execution.StageSelected, TestRepository: repository, TestRevision: catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: strings.Repeat("b", 40)}, Adapter: "playwright", Project: "chromium-desktop", Environment: "ci",
		Tests: []browser.Test{{SuiteKey: "shop", TestKey: "checkout", Name: "checkout shows total"}}}
	result := browser.Result{APIVersion: browser.ResultVersion, AttemptID: request.AttemptID, AdapterID: "playwright", AdapterVersion: "1.58.2", Project: request.Project, Environment: request.Environment,
		StartedAt: now, CompletedAt: now.Add(time.Second), Results: []execution.TestResult{{SuiteKey: "shop", TestKey: "checkout", Outcome: execution.TestPassed, Duration: time.Second}},
		Artifacts: []browser.Artifact{{Key: "trace", SuiteKey: "shop", TestKey: "checkout", Kind: "trace", URI: "file:///tmp/trace.zip", SHA256: strings.Repeat("c", 64)},
			{Key: "screenshot", SuiteKey: "shop", TestKey: "checkout", Kind: "screenshot", URI: "file:///tmp/screenshot.png", SHA256: strings.Repeat("d", 64)},
			{Key: "accessibility", SuiteKey: "shop", TestKey: "checkout", Kind: "accessibility", URI: "file:///tmp/a11y.txt", SHA256: strings.Repeat("e", 64)}}}
	attempt := browser.Attempt{APIVersion: browser.AttemptVersion, Request: request, Result: result, Outcome: execution.AttemptPassed}
	document, err := ExportBrowserAttempt(attempt)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := ImportBrowserAttempt(document)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, browser.CanonicalAttempt(attempt)) {
		t.Fatalf("browser contract round trip differs: %+v", decoded)
	}
}
