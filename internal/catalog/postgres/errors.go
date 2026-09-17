package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stanimirivanov/argus/internal/catalog"
)

// classifyDatabaseError is the adapter's redaction and retry-classification
// boundary. Raw server messages and connection details never cross it.
func classifyDatabaseError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return catalog.ErrNotFound
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}

	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		if !validSQLState(postgresError.Code) {
			return catalog.ErrUnavailable
		}
		if strings.HasPrefix(postgresError.Code, "08") ||
			strings.HasPrefix(postgresError.Code, "53") ||
			strings.HasPrefix(postgresError.Code, "57") ||
			postgresError.Code == "40001" ||
			postgresError.Code == "40P01" ||
			postgresError.Code == "55P03" {
			return catalog.ErrUnavailable
		}

		return &databaseError{sqlState: postgresError.Code}
	}

	return catalog.ErrUnavailable
}

type databaseError struct {
	sqlState string
}

func (err *databaseError) Error() string {
	return fmt.Sprintf("catalog database operation failed (SQLSTATE %s)", err.sqlState)
}

func validSQLState(code string) bool {
	if len(code) != 5 {
		return false
	}
	for _, character := range code {
		if (character < '0' || character > '9') &&
			(character < 'A' || character > 'Z') {
			return false
		}
	}

	return true
}
