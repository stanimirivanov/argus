//go:build integration

package postgres

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"reflect"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/execution/browser"
)

func stagedBrowserAttempt(id string, stage execution.Stage, mode string) browser.Attempt {
	base := integrationExecutionAttempt(id, execution.StageSelected)
	tests := []browser.Test{{SuiteKey: "orders", TestKey: "create", Name: "create order"}}
	request := browser.Request{APIVersion: browser.StagedRequestVersion, AttemptID: id, Manifest: execution.ManifestReference{APIVersion: browser.ManifestVersion, SHA256: strings.Repeat("a", 64)}, Stage: stage, TestRepository: base.TestRepository, TestRevision: base.TestRevision, Adapter: "playwright", Project: "chromium-desktop", Environment: "ci", Tests: tests, Candidates: tests,
		Staging: &browser.Staging{Staging: execution.Staging{Schedule: execution.ScheduleReference{APIVersion: execution.ScheduleAPIVersion, SHA256: strings.Repeat("b", 64)}, EpisodeID: "episode", GroupKey: "desktop", Environment: "ci", Mode: mode}, DiscoverySHA256: strings.Repeat("c", 64)}}
	result := browser.Result{APIVersion: browser.ResultVersion, AttemptID: id, AdapterID: "playwright", AdapterVersion: "test", Project: request.Project, Environment: request.Environment, StartedAt: base.StartedAt, CompletedAt: base.CompletedAt,
		Results: []execution.TestResult{{SuiteKey: "orders", TestKey: "create", Outcome: execution.TestSkipped}}, Artifacts: []browser.Artifact{}}

	return browser.Attempt{APIVersion: browser.StagedAttemptVersion, Request: request, Result: result, Outcome: execution.AttemptIncomplete}
}

func TestBrowserStagesUpgradeRetryConflictAndRestart(t *testing.T) {
	url := newTestDatabase(t)
	migrator := openTestMigrator(t, url)
	applyReleasedBrowserSchema(t, migrator)
	store := openTestStore(t, url)
	legacy := stagedBrowserAttempt("legacy-browser", execution.StageSelected, "shadow")
	legacy.APIVersion, legacy.Request.APIVersion, legacy.Request.Staging, legacy.Request.Candidates = browser.AttemptVersion, browser.RequestVersion, nil, nil
	legacy = browser.CanonicalAttempt(legacy)
	payload, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(payload, []byte(`"staging"`)) || bytes.Contains(payload, []byte(`"candidates"`)) {
		t.Fatal("legacy payload gained new fields")
	}
	sum := sha256.Sum256(payload)
	_, err = store.pool.Exec(t.Context(), `INSERT INTO argus_catalog.repositories(provider,host,provider_repository_id,owner_name,repository_name) VALUES ('github','github.com','integration-tests-42','example','tests')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.pool.Exec(t.Context(), `INSERT INTO argus_catalog.browser_execution_attempts(attempt_id,attempt_api_version,manifest_sha256,test_repository_id,project_key,environment_key,attempt_outcome,attempt_sha256,payload)
 SELECT $1,$2,$3,repository_id,$4,$5,$6,$7,$8 FROM argus_catalog.repositories WHERE provider_repository_id='integration-tests-42'`, legacy.Request.AttemptID, legacy.APIVersion, legacy.Request.Manifest.SHA256, legacy.Request.Project, legacy.Request.Environment, legacy.Outcome, hex.EncodeToString(sum[:]), payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrator.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	got, err := store.FindBrowserAttempt(t.Context(), legacy.Request.AttemptID)
	if err != nil || !reflect.DeepEqual(got, legacy) {
		t.Fatalf("legacy upgrade: %+v %v", got, err)
	}
	if created, err := store.SaveBrowserAttempt(t.Context(), legacy); created || err != nil {
		t.Fatalf("legacy retry: %t %v", created, err)
	}
	for _, stage := range []execution.Stage{execution.StageRelevantNow, execution.StageRemainingLater, execution.StageShadowControl} {
		mode := "staged"
		if stage == execution.StageShadowControl {
			mode = "shadow"
		}
		attempt := stagedBrowserAttempt(string(stage), stage, mode)
		checkBrowserDatabaseRetry(t, url, store, attempt)
	}
	store.Close()
	reopened, err := openIntegratedStore(t.Context(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reopened.Close)
	if _, err := reopened.FindBrowserAttempt(t.Context(), string(execution.StageShadowControl)); err != nil {
		t.Fatal(err)
	}
}

func checkBrowserDatabaseRetry(t *testing.T, url string, store *integratedStore, attempt browser.Attempt) {
	t.Helper()
	stores := []*integratedStore{store, openTestStore(t, url)}
	created, failures := make([]bool, 2), make([]error, 2)
	var group sync.WaitGroup
	for i := range stores {
		group.Add(1)
		go func() {
			defer group.Done()
			created[i], failures[i] = stores[i].SaveBrowserAttempt(t.Context(), attempt)
		}()
	}
	group.Wait()
	if failures[0] != nil || failures[1] != nil || created[0] == created[1] {
		t.Fatalf("concurrent retry: %v %v", created, failures)
	}
	got, err := stores[1].FindBrowserAttempt(t.Context(), attempt.Request.AttemptID)
	if err != nil || !reflect.DeepEqual(got, browser.CanonicalAttempt(attempt)) {
		t.Fatalf("staged round trip: %+v %v", got, err)
	}
	changed := browser.CanonicalAttempt(attempt)
	changed.Request.Staging.DiscoverySHA256 = strings.Repeat("f", 64)
	if _, err := store.SaveBrowserAttempt(t.Context(), changed); !errors.Is(err, execution.ErrConflict) {
		t.Fatalf("discovery conflict: %v", err)
	}
	_, err = store.pool.Exec(t.Context(), `UPDATE argus_catalog.browser_execution_attempts SET discovery_sha256=NULL WHERE attempt_id=$1`, attempt.Request.AttemptID)
	var constraint *pgconn.PgError
	if !errors.As(err, &constraint) || constraint.Code != "23514" {
		t.Fatalf("missing provenance accepted: %v", err)
	}
}

func applyReleasedBrowserSchema(t *testing.T, migrator *Migrator) {
	t.Helper()
	chain := fstest.MapFS{}
	files, err := fs.ReadDir(embeddedMigrations, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if file.Name() >= "20261022062238_add_predictive_browser_stages.sql" {
			continue
		}
		name := "migrations/" + file.Name()
		data, err := fs.ReadFile(embeddedMigrations, name)
		if err != nil {
			t.Fatal(err)
		}
		chain[name] = &fstest.MapFile{Data: data}
	}
	if err := applyMigrationChain(t.Context(), migrator, chain); err != nil {
		t.Fatal(err)
	}
}
