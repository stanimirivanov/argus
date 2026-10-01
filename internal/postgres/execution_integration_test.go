//go:build integration

package postgres

import (
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/execution/shadow"
)

func TestExecutionAttemptRoundTripRetryConflictShadowAndRestart(t *testing.T) {
	databaseURL := newTestDatabase(t)
	migrateTestDatabase(t, databaseURL)
	store := openTestStore(t, databaseURL)
	selected := integrationExecutionAttempt("attempt-selected", execution.StageSelected)
	fullSuite := integrationExecutionAttempt("attempt-full", execution.StageFullSuite)
	fullSuite.Results = append(fullSuite.Results, execution.TestResult{
		SuiteKey: "orders", TestKey: "delete", Outcome: execution.TestFailed,
		Duration: 4 * time.Second,
		Failure:  &execution.Failure{Code: "assertion", Message: "expected 204"},
	})
	fullSuite.Outcome = execution.DeriveAttemptOutcome(fullSuite.Results)

	if _, err := store.FindExecutionAttempt(t.Context(), selected.AttemptID); !errors.Is(err, execution.ErrNotFound) {
		t.Fatalf("find missing attempt = %v, want ErrNotFound", err)
	}
	created, err := store.SaveExecutionAttempt(t.Context(), selected)
	if err != nil || !created {
		t.Fatalf("save selected attempt: created=%t err=%v", created, err)
	}
	created, err = store.SaveExecutionAttempt(t.Context(), execution.CanonicalAttempt(selected))
	if err != nil || created {
		t.Fatalf("retry selected attempt: created=%t err=%v", created, err)
	}
	if created, err = store.SaveExecutionAttempt(t.Context(), fullSuite); err != nil || !created {
		t.Fatalf("save full-suite attempt: created=%t err=%v", created, err)
	}

	want := execution.CanonicalAttempt(selected)
	got, err := store.FindExecutionAttempt(t.Context(), selected.AttemptID)
	if err != nil {
		t.Fatalf("find selected attempt: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("attempt round trip differs:\ngot:  %#v\nwant: %#v", got, want)
	}
	report, err := shadow.NewService(store).Compare(t.Context(), selected.AttemptID, fullSuite.AttemptID)
	if err != nil {
		t.Fatalf("compare shadow attempts: %v", err)
	}
	if report.FullSuiteFailureCount != 2 || report.CaughtFullSuiteFailureCount != 1 ||
		report.FailureRecallBasisPoints == nil || *report.FailureRecallBasisPoints != 5000 ||
		len(report.MissedFailures) != 1 || report.MissedFailures[0].Reason != shadow.MissNotSelected {
		t.Fatalf("unexpected shadow report: %#v", report)
	}

	conflict := selected
	conflict.Results = append([]execution.TestResult{}, selected.Results...)
	conflict.Results[0].Duration += time.Millisecond
	if _, err := store.SaveExecutionAttempt(t.Context(), conflict); !errors.Is(err, execution.ErrConflict) {
		t.Fatalf("save conflicting attempt = %v, want ErrConflict", err)
	}

	store.Close()
	reopened, err := openIntegratedStore(t.Context(), databaseURL)
	if err != nil {
		t.Fatalf("reopen execution store: %v", err)
	}
	t.Cleanup(reopened.Close)
	if _, err := reopened.FindExecutionAttempt(t.Context(), selected.AttemptID); err != nil {
		t.Fatalf("find execution attempt after restart: %v", err)
	}
}

func TestConcurrentExecutionAttemptRetryCreatesOnce(t *testing.T) {
	databaseURL := newTestDatabase(t)
	migrateTestDatabase(t, databaseURL)
	stores := []*integratedStore{openTestStore(t, databaseURL), openTestStore(t, databaseURL)}
	attempt := integrationExecutionAttempt("attempt-concurrent", execution.StageSelected)

	createdByStore := make([]bool, len(stores))
	errorsByStore := make([]error, len(stores))
	var waitGroup sync.WaitGroup
	for index, store := range stores {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			createdByStore[index], errorsByStore[index] = store.SaveExecutionAttempt(t.Context(), attempt)
		}()
	}
	waitGroup.Wait()

	createdCount := 0
	for index, err := range errorsByStore {
		if err != nil {
			t.Fatalf("concurrent attempt save %d: %v", index, err)
		}
		if createdByStore[index] {
			createdCount++
		}
	}
	if createdCount != 1 {
		t.Fatalf("created count = %d, want 1", createdCount)
	}
}

func TestExecutionAttemptConstraintsRejectInvalidChildEvidence(t *testing.T) {
	databaseURL := newTestDatabase(t)
	migrateTestDatabase(t, databaseURL)
	store := openTestStore(t, databaseURL)
	attempt := integrationExecutionAttempt("attempt-constraints", execution.StageSelected)
	if _, err := store.SaveExecutionAttempt(t.Context(), attempt); err != nil {
		t.Fatalf("save prerequisite attempt: %v", err)
	}

	_, err := store.pool.Exec(t.Context(), `
		INSERT INTO argus_catalog.execution_test_results (
			execution_attempt_id, suite_key, test_key, test_outcome, duration_ms
		)
		SELECT execution_attempt_id, 'INVALID', 'invalid', 'passed', 1
		FROM argus_catalog.execution_attempts
		WHERE attempt_id = $1
	`, attempt.AttemptID)
	if err == nil {
		t.Fatal("expected invalid execution child row to violate a database constraint")
	}
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) || postgresError.Code != "23514" {
		t.Fatalf("invalid execution child error = %v, want SQLSTATE 23514", err)
	}
}

func integrationExecutionAttempt(attemptID string, stage execution.Stage) execution.Attempt {
	return execution.Attempt{
		APIVersion: execution.AttemptAPIVersion,
		AttemptID:  attemptID,
		Manifest: execution.ManifestReference{
			APIVersion: "argus.dev/execution-manifest/v1", SHA256: strings.Repeat("a", 64),
		},
		Stage: stage,
		TestRepository: catalog.Repository{
			Identity: catalog.RepositoryIdentity{
				Provider: catalog.ProviderGitHub, Host: "github.com",
				ProviderRepositoryID: "integration-tests-42",
			},
			Owner: "example", Name: "orders-tests",
		},
		TestRevision: catalog.Revision{
			Algorithm: catalog.RevisionGitSHA1,
			Digest:    "0123456789abcdef0123456789abcdef01234567",
		},
		AdapterID: "rest-assured", AdapterVersion: "1.2.3",
		StartedAt:   time.Date(2026, 9, 26, 8, 0, 0, 0, time.UTC),
		CompletedAt: time.Date(2026, 9, 26, 8, 0, 3, 0, time.UTC),
		Outcome:     execution.AttemptFailed,
		Results: []execution.TestResult{{
			SuiteKey: "orders", TestKey: "create", Outcome: execution.TestFailed,
			Duration: 3 * time.Second,
			Failure:  &execution.Failure{Code: "assertion", Message: "expected 201"},
		}},
		Artifacts: []execution.ArtifactReference{{
			Key: "junit", Kind: "junit-xml", URI: "s3://argus/attempts/junit.xml",
			SHA256: strings.Repeat("b", 64),
		}},
	}
}
