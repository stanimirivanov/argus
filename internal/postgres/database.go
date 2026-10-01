package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stanimirivanov/argus/internal/adaptation/outcome"
	"github.com/stanimirivanov/argus/internal/catalog/impact"
	"github.com/stanimirivanov/argus/internal/catalog/snapshot"
	"github.com/stanimirivanov/argus/internal/catalog/testquery"
	changeimpact "github.com/stanimirivanov/argus/internal/change/impact"
	"github.com/stanimirivanov/argus/internal/change/ingest"
	"github.com/stanimirivanov/argus/internal/execution/attempts"
	"github.com/stanimirivanov/argus/internal/execution/shadow"
)

const (
	operationTimeout       = 15 * time.Second
	runtimeMaxConnections  = 16
	migratorMaxConnections = 1
)

// Runtime owns one bounded connection pool shared by capability-scoped stores.
// Obtain it with OpenRuntime; its zero value is invalid. Opening it never runs
// migrations, and the composition root must close it after its views stop use.
type Runtime struct {
	pool *pgxpool.Pool
}

// CatalogStore owns catalog snapshots, test queries, and impact-edge evidence.
// Obtain it from Runtime.Catalog; its zero value is invalid.
type CatalogStore struct{ runtime *Runtime }

// ChangeStore owns delivery and semantic capability-impact evidence.
// Obtain it from Runtime.Change; its zero value is invalid.
type ChangeStore struct{ runtime *Runtime }

// ExecutionStore owns immutable execution attempts and shadow reads.
// Obtain it from Runtime.Execution; its zero value is invalid.
type ExecutionStore struct{ runtime *Runtime }

// AdaptationStore owns terminal review and validation-rejection evidence.
// Obtain it from Runtime.Adaptation; its zero value is invalid.
type AdaptationStore struct{ runtime *Runtime }

var _ snapshot.Store = (*CatalogStore)(nil)
var _ testquery.TestCatalogReader = (*CatalogStore)(nil)
var _ impact.EvidenceStore = (*CatalogStore)(nil)
var _ impact.EdgeReader = (*CatalogStore)(nil)
var _ ingest.Store = (*ChangeStore)(nil)
var _ changeimpact.Store = (*ChangeStore)(nil)
var _ attempts.Store = (*ExecutionStore)(nil)
var _ shadow.AttemptReader = (*ExecutionStore)(nil)
var _ outcome.EvidenceStore = (*AdaptationStore)(nil)

// OpenRuntime validates the secret database configuration, establishes a bounded
// connection pool, and verifies connectivity without changing schema state.
func OpenRuntime(ctx context.Context, databaseURL string) (*Runtime, error) {
	// Preserve the existing application_name during this behavior-preserving
	// ownership refactor so database monitoring does not silently split series.
	pool, err := openPool(ctx, databaseURL, "argus-catalog", runtimeMaxConnections)
	if err != nil {
		return nil, err
	}

	return &Runtime{pool: pool}, nil
}

// Catalog returns the catalog-only view of this runtime's pool.
func (runtime *Runtime) Catalog() *CatalogStore { return &CatalogStore{runtime: runtime} }

// Change returns the change-only view of this runtime's pool.
func (runtime *Runtime) Change() *ChangeStore { return &ChangeStore{runtime: runtime} }

// Execution returns the execution-only view of this runtime's pool.
func (runtime *Runtime) Execution() *ExecutionStore { return &ExecutionStore{runtime: runtime} }

// Adaptation returns the adaptation-only view of this runtime's pool.
func (runtime *Runtime) Adaptation() *AdaptationStore { return &AdaptationStore{runtime: runtime} }

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

// Close releases all database connections owned by the Runtime.
func (runtime *Runtime) Close() {
	runtime.pool.Close()
}

// beginWrite establishes the durability and lock-wait policy shared by runtime
// writes. The returned context owns the complete transaction deadline.
func (runtime *Runtime) beginWrite(ctx context.Context) (pgx.Tx, context.Context, context.CancelFunc, error) {
	operationContext, cancel := context.WithTimeout(ctx, operationTimeout)
	tx, err := runtime.pool.BeginTx(operationContext, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
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
