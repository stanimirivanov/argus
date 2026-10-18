//go:build integration

package postgres

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/execution/browser"
)

func TestBrowserAttemptRoundTripRetryAndConflict(t *testing.T) {
	databaseURL := newTestDatabase(t)
	migrateTestDatabase(t, databaseURL)
	store := openTestStore(t, databaseURL)
	base := integrationExecutionAttempt("browser-1", execution.StageSelected)
	request := browser.Request{APIVersion: browser.RequestVersion, AttemptID: "browser-1", Manifest: execution.ManifestReference{APIVersion: browser.ManifestVersion, SHA256: strings.Repeat("a", 64)},
		Stage: execution.StageSelected, TestRepository: base.TestRepository, TestRevision: base.TestRevision, Adapter: "playwright", Project: "chromium-desktop", Environment: "ci",
		Tests: []browser.Test{{SuiteKey: "orders", TestKey: "create", Name: "create order"}}}
	result := browser.Result{APIVersion: browser.ResultVersion, AttemptID: "browser-1", AdapterID: "playwright", AdapterVersion: "1.58.2", Project: "chromium-desktop", Environment: "ci",
		StartedAt: base.StartedAt, CompletedAt: base.CompletedAt, Results: base.Results,
		Artifacts: []browser.Artifact{{Key: "trace", SuiteKey: "orders", TestKey: "create", Kind: "trace", URI: "file:///tmp/trace.zip", SHA256: strings.Repeat("b", 64)},
			{Key: "screenshot", SuiteKey: "orders", TestKey: "create", Kind: "screenshot", URI: "file:///tmp/screenshot.png", SHA256: strings.Repeat("d", 64)},
			{Key: "accessibility", SuiteKey: "orders", TestKey: "create", Kind: "accessibility", URI: "file:///tmp/a11y.txt", SHA256: strings.Repeat("c", 64)}}}
	attempt := browser.Attempt{APIVersion: browser.AttemptVersion, Request: request, Result: result, Outcome: execution.AttemptFailed}
	if _, err := store.FindBrowserAttempt(t.Context(), "browser-1"); !errors.Is(err, execution.ErrNotFound) {
		t.Fatalf("missing attempt: %v", err)
	}
	created, err := store.SaveBrowserAttempt(t.Context(), attempt)
	if err != nil || !created {
		t.Fatalf("save: %t %v", created, err)
	}
	created, err = store.SaveBrowserAttempt(t.Context(), browser.CanonicalAttempt(attempt))
	if err != nil || created {
		t.Fatalf("retry: %t %v", created, err)
	}
	stored, err := store.FindBrowserAttempt(t.Context(), "browser-1")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stored, browser.CanonicalAttempt(attempt)) {
		t.Fatalf("round trip mismatch: %+v", stored)
	}
	changed := attempt
	changed.Request.Project = "chromium-mobile"
	changed.Result.Project = "chromium-mobile"
	if _, err := store.SaveBrowserAttempt(t.Context(), changed); !errors.Is(err, execution.ErrConflict) {
		t.Fatalf("conflict: %v", err)
	}
}
