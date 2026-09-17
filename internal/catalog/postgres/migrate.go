package postgres

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"regexp"

	"github.com/jackc/pgx/v5"
)

const migrationAdvisoryLock int64 = 418531940723

var migrationNamePattern = regexp.MustCompile(`^[0-9]{14}_[a-z][a-z0-9_]*\.sql$`)

//go:embed migrations/*.sql
var embeddedMigrations embed.FS

type migration struct {
	name     string
	contents []byte
	sha256   string
}

// Migrate applies the complete embedded forward migration chain explicitly.
// The chain and checksum ledger commit atomically under an advisory lock.
func (store *Store) Migrate(ctx context.Context) error {
	return migrate(ctx, store, embeddedMigrations)
}

func migrate(ctx context.Context, store *Store, migrationFS fs.FS) error {
	migrations, err := loadMigrations(migrationFS)
	if err != nil {
		return err
	}

	operationContext, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()

	tx, err := store.pool.BeginTx(operationContext, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return classifyDatabaseError(err)
	}
	defer func() {
		_ = tx.Rollback(operationContext) //nolint:errcheck // Best effort after commit or failure.
	}()

	if err := prepareMigrationLedger(operationContext, tx); err != nil {
		return err
	}

	appliedCount, err := validateMigrationLedger(operationContext, tx, migrations)
	if err != nil {
		return err
	}
	for _, pending := range migrations[appliedCount:] {
		if _, err := tx.Exec(operationContext, string(pending.contents)); err != nil {
			return fmt.Errorf("apply migration %s: %w", pending.name, classifyDatabaseError(err))
		}
		if _, err := tx.Exec(
			operationContext,
			"INSERT INTO argus_catalog.schema_migrations(version, sha256) VALUES ($1, $2)",
			pending.name,
			pending.sha256,
		); err != nil {
			return classifyDatabaseError(err)
		}
	}

	return classifyDatabaseError(tx.Commit(operationContext))
}

func loadMigrations(migrationFS fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read embedded migrations: %w", err)
	}
	if len(entries) == 0 {
		return nil, errors.New("no catalog migrations are embedded")
	}

	migrations := make([]migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !migrationNamePattern.MatchString(entry.Name()) {
			return nil, fmt.Errorf("invalid catalog migration filename %q", entry.Name())
		}

		contents, err := fs.ReadFile(migrationFS, "migrations/"+entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}
		digest := sha256.Sum256(contents)
		migrations = append(migrations, migration{
			name:     entry.Name(),
			contents: contents,
			sha256:   hex.EncodeToString(digest[:]),
		})
	}

	return migrations, nil
}

func prepareMigrationLedger(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, "SET LOCAL synchronous_commit = on; SET LOCAL lock_timeout = '15s'"); err != nil {
		return classifyDatabaseError(err)
	}
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", migrationAdvisoryLock); err != nil {
		return classifyDatabaseError(err)
	}
	if _, err := tx.Exec(ctx, `
		CREATE SCHEMA IF NOT EXISTS argus_catalog;
		CREATE TABLE IF NOT EXISTS argus_catalog.schema_migrations (
			version TEXT NOT NULL,
			sha256 TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT statement_timestamp(),
			CONSTRAINT pk_schema_migrations PRIMARY KEY (version),
			CONSTRAINT ck_schema_migrations_version
				CHECK (version ~ '^[0-9]{14}_[a-z][a-z0-9_]*[.]sql$'),
			CONSTRAINT ck_schema_migrations_sha256
				CHECK (sha256 ~ '^[0-9a-f]{64}$')
		)
	`); err != nil {
		return classifyDatabaseError(err)
	}

	return nil
}

func validateMigrationLedger(ctx context.Context, tx pgx.Tx, expected []migration) (int, error) {
	rows, err := tx.Query(
		ctx,
		"SELECT version, sha256 FROM argus_catalog.schema_migrations ORDER BY version",
	)
	if err != nil {
		return 0, classifyDatabaseError(err)
	}
	defer rows.Close()

	appliedCount := 0
	for rows.Next() {
		var version string
		var checksum string
		if err := rows.Scan(&version, &checksum); err != nil {
			return 0, classifyDatabaseError(err)
		}
		if appliedCount >= len(expected) ||
			version != expected[appliedCount].name ||
			checksum != expected[appliedCount].sha256 {
			return 0, ErrMigrationDrift
		}

		appliedCount++
	}
	if err := rows.Err(); err != nil {
		return 0, classifyDatabaseError(err)
	}

	return appliedCount, nil
}
