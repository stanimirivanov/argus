//go:build integration

package postgres

import (
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stanimirivanov/argus/internal/adaptation"
)

func TestReviewOutcomeRoundTripRetryConflictAndRestart(t *testing.T) {
	databaseURL := newTestDatabase(t)
	migrateTestDatabase(t, databaseURL)
	store := openTestStore(t, databaseURL)
	reviewOutcome := loadReviewOutcomeFixture(t)

	if _, err := store.FindReviewOutcome(
		t.Context(), reviewOutcome.OutcomeID,
	); !errors.Is(err, adaptation.ErrOutcomeNotFound) {
		t.Fatalf("find missing review outcome = %v, want ErrOutcomeNotFound", err)
	}
	created, err := store.SaveReviewOutcome(t.Context(), reviewOutcome)
	if err != nil || !created {
		t.Fatalf("save review outcome: created=%t err=%v", created, err)
	}
	retry := reviewOutcome
	retry.ObservedAt = retry.ObservedAt.Add(time.Minute)
	created, err = store.SaveReviewOutcome(t.Context(), retry)
	if err != nil || created {
		t.Fatalf("retry review outcome: created=%t err=%v", created, err)
	}

	want := adaptation.CanonicalReviewOutcome(reviewOutcome)
	got, err := store.FindReviewOutcome(t.Context(), reviewOutcome.OutcomeID)
	if err != nil {
		t.Fatalf("find review outcome: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("review outcome round trip differs:\ngot:  %#v\nwant: %#v", got, want)
	}

	conflict := reviewOutcome
	conflict.ReasonNote = "A different immutable reviewer explanation."
	conflict.OutcomeID = adaptation.DeriveReviewOutcomeID(conflict)
	if _, err := store.SaveReviewOutcome(
		t.Context(), conflict,
	); !errors.Is(err, adaptation.ErrOutcomeConflict) {
		t.Fatalf("save conflicting review outcome = %v, want ErrOutcomeConflict", err)
	}

	store.Close()
	reopened, err := openIntegratedStore(t.Context(), databaseURL)
	if err != nil {
		t.Fatalf("reopen review outcome store: %v", err)
	}
	t.Cleanup(reopened.Close)
	if _, err := reopened.FindReviewOutcome(t.Context(), reviewOutcome.OutcomeID); err != nil {
		t.Fatalf("find review outcome after restart: %v", err)
	}
}

func TestConcurrentReviewOutcomeRetryCreatesOnce(t *testing.T) {
	databaseURL := newTestDatabase(t)
	migrateTestDatabase(t, databaseURL)
	stores := []*integratedStore{openTestStore(t, databaseURL), openTestStore(t, databaseURL)}
	reviewOutcome := loadReviewOutcomeFixture(t)

	createdByStore := make([]bool, len(stores))
	errorsByStore := make([]error, len(stores))
	var waitGroup sync.WaitGroup
	for index, store := range stores {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			createdByStore[index], errorsByStore[index] = store.SaveReviewOutcome(
				t.Context(), reviewOutcome,
			)
		}()
	}
	waitGroup.Wait()

	createdCount := 0
	for index, err := range errorsByStore {
		if err != nil {
			t.Fatalf("concurrent review outcome save %d: %v", index, err)
		}
		if createdByStore[index] {
			createdCount++
		}
	}
	if createdCount != 1 {
		t.Fatalf("created count = %d, want 1", createdCount)
	}
}

func TestReviewOutcomeConstraintsRejectInvalidChildEvidence(t *testing.T) {
	databaseURL := newTestDatabase(t)
	migrateTestDatabase(t, databaseURL)
	store := openTestStore(t, databaseURL)
	reviewOutcome := loadReviewOutcomeFixture(t)
	if _, err := store.SaveReviewOutcome(t.Context(), reviewOutcome); err != nil {
		t.Fatalf("save prerequisite review outcome: %v", err)
	}

	_, err := store.pool.Exec(t.Context(), `
		INSERT INTO argus_catalog.adaptation_review_edits (
			review_outcome_id, path, edit_kind, additions, deletions, patch
		)
		SELECT review_outcome_id, '/absolute/path', 'modified', 1, 0, 'patch'
		FROM argus_catalog.adaptation_review_outcomes
		WHERE outcome_id = $1
	`, reviewOutcome.OutcomeID)
	if err == nil {
		t.Fatal("expected invalid review edit row to violate a database constraint")
	}
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) || postgresError.Code != "23514" {
		t.Fatalf("invalid review edit error = %v, want SQLSTATE 23514", err)
	}
}

func TestValidationRejectionRoundTripRetryConflictAndRestart(t *testing.T) {
	databaseURL := newTestDatabase(t)
	migrateTestDatabase(t, databaseURL)
	store := openTestStore(t, databaseURL)
	evidence := loadValidationRejectionFixture(t)

	if _, err := store.FindValidationRejection(
		t.Context(), evidence.ValidationID,
	); !errors.Is(err, adaptation.ErrValidationRejectionNotFound) {
		t.Fatalf("find missing validation rejection = %v", err)
	}
	created, err := store.SaveValidationRejection(t.Context(), evidence)
	if err != nil || !created {
		t.Fatalf("save validation rejection: created=%t err=%v", created, err)
	}
	created, err = store.SaveValidationRejection(t.Context(), evidence)
	if err != nil || created {
		t.Fatalf("retry validation rejection: created=%t err=%v", created, err)
	}
	got, err := store.FindValidationRejection(t.Context(), evidence.ValidationID)
	if err != nil {
		t.Fatalf("find validation rejection: %v", err)
	}
	if !reflect.DeepEqual(got, adaptation.CanonicalValidationRejectionEvidence(evidence)) {
		t.Fatalf("validation rejection round trip differs:\ngot:  %#v\nwant: %#v", got, evidence)
	}

	conflict := evidence
	conflict.Runs[1].Failure.Message = "Different immutable diagnostic"
	if _, err := store.SaveValidationRejection(
		t.Context(), conflict,
	); !errors.Is(err, adaptation.ErrValidationRejectionConflict) {
		t.Fatalf("save conflicting validation rejection = %v", err)
	}

	store.Close()
	reopened, err := openIntegratedStore(t.Context(), databaseURL)
	if err != nil {
		t.Fatalf("reopen validation rejection store: %v", err)
	}
	t.Cleanup(reopened.Close)
	if _, err := reopened.FindValidationRejection(t.Context(), evidence.ValidationID); err != nil {
		t.Fatalf("find validation rejection after restart: %v", err)
	}
}
