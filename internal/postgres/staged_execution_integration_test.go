//go:build integration

package postgres

import (
	"errors"
	"io/fs"
	"reflect"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stanimirivanov/argus/internal/execution"
)

func TestStagedAttemptsUpgradeRoundTripAndConcurrentRetry(t *testing.T) {
	databaseURL := newTestDatabase(t)
	migrator := openTestMigrator(t, databaseURL)
	released := fstest.MapFS{}
	files, err := fs.ReadDir(embeddedMigrations, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if file.Name() == "20261021211703_add_predictive_api_stages.sql" {
			continue
		}
		name := "migrations/" + file.Name()
		data, err := fs.ReadFile(embeddedMigrations, name)
		if err != nil {
			t.Fatal(err)
		}
		released[name] = &fstest.MapFile{Data: data}
	}
	if err := applyMigrationChain(t.Context(), migrator, released); err != nil {
		t.Fatal(err)
	}
	store := openTestStore(t, databaseURL)
	// The old writer targets only legacy columns; exercise it before migration.
	_, err = store.pool.Exec(t.Context(), `
		INSERT INTO argus_catalog.repositories (provider, host, provider_repository_id, owner_name, repository_name)
		VALUES ('github', 'github.com', 'integration-tests-42', 'example', 'tests')`)
	if err != nil {
		t.Fatal(err)
	}
	legacy := integrationExecutionAttempt("legacy-after-upgrade", execution.StageSelected)
	fingerprint, err := executionAttemptFingerprint(legacy)
	if err != nil {
		t.Fatal(err)
	}
	var key int64
	err = store.pool.QueryRow(t.Context(), `INSERT INTO argus_catalog.execution_attempts
 (attempt_id, attempt_api_version, manifest_api_version, manifest_sha256, execution_stage, test_repository_id,
 test_repository_owner_name, test_repository_name, test_revision_algorithm, test_revision_digest, adapter_id,
 adapter_version, started_at, completed_at, attempt_outcome, attempt_sha256)
 SELECT $1,$2,$3,$4,$5,repository_id,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15
 FROM argus_catalog.repositories WHERE provider_repository_id='integration-tests-42' RETURNING execution_attempt_id`,
		legacy.AttemptID, legacy.APIVersion, legacy.Manifest.APIVersion, legacy.Manifest.SHA256, legacy.Stage,
		legacy.TestRepository.Owner, legacy.TestRepository.Name, legacy.TestRevision.Algorithm, legacy.TestRevision.Digest,
		legacy.AdapterID, legacy.AdapterVersion, legacy.StartedAt, legacy.CompletedAt, legacy.Outcome, fingerprint).Scan(&key)
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range legacy.Results {
		var code, message any
		if result.Failure != nil {
			code, message = result.Failure.Code, result.Failure.Message
		}
		_, err = store.pool.Exec(t.Context(), `INSERT INTO argus_catalog.execution_test_results
  (execution_attempt_id,suite_key,test_key,test_outcome,duration_ms,failure_code,failure_message) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			key, result.SuiteKey, result.TestKey, result.Outcome, result.Duration.Milliseconds(), code, message)
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, artifact := range legacy.Artifacts {
		_, err = store.pool.Exec(t.Context(), `INSERT INTO argus_catalog.execution_artifacts (execution_attempt_id,artifact_key,artifact_kind,artifact_uri,artifact_sha256) VALUES ($1,$2,$3,$4,$5)`, key, artifact.Key, artifact.Kind, artifact.URI, artifact.SHA256)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := migrator.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	got, err := store.FindExecutionAttempt(t.Context(), legacy.AttemptID)
	if err != nil || !reflect.DeepEqual(got, execution.CanonicalAttempt(legacy)) {
		t.Fatalf("legacy upgrade: %+v %v", got, err)
	}
	if _, err := store.SaveExecutionAttempt(t.Context(), legacy); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		mode  string
		stage execution.Stage
	}{
		{"staged", execution.StageRelevantNow}, {"staged", execution.StageRemainingLater}, {"shadow", execution.StageShadowControl},
	} {
		attempt := integrationExecutionAttempt(tc.mode+"-"+string(tc.stage), tc.stage)
		attempt.APIVersion = execution.StagedAttemptAPIVersion
		attempt.Staging = &execution.Staging{Schedule: execution.ScheduleReference{APIVersion: execution.ScheduleAPIVersion, SHA256: strings.Repeat("c", 64)}, EpisodeID: "episode", GroupKey: "orders", Environment: "ci", Mode: tc.mode}
		stores := []*integratedStore{store, openTestStore(t, databaseURL)}
		created, failures := make([]bool, 2), make([]error, 2)
		var group sync.WaitGroup
		for i := range stores {
			group.Add(1)
			go func() {
				defer group.Done()
				created[i], failures[i] = stores[i].SaveExecutionAttempt(t.Context(), attempt)
			}()
		}
		group.Wait()
		if failures[0] != nil || failures[1] != nil || created[0] == created[1] {
			t.Fatalf("concurrent retry: %v %v", created, failures)
		}
		got, err := stores[1].FindExecutionAttempt(t.Context(), attempt.AttemptID)
		if err != nil || !reflect.DeepEqual(got, execution.CanonicalAttempt(attempt)) {
			t.Fatalf("readback: %+v %v", got, err)
		}
		conflict := execution.CanonicalAttempt(attempt)
		conflict.Staging.Environment = "other"
		if _, err := store.SaveExecutionAttempt(t.Context(), conflict); !errors.Is(err, execution.ErrConflict) {
			t.Fatalf("provenance conflict: %v", err)
		}
		_, err = store.pool.Exec(t.Context(), `UPDATE argus_catalog.execution_attempts SET schedule_sha256 = NULL WHERE attempt_id=$1`, attempt.AttemptID)
		var constraint *pgconn.PgError
		if !errors.As(err, &constraint) || constraint.Code != "23514" {
			t.Fatalf("missing provenance accepted: %v", err)
		}
	}
	if _, err := store.FindExecutionAttempt(t.Context(), legacy.AttemptID); err != nil {
		t.Fatal(err)
	}
}
