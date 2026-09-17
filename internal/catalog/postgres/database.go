// Package postgres persists immutable catalog snapshots in PostgreSQL.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	operationTimeout = 15 * time.Second
	maxConnections   = 16
)

var (
	// ErrNotFound means the requested immutable catalog snapshot does not exist.
	ErrNotFound = errors.New("catalog snapshot not found")
	// ErrConflict means an immutable catalog identity is already bound to
	// different content.
	ErrConflict = errors.New("catalog snapshot identity conflict")
	// ErrUnavailable means a transient or ambiguous database failure prevented a
	// trustworthy outcome.
	ErrUnavailable = errors.New("catalog database unavailable")
	// ErrMigrationDrift means the applied migration ledger is not a prefix of the
	// migrations embedded in this binary.
	ErrMigrationDrift = errors.New("catalog migration history drift")
)

// Store is the bounded PostgreSQL catalog adapter. Opening a Store does not run
// migrations. Its owner must call Close during shutdown.
type Store struct {
	pool *pgxpool.Pool
}

// Open validates the secret database configuration, establishes a bounded
// connection pool, and verifies connectivity without changing schema state.
func Open(ctx context.Context, databaseURL string) (*Store, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return nil, errors.New("database connection configuration is required")
	}

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, errors.New("invalid database connection configuration")
	}
	config.MaxConns = maxConnections
	config.MinConns = 0
	config.MaxConnLifetime = 30 * time.Minute
	config.ConnConfig.RuntimeParams["application_name"] = "argus-catalog"

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, errors.New("open catalog database")
	}

	operationContext, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	if err := pool.Ping(operationContext); err != nil {
		pool.Close()

		return nil, classifyDatabaseError(err)
	}

	return &Store{pool: pool}, nil
}

// Close releases all database connections owned by the Store.
func (store *Store) Close() {
	store.pool.Close()
}

func (store *Store) beginWrite(ctx context.Context) (pgx.Tx, context.Context, context.CancelFunc, error) {
	operationContext, cancel := context.WithTimeout(ctx, operationTimeout)
	tx, err := store.pool.BeginTx(operationContext, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		cancel()

		return nil, nil, nil, classifyDatabaseError(err)
	}
	if _, err := tx.Exec(
		operationContext,
		"SET LOCAL synchronous_commit = on; SET LOCAL lock_timeout = '5s'",
	); err != nil {
		_ = tx.Rollback(operationContext) //nolint:errcheck // Preserve the configuration error.
		cancel()

		return nil, nil, nil, classifyDatabaseError(err)
	}

	return tx, operationContext, cancel, nil
}

func classifyDatabaseError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
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
			return ErrUnavailable
		}
		if strings.HasPrefix(postgresError.Code, "08") ||
			strings.HasPrefix(postgresError.Code, "53") ||
			strings.HasPrefix(postgresError.Code, "57") ||
			postgresError.Code == "40001" ||
			postgresError.Code == "40P01" ||
			postgresError.Code == "55P03" {
			return ErrUnavailable
		}

		return &databaseError{sqlState: postgresError.Code}
	}

	return ErrUnavailable
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
