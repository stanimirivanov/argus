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
	if len(migrations) != 16 {
		t.Fatalf("migration count = %d, want 16", len(migrations))
	}
	wantNames := []string{
		"20260917052718_create_catalog.sql",
		"20260918045925_add_impact_evidence.sql",
		"20260918114040_add_change_ingestion.sql",
		"20260918154352_add_openapi_capability_impact.sql",
		"20260926021536_add_execution_attempts.sql",
		"20260927180257_add_review_outcomes.sql",
		"20260928121500_add_validation_rejections.sql",
		"20261015120000_add_composite_file_impact.sql",
		"20261016141106_allow_reviewed_execution_manifest.sql",
		"20261018063250_add_browser_discovery.sql",
		"20261018094119_add_browser_attempts.sql",
		"20261019122008_add_browser_locator_validations.sql",
		"20261021211703_add_predictive_api_stages.sql",
		"20261022062238_add_predictive_browser_stages.sql",
		"20261023123421_add_performance_catalog.sql",
		"20261025093826_add_performance_request_intents.sql",
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
