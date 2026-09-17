package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stanimirivanov/argus/internal/catalog"
)

const (
	operationTimeout       = 15 * time.Second
	storeMaxConnections    = 16
	migratorMaxConnections = 1
)

// Store is the bounded PostgreSQL catalog adapter. Opening a Store does not run
// migrations. Its owner must call Close during shutdown.
type Store struct {
	pool *pgxpool.Pool
}

var _ catalog.SnapshotStore = (*Store)(nil)
var _ catalog.TestCatalogReader = (*Store)(nil)

// OpenStore validates the secret database configuration, establishes a bounded
// connection pool, and verifies connectivity without changing schema state.
func OpenStore(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := openPool(ctx, databaseURL, "argus-catalog", storeMaxConnections)
	if err != nil {
		return nil, err
	}

	return &Store{pool: pool}, nil
}

func openPool(
	ctx context.Context,
	databaseURL string,
	applicationName string,
	maxConnections int32,
) (*pgxpool.Pool, error) {
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
	config.ConnConfig.RuntimeParams["application_name"] = applicationName

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

	return pool, nil
}

// Close releases all database connections owned by the Store.
func (store *Store) Close() {
	store.pool.Close()
}

// beginWrite establishes the durability and lock-wait policy shared by catalog
// writes. The returned context owns the complete transaction deadline.
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
