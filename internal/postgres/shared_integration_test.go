//go:build integration

package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const testDatabaseURLEnvironment = "ARGUS_TEST_POSTGRES_URL"

// integratedStore is a test-only aggregate for the historical cross-capability
// integration scenarios. Production composition uses only scoped views.
type integratedStore struct {
	*CatalogStore
	*ChangeStore
	*ExecutionStore
	*AdaptationStore
	runtime *Runtime
	pool    *pgxpool.Pool
}

func openIntegratedStore(ctx context.Context, databaseURL string) (*integratedStore, error) {
	runtime, err := OpenRuntime(ctx, databaseURL)
	if err != nil {
		return nil, err
	}

	return &integratedStore{
		CatalogStore: runtime.Catalog(), ChangeStore: runtime.Change(),
		ExecutionStore: runtime.Execution(), AdaptationStore: runtime.Adaptation(),
		runtime: runtime, pool: runtime.pool,
	}, nil
}

func (store *integratedStore) Close() { store.runtime.Close() }

func TestScopedStoresShareOneRuntimePool(t *testing.T) {
	databaseURL := newTestDatabase(t)
	runtime, err := OpenRuntime(t.Context(), databaseURL)
	if err != nil {
		t.Fatalf("open runtime: %v", err)
	}
	t.Cleanup(runtime.Close)
	if runtime.Catalog().runtime != runtime || runtime.Change().runtime != runtime ||
		runtime.Execution().runtime != runtime || runtime.Adaptation().runtime != runtime {
		t.Fatal("capability stores do not share the runtime pool owner")
	}
}

func migratedTestStore(t *testing.T) *integratedStore {
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

func openTestStore(t *testing.T, databaseURL string) *integratedStore {
	t.Helper()

	store, err := openIntegratedStore(t.Context(), databaseURL)
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
