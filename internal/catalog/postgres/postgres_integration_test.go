//go:build integration

package postgres

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stanimirivanov/argus/contracts"
	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/catalog/descriptor"
)

const testDatabaseURLEnvironment = "ARGUS_TEST_POSTGRES_URL"

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
	if count != 1 {
		t.Fatalf("migration ledger count = %d, want 1", count)
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

func TestSnapshotRoundTripRetryConflictAndRestart(t *testing.T) {
	databaseURL := newTestDatabase(t)
	migrateTestDatabase(t, databaseURL)
	store := openTestStore(t, databaseURL)
	snapshot := loadTestSnapshot(t)
	if _, err := store.GetSnapshot(t.Context(), snapshot.Key()); !errors.Is(err, catalog.ErrNotFound) {
		t.Fatalf("get missing snapshot = %v, want ErrNotFound", err)
	}

	created, err := store.SaveSnapshot(t.Context(), snapshot)
	if err != nil || !created {
		t.Fatalf("save new snapshot: created=%t err=%v", created, err)
	}
	created, err = store.SaveSnapshot(t.Context(), reorderedSnapshot(snapshot))
	if err != nil || created {
		t.Fatalf("retry reordered snapshot: created=%t err=%v", created, err)
	}

	want := catalog.CanonicalSnapshot(snapshot)
	got, err := store.GetSnapshot(t.Context(), snapshot.Key())
	if err != nil {
		t.Fatalf("get snapshot: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round-trip snapshot differs:\ngot:  %#v\nwant: %#v", got, want)
	}

	conflict := cloneSnapshotForIntegration(snapshot)
	conflict.Capabilities[0].Name = "Changed immutable content"
	if _, err := store.SaveSnapshot(t.Context(), conflict); !errors.Is(err, catalog.ErrConflict) {
		t.Fatalf("save conflicting snapshot = %v, want ErrConflict", err)
	}

	store.Close()
	reopened, err := OpenStore(t.Context(), databaseURL)
	if err != nil {
		t.Fatalf("reopen catalog store: %v", err)
	}
	t.Cleanup(reopened.Close)
	got, err = reopened.GetSnapshot(t.Context(), snapshot.Key())
	if err != nil {
		t.Fatalf("get snapshot after restart: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("snapshot changed after store restart")
	}
}

func TestConcurrentSnapshotRetryCreatesOnce(t *testing.T) {
	databaseURL := newTestDatabase(t)
	migrateTestDatabase(t, databaseURL)
	stores := []*Store{openTestStore(t, databaseURL), openTestStore(t, databaseURL)}
	snapshot := loadTestSnapshot(t)

	createdByStore := make([]bool, len(stores))
	errorsByStore := make([]error, len(stores))
	var waitGroup sync.WaitGroup
	for index, store := range stores {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			createdByStore[index], errorsByStore[index] = store.SaveSnapshot(t.Context(), snapshot)
		}()
	}
	waitGroup.Wait()

	createdCount := 0
	for index, err := range errorsByStore {
		if err != nil {
			t.Fatalf("concurrent save %d: %v", index, err)
		}
		if createdByStore[index] {
			createdCount++
		}
	}
	if createdCount != 1 {
		t.Fatalf("created count = %d, want 1", createdCount)
	}
}

func TestDatabaseConstraintsProtectCapabilityMappings(t *testing.T) {
	store := migratedTestStore(t)
	snapshot := loadTestSnapshot(t)
	if _, err := store.SaveSnapshot(t.Context(), snapshot); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}

	var snapshotID int64
	if err := store.pool.QueryRow(t.Context(), `
		SELECT snapshot_id
		FROM argus_catalog.catalog_snapshots
		WHERE revision_digest = $1
	`, snapshot.Revision.Digest).Scan(&snapshotID); err != nil {
		t.Fatalf("find snapshot: %v", err)
	}
	_, err := store.pool.Exec(t.Context(), `
		INSERT INTO argus_catalog.component_capabilities (
			snapshot_id,
			component_key,
			capability_key
		) VALUES ($1, $2, $3)
	`, snapshotID, snapshot.Components[0].Key, "missing-capability")
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) || postgresError.Code != "23503" {
		t.Fatalf("invalid mapping error = %v, want foreign-key violation", err)
	}
}

func TestTestCatalogQueryPaginatesFiltersAndSurvivesRestart(t *testing.T) {
	databaseURL := newTestDatabase(t)
	migrateTestDatabase(t, databaseURL)
	store := openTestStore(t, databaseURL)
	snapshot := queryableTestSnapshot(t)
	if _, err := store.SaveSnapshot(t.Context(), snapshot); err != nil {
		t.Fatalf("save queryable snapshot: %v", err)
	}

	query := catalog.TestCatalogQuery{Snapshot: snapshot.Key(), PageSize: 2}
	items := collectTestCatalogPages(t, catalog.NewTestCatalogService(store), query)
	if len(items) != 4 {
		t.Fatalf("catalog entry count = %d, want 4", len(items))
	}
	for index := 1; index < len(items); index++ {
		if compareTestCatalogIdentities(items[index-1].Identity(), items[index].Identity()) >= 0 {
			t.Fatalf("catalog entries are not strictly ordered at index %d", index)
		}
	}

	query.CapabilityKey = "cancel-order"
	query.PageSize = 1
	filtered := collectTestCatalogPages(t, catalog.NewTestCatalogService(store), query)
	if len(filtered) != 2 {
		t.Fatalf("filtered catalog entry count = %d, want 2", len(filtered))
	}
	for _, entry := range filtered {
		if !entryHasCapability(entry, query.CapabilityKey) {
			t.Fatalf("filtered entry %#v does not contain %q", entry.Identity(), query.CapabilityKey)
		}
	}

	empty, err := catalog.NewTestCatalogService(store).ListTests(t.Context(), catalog.TestCatalogQuery{
		Snapshot:      snapshot.Key(),
		CapabilityKey: "missing-capability",
		PageSize:      10,
	})
	if err != nil {
		t.Fatalf("list empty capability result: %v", err)
	}
	if len(empty.Items) != 0 || empty.Snapshot.SourceRepository.Identity != snapshot.Repository.Identity {
		t.Fatalf("empty catalog page = %#v", empty)
	}

	missing := snapshot.Key()
	missing.Revision.Digest = "1123456789abcdef0123456789abcdef01234567"
	if _, err := catalog.NewTestCatalogService(store).ListTests(t.Context(), catalog.TestCatalogQuery{
		Snapshot: missing,
		PageSize: 10,
	}); !errors.Is(err, catalog.ErrNotFound) {
		t.Fatalf("list missing snapshot = %v, want ErrNotFound", err)
	}

	store.Close()
	reopened, err := OpenStore(t.Context(), databaseURL)
	if err != nil {
		t.Fatalf("reopen catalog store: %v", err)
	}
	t.Cleanup(reopened.Close)
	afterRestart := collectTestCatalogPages(t, catalog.NewTestCatalogService(reopened), catalog.TestCatalogQuery{
		Snapshot: snapshot.Key(),
		PageSize: 3,
	})
	if !reflect.DeepEqual(afterRestart, items) {
		t.Fatal("catalog query changed after store restart")
	}
}

func collectTestCatalogPages(
	t *testing.T,
	service *catalog.TestCatalogService,
	query catalog.TestCatalogQuery,
) []catalog.TestCatalogEntry {
	t.Helper()

	var collected []catalog.TestCatalogEntry
	for pageNumber := 0; pageNumber < 10; pageNumber++ {
		page, err := service.ListTests(t.Context(), query)
		if err != nil {
			t.Fatalf("list catalog page %d: %v", pageNumber, err)
		}
		collected = append(collected, page.Items...)
		if page.NextCursor == "" {
			return collected
		}
		query.Cursor = page.NextCursor
	}

	t.Fatal("catalog pagination did not terminate")

	return nil
}

func compareTestCatalogIdentities(left, right catalog.TestCatalogIdentity) int {
	if compared := compareRepositoryIdentities(left.TestRepository, right.TestRepository); compared != 0 {
		return compared
	}
	if compared := cmp.Compare(left.SuiteKey, right.SuiteKey); compared != 0 {
		return compared
	}

	return cmp.Compare(left.TestKey, right.TestKey)
}

func entryHasCapability(entry catalog.TestCatalogEntry, key string) bool {
	for _, capability := range entry.Capabilities {
		if capability.Key == key {
			return true
		}
	}

	return false
}

func migratedTestStore(t *testing.T) *Store {
	t.Helper()

	databaseURL := newTestDatabase(t)
	migrateTestDatabase(t, databaseURL)

	return openTestStore(t, databaseURL)
}

func migrateTestDatabase(t *testing.T, databaseURL string) *Migrator {
	t.Helper()

	migrator := openTestMigrator(t, databaseURL)
	if err := migrator.Migrate(t.Context()); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}

	return migrator
}

func openTestStore(t *testing.T, databaseURL string) *Store {
	t.Helper()

	store, err := OpenStore(t.Context(), databaseURL)
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	t.Cleanup(store.Close)

	return store
}

func openTestMigrator(t *testing.T, databaseURL string) *Migrator {
	t.Helper()

	migrator, err := OpenMigrator(t.Context(), databaseURL)
	if err != nil {
		t.Fatalf("open test migrator: %v", err)
	}
	t.Cleanup(migrator.Close)

	return migrator
}

func newTestDatabase(t *testing.T) string {
	t.Helper()

	databaseURL := os.Getenv(testDatabaseURLEnvironment)
	if databaseURL == "" {
		t.Fatalf("%s is required for integration tests", testDatabaseURLEnvironment)
	}
	adminConfig, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse test database configuration: %v", err)
	}
	if !isLoopbackHost(adminConfig.Host) {
		t.Fatalf("%s must identify a loopback PostgreSQL server", testDatabaseURLEnvironment)
	}

	adminConnection, err := pgx.ConnectConfig(t.Context(), adminConfig)
	if err != nil {
		t.Fatalf("connect to test database server: %v", err)
	}
	t.Cleanup(func() {
		_ = adminConnection.Close(context.Background()) //nolint:errcheck // Cleanup cannot recover a close failure.
	})

	databaseName := "argus_test_" + randomHex(t, 8)
	identifier := pgx.Identifier{databaseName}.Sanitize()
	if _, err := adminConnection.Exec(t.Context(), "CREATE DATABASE "+identifier); err != nil {
		t.Fatalf("create disposable database: %v", err)
	}
	t.Cleanup(func() {
		if _, err := adminConnection.Exec(
			context.Background(),
			"DROP DATABASE "+identifier+" WITH (FORCE)",
		); err != nil {
			t.Errorf("drop disposable database: %v", err)
		}
	})

	testDatabaseURL, err := databaseURLForName(databaseURL, databaseName)
	if err != nil {
		t.Fatalf("derive disposable database URL: %v", err)
	}

	return testDatabaseURL
}

func databaseURLForName(databaseURL, databaseName string) (string, error) {
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return "", fmt.Errorf("parse PostgreSQL URL: %w", err)
	}
	if parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" {
		return "", errors.New("test database configuration must be a PostgreSQL URL")
	}
	if parsed.Host == "" {
		return "", errors.New("test database URL must include a server address")
	}

	parsed.Path = "/" + databaseName
	parsed.RawPath = ""

	return parsed.String(), nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)

	return ip != nil && ip.IsLoopback()
}

func randomHex(t *testing.T, byteCount int) string {
	t.Helper()

	value := make([]byte, byteCount)
	if _, err := rand.Read(value); err != nil {
		t.Fatalf("generate disposable database name: %v", err)
	}

	return hex.EncodeToString(value)
}

func loadTestSnapshot(t *testing.T) catalog.Snapshot {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(
		"..",
		"..",
		"..",
		"contracts",
		"fixtures",
		"repository-descriptor",
		"v1",
		"valid",
		"source-and-test-repositories.json",
	))
	if err != nil {
		t.Fatalf("read descriptor fixture: %v", err)
	}
	document, err := contracts.DecodeRepositoryDescriptorV1(data)
	if err != nil {
		t.Fatalf("decode descriptor fixture: %v", err)
	}
	revision, err := catalog.NewRevision(
		catalog.RevisionGitSHA1,
		"0123456789abcdef0123456789abcdef01234567",
	)
	if err != nil {
		t.Fatalf("create test revision: %v", err)
	}
	snapshot, err := descriptor.Import(document, revision)
	if err != nil {
		t.Fatalf("import descriptor fixture: %v", err)
	}

	return snapshot
}

func queryableTestSnapshot(t *testing.T) catalog.Snapshot {
	t.Helper()

	snapshot := loadTestSnapshot(t)
	snapshot.TestSuites[0].Tests = append(
		snapshot.TestSuites[0].Tests,
		catalog.Test{
			Key:          "cancel-order-valid",
			Name:         "cancel an existing order",
			Capabilities: []string{"cancel-order"},
		},
		catalog.Test{
			Key:          "create-and-cancel-order",
			Name:         "create and cancel an order",
			Capabilities: []string{"create-order", "cancel-order"},
		},
	)
	snapshot.TestSuites = append(snapshot.TestSuites, catalog.TestSuite{
		Key: "orders-browser",
		Repository: catalog.Repository{
			Identity: catalog.RepositoryIdentity{
				Provider:             catalog.ProviderGitHub,
				Host:                 "github.com",
				ProviderRepositoryID: "R_orders_browser_01",
			},
			Owner: "example",
			Name:  "orders-browser-tests",
		},
		Family:  catalog.TestFamilyFunctionalUI,
		Adapter: "playwright",
		Tests: []catalog.Test{
			{
				Key:          "create-order-browser",
				Name:         "create an order in the browser",
				Capabilities: []string{"create-order"},
			},
		},
	})

	return snapshot
}

func reorderedSnapshot(snapshot catalog.Snapshot) catalog.Snapshot {
	reordered := cloneSnapshotForIntegration(snapshot)
	for index := range reordered.Components {
		reverseStrings(reordered.Components[index].Capabilities)
	}
	for suiteIndex := range reordered.TestSuites {
		for testIndex := range reordered.TestSuites[suiteIndex].Tests {
			reverseStrings(reordered.TestSuites[suiteIndex].Tests[testIndex].Capabilities)
		}
	}

	return reordered
}

func reverseStrings(values []string) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

func cloneSnapshotForIntegration(snapshot catalog.Snapshot) catalog.Snapshot {
	clone := snapshot
	clone.Capabilities = append([]catalog.Capability(nil), snapshot.Capabilities...)
	clone.Components = make([]catalog.Component, len(snapshot.Components))
	for index, component := range snapshot.Components {
		clone.Components[index] = component
		clone.Components[index].Capabilities = append([]string(nil), component.Capabilities...)
	}
	clone.TestSuites = make([]catalog.TestSuite, len(snapshot.TestSuites))
	for suiteIndex, suite := range snapshot.TestSuites {
		clone.TestSuites[suiteIndex] = suite
		clone.TestSuites[suiteIndex].Tests = make([]catalog.Test, len(suite.Tests))
		for testIndex, test := range suite.Tests {
			clone.TestSuites[suiteIndex].Tests[testIndex] = test
			clone.TestSuites[suiteIndex].Tests[testIndex].Capabilities = append(
				[]string(nil),
				test.Capabilities...,
			)
		}
	}

	return clone
}
