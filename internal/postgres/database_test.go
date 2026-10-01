package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stanimirivanov/argus/internal/adaptation"
	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
	"github.com/stanimirivanov/argus/internal/change/ingest"
	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/execution/attempts"
)

func TestCapabilityStoresDoNotExposeOtherCapabilityPorts(t *testing.T) {
	t.Parallel()
	if _, ok := any(&CatalogStore{}).(ingest.Store); ok {
		t.Fatal("catalog store exposes change ingestion")
	}
	if _, ok := any(&ChangeStore{}).(attempts.Store); ok {
		t.Fatal("change store exposes execution attempts")
	}
	if _, ok := any(&ExecutionStore{}).(ingest.Store); ok {
		t.Fatal("execution store exposes change ingestion")
	}
	if _, ok := any(&AdaptationStore{}).(attempts.Store); ok {
		t.Fatal("adaptation store exposes execution attempts")
	}
}

func TestCapabilityDatabaseErrorsRetainOwningSentinels(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name        string
		classify    func(error) error
		notFound    error
		unavailable error
	}{
		{"change", classifyChangeDatabaseError, change.ErrNotFound, change.ErrUnavailable},
		{"execution", classifyExecutionDatabaseError, execution.ErrNotFound, execution.ErrUnavailable},
		{"adaptation", classifyAdaptationDatabaseError, adaptation.ErrOutcomeNotFound, adaptation.ErrUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := test.classify(pgx.ErrNoRows); !errors.Is(err, test.notFound) {
				t.Fatalf("missing row = %v, want %v", err, test.notFound)
			}
			if err := test.classify(&pgconn.PgError{Code: "08006"}); !errors.Is(err, test.unavailable) {
				t.Fatalf("connection failure = %v, want %v", err, test.unavailable)
			}
		})
	}
}

func TestClassifyDatabaseErrorDoesNotExposeServerMessage(t *testing.T) {
	t.Parallel()

	classified := classifyDatabaseError(&pgconn.PgError{
		Code:    "23505",
		Message: "secret row value",
	})
	if classified.Error() != "catalog database operation failed (SQLSTATE 23505)" {
		t.Fatalf("classified error = %q", classified)
	}
	if errors.Is(classified, catalog.ErrUnavailable) {
		t.Fatal("integrity violation should not be classified as unavailable")
	}
}

func TestClassifyDatabaseErrorPreservesStableSentinels(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		err  error
		want error
	}{
		{name: "not found", err: pgx.ErrNoRows, want: catalog.ErrNotFound},
		{name: "canceled", err: context.Canceled, want: context.Canceled},
		{name: "deadline", err: context.DeadlineExceeded, want: context.DeadlineExceeded},
		{name: "connection", err: &pgconn.PgError{Code: "08006"}, want: catalog.ErrUnavailable},
		{name: "malformed state", err: &pgconn.PgError{Code: "no"}, want: catalog.ErrUnavailable},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if classified := classifyDatabaseError(test.err); !errors.Is(classified, test.want) {
				t.Fatalf("classifyDatabaseError() = %v, want %v", classified, test.want)
			}
		})
	}
}
