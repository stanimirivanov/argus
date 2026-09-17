package postgres

import (
	"io/fs"
	"testing"
	"testing/fstest"
)

func TestEmbeddedMigrationChainLoadsInFilenameOrder(t *testing.T) {
	t.Parallel()

	migrations, err := loadMigrations(embeddedMigrations)
	if err != nil {
		t.Fatalf("load embedded migrations: %v", err)
	}
	if len(migrations) != 1 {
		t.Fatalf("migration count = %d, want 1", len(migrations))
	}
	if migrations[0].name != "20260917052718_create_catalog.sql" {
		t.Fatalf("migration name = %q", migrations[0].name)
	}
	if len(migrations[0].sha256) != 64 {
		t.Fatalf("migration checksum length = %d, want 64", len(migrations[0].sha256))
	}
}

func TestLoadMigrationsRejectsInvalidFilename(t *testing.T) {
	t.Parallel()

	migrationFS := fstest.MapFS{
		"migrations/not_timestamped.sql": &fstest.MapFile{Data: []byte("SELECT 1;")},
	}
	if _, err := loadMigrations(migrationFS); err == nil {
		t.Fatal("expected invalid migration filename to fail")
	}
}

func TestLoadMigrationsRejectsEmptyChain(t *testing.T) {
	t.Parallel()

	migrationFS := fstest.MapFS{
		"migrations": &fstest.MapFile{Mode: fs.ModeDir},
	}
	if _, err := loadMigrations(migrationFS); err == nil {
		t.Fatal("expected empty migration chain to fail")
	}
}
