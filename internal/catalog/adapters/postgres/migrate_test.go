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
	if len(migrations) != 3 {
		t.Fatalf("migration count = %d, want 3", len(migrations))
	}
	wantNames := []string{
		"20260917052718_create_catalog.sql",
		"20260918045925_add_impact_evidence.sql",
		"20260918114040_add_change_ingestion.sql",
	}
	for index, migration := range migrations {
		if migration.name != wantNames[index] {
			t.Fatalf("migration %d name = %q, want %q", index, migration.name, wantNames[index])
		}
		if len(migration.sha256) != 64 {
			t.Fatalf("migration %d checksum length = %d, want 64", index, len(migration.sha256))
		}
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
