package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stanimirivanov/argus/internal/catalog"
)

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
