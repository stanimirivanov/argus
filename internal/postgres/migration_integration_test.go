//go:build integration

package postgres

import (
	"errors"
	"io/fs"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5"
)

func TestDatabaseURLTargetsDisposableDatabase(t *testing.T) {
	t.Parallel()

	databaseURL, err := databaseURLForName(
		"postgres://postgres:test@127.0.0.1:5432/postgres?sslmode=disable",
		"argus_test_0123456789abcdef",
	)
	if err != nil {
		t.Fatalf("derive disposable database URL: %v", err)
	}
	config, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse disposable database URL: %v", err)
	}
	if config.Database != "argus_test_0123456789abcdef" {
		t.Fatalf("database = %q, want disposable database", config.Database)
	}
	if config.Host != "127.0.0.1" || config.Port != 5432 {
		t.Fatalf("server address changed: host=%q port=%d", config.Host, config.Port)
	}
	if config.Config.TLSConfig != nil {
		t.Fatal("sslmode query parameter was not preserved")
	}
}

func TestDatabaseURLRejectsNonURLConfiguration(t *testing.T) {
	t.Parallel()

	if _, err := databaseURLForName("host=localhost dbname=postgres", "argus_test_01"); err == nil {
		t.Fatal("expected keyword connection configuration to be rejected")
	}
}

func TestMigrateEmptyDatabaseAndRepeat(t *testing.T) {
	databaseURL := newTestDatabase(t)
	migrator := openTestMigrator(t, databaseURL)

	if err := migrator.Migrate(t.Context()); err != nil {
		t.Fatalf("migrate empty database: %v", err)
	}
	if err := migrator.Migrate(t.Context()); err != nil {
		t.Fatalf("repeat migration: %v", err)
	}

	var count int
	if err := migrator.pool.QueryRow(
		t.Context(),
		"SELECT count(*) FROM argus_catalog.schema_migrations",
	).Scan(&count); err != nil {
		t.Fatalf("count migration ledger: %v", err)
	}
	if count != 7 {
		t.Fatalf("migration ledger count = %d, want 7", count)
	}
}

func TestMigrationUpgradesReleasedCatalogWithRepresentativeData(t *testing.T) {
	databaseURL := newTestDatabase(t)
	migrator := openTestMigrator(t, databaseURL)
	firstMigration, err := fs.ReadFile(
		embeddedMigrations,
		"migrations/20260917052718_create_catalog.sql",
	)
	if err != nil {
		t.Fatalf("read released catalog migration: %v", err)
	}
	releasedChain := fstest.MapFS{
		"migrations/20260917052718_create_catalog.sql": &fstest.MapFile{Data: firstMigration},
	}
	if err := applyMigrationChain(t.Context(), migrator, releasedChain); err != nil {
		t.Fatalf("apply released catalog schema: %v", err)
	}

	store := openTestStore(t, databaseURL)
	snapshot := loadTestSnapshot(t)
	if _, err := store.SaveSnapshot(t.Context(), snapshot); err != nil {
		t.Fatalf("save representative released-schema data: %v", err)
	}
	if err := migrator.Migrate(t.Context()); err != nil {
		t.Fatalf("upgrade released catalog schema: %v", err)
	}
	if _, err := store.GetSnapshot(t.Context(), snapshot.Key()); err != nil {
		t.Fatalf("read representative data after upgrade: %v", err)
	}

	var migrationCount int
	if err := store.pool.QueryRow(
		t.Context(),
		"SELECT count(*) FROM argus_catalog.schema_migrations",
	).Scan(&migrationCount); err != nil {
		t.Fatalf("count upgraded migration ledger: %v", err)
	}
	if migrationCount != 7 {
		t.Fatalf("upgraded migration count = %d, want 7", migrationCount)
	}
}

func TestConcurrentMigrationsSerialize(t *testing.T) {
	databaseURL := newTestDatabase(t)
	migrators := []*Migrator{openTestMigrator(t, databaseURL), openTestMigrator(t, databaseURL)}

	errorsByMigrator := make([]error, len(migrators))
	var waitGroup sync.WaitGroup
	for index, migrator := range migrators {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			errorsByMigrator[index] = migrator.Migrate(t.Context())
		}()
	}
	waitGroup.Wait()

	for index, err := range errorsByMigrator {
		if err != nil {
			t.Fatalf("concurrent migration %d: %v", index, err)
		}
	}
}

func TestMigrationRejectsChangedChecksum(t *testing.T) {
	databaseURL := newTestDatabase(t)
	migrator := migrateTestDatabase(t, databaseURL)
	store := openTestStore(t, databaseURL)

	if _, err := store.pool.Exec(t.Context(), `
		UPDATE argus_catalog.schema_migrations
		SET sha256 = $1
	`, strings.Repeat("0", 64)); err != nil {
		t.Fatalf("tamper migration checksum: %v", err)
	}
	if err := migrator.Migrate(t.Context()); !errors.Is(err, ErrMigrationDrift) {
		t.Fatalf("migrate after checksum drift = %v, want ErrMigrationDrift", err)
	}
}

func TestMigrationRejectsUnknownLedgerVersion(t *testing.T) {
	databaseURL := newTestDatabase(t)
	migrator := migrateTestDatabase(t, databaseURL)
	store := openTestStore(t, databaseURL)

	if _, err := store.pool.Exec(t.Context(), `
		INSERT INTO argus_catalog.schema_migrations (version, sha256)
		VALUES ($1, $2)
	`, "20260917052719_unknown.sql", strings.Repeat("0", 64)); err != nil {
		t.Fatalf("insert unknown migration version: %v", err)
	}
	if err := migrator.Migrate(t.Context()); !errors.Is(err, ErrMigrationDrift) {
		t.Fatalf("migrate with unknown ledger version = %v, want ErrMigrationDrift", err)
	}
}

func TestMigrationChainRollsBackAtomically(t *testing.T) {
	databaseURL := newTestDatabase(t)
	migrator := openTestMigrator(t, databaseURL)
	firstMigration, err := fs.ReadFile(
		embeddedMigrations,
		"migrations/20260917052718_create_catalog.sql",
	)
	if err != nil {
		t.Fatalf("read embedded migration: %v", err)
	}
	migrationFS := fstest.MapFS{
		"migrations/20260917052718_create_catalog.sql": &fstest.MapFile{Data: firstMigration},
		"migrations/20260917052719_force_rollback.sql": &fstest.MapFile{
			Data: []byte("SELECT * FROM argus_catalog.table_that_does_not_exist;"),
		},
	}

	if err := applyMigrationChain(t.Context(), migrator, migrationFS); err == nil {
		t.Fatal("expected invalid migration chain to fail")
	}

	var schemaMissing bool
	if err := migrator.pool.QueryRow(
		t.Context(),
		"SELECT to_regnamespace('argus_catalog') IS NULL",
	).Scan(&schemaMissing); err != nil {
		t.Fatalf("inspect rolled-back schema: %v", err)
	}
	if !schemaMissing {
		t.Fatal("failed migration left the catalog schema behind")
	}
}
