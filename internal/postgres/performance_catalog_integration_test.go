//go:build integration

package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/performance/adapters/cataloghttp"
	"github.com/stanimirivanov/argus/internal/performance/adapters/cli/catalogcli"
	"github.com/stanimirivanov/argus/internal/performance/adapters/perfeng"
	"github.com/stanimirivanov/argus/internal/performance/catalogsync"
)

// Database time is the subject: fixtures keep their published resource digest,
// while authority/source/window metadata is set relative to the test database.
func performanceFixture(t *testing.T, store *integratedStore, name, id string, offset time.Duration) catalogsync.Snapshot {
	t.Helper()
	var now time.Time
	if err := store.pool.QueryRow(t.Context(), "SELECT clock_timestamp()").Scan(&now); err != nil {
		t.Fatal(err)
	}

	return performanceFixtureAt(t, name, id, now.Add(offset), now)
}

func TestPerformanceSyncToPublicationAndFailedRefresh(t *testing.T) {
	store := migratedTestStore(t)
	snapshot := performanceFixture(t, store, "pages", "sync", -time.Minute)
	var documents []json.RawMessage
	if err := json.Unmarshal(snapshot.Evidence().Pages, &documents); err != nil {
		t.Fatal(err)
	}
	var index atomic.Int32
	var fail atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("missing configured credential")
		}
		if fail.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		offset := int(index.Add(1) - 1)
		if _, err := w.Write(documents[offset]); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	storage := catalogcli.Storage{Publish: func(ctx context.Context, source []json.RawMessage, expected perfeng.Expectations) (catalogcli.Publication, error) {
		imported, err := perfeng.Import(source, expected, time.Now())
		if err != nil {
			return catalogcli.Publication{}, err
		}
		result, err := catalogsync.Publish(ctx, store.runtime.Performance(), imported)
		if err != nil {
			return catalogcli.Publication{}, err
		}
		var pages []json.RawMessage
		if err := json.Unmarshal(result.Snapshot.Evidence().Pages, &pages); err != nil {
			return catalogcli.Publication{}, err
		}

		return catalogcli.Publication{Created: result.Created, Pages: pages}, nil
	}}
	fetch := func(ctx context.Context, endpoint, token string, expected perfeng.Expectations) ([]json.RawMessage, error) {
		return (cataloghttp.Source{Endpoint: endpoint, Token: token, Expected: expected}).Fetch(ctx)
	}
	scope := snapshot.Evidence().Scope
	args := []string{"-publish", "-endpoint", server.URL, "-issuer", scope.Issuer, "-principal-issuer", scope.PrincipalIssuer, "-principal-id", scope.PrincipalID, "-producer-version", "1.0.0"}
	var output bytes.Buffer
	if err := catalogcli.RunWithStorage(t.Context(), args, "test-token", &output, fetch, storage); err != nil {
		t.Fatal(err)
	}
	var result catalogcli.Publication
	if err := json.Unmarshal(output.Bytes(), &result); err != nil || !result.Created || len(result.Pages) != 2 {
		t.Fatalf("missing durable publication output: %v", err)
	}
	fail.Store(true)
	output.Reset()
	if err := catalogcli.RunWithStorage(t.Context(), args, "test-token", &output, fetch, storage); err == nil || output.Len() != 0 {
		t.Fatal("failed provider refresh exposed publication")
	}
	current, err := store.runtime.Performance().Current(t.Context(), scope)
	if err != nil || current.Fingerprint() != snapshot.Fingerprint() {
		t.Fatalf("failed refresh lost prior view: %v", err)
	}
}

func performanceFixtureAt(t *testing.T, name, id string, generated, now time.Time) catalogsync.Snapshot {
	t.Helper()
	raw, err := os.ReadFile("../performance/adapters/perfeng/snapshot/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var source []map[string]any
	if err := json.Unmarshal(raw, &source); err != nil {
		t.Fatal(err)
	}
	pages := make([]json.RawMessage, 0, len(source))
	for _, page := range source {
		header := page["snapshot"].(map[string]any)
		header["reference"].(map[string]any)["id"] = id
		header["generatedAt"] = generated.UTC().Format(time.RFC3339Nano)
		header["validUntil"] = now.Add(time.Hour).UTC().Format(time.RFC3339Nano)
		encoded, err := json.Marshal(page)
		if err != nil {
			t.Fatal(err)
		}
		pages = append(pages, encoded)
	}
	expected := perfeng.Expectations{Issuer: "https://provider.example/identity", PrincipalIssuer: "https://auth.example/tenant", PrincipalID: "argus-service", ProducerID: "perfeng-control-plane", ProducerVersion: "1.0.0"}
	snapshot, err := perfeng.Import(pages, expected, now)
	if err != nil {
		t.Fatal(err)
	}

	return snapshot
}

func TestPerformancePublicationRetainsHistoryAndEmptyRefresh(t *testing.T) {
	store := migratedTestStore(t)
	capability := store.runtime.Performance()
	first := performanceFixture(t, store, "pages", "first", -10*time.Minute)
	result, err := catalogsync.Publish(t.Context(), capability, first)
	if err != nil || !result.Created || result.Snapshot.Fingerprint() != first.Fingerprint() {
		t.Fatalf("publication/readback: %v, %v", result.Created, err)
	}
	if len(result.Snapshot.Evidence().Resources) != 8 {
		t.Fatal("lost published resource identities")
	}
	retired := false
	for _, resource := range result.Snapshot.Evidence().Resources {
		retired = retired || resource.State == "RETIRED"
	}
	if !retired {
		t.Fatal("lost explicit retirement evidence")
	}
	second := performanceFixture(t, store, "empty", "second", -5*time.Minute)
	if created, err := capability.Publish(t.Context(), second); err != nil || !created {
		t.Fatalf("empty refresh: %v, %v", created, err)
	}
	if created, err := capability.Publish(t.Context(), first); err != nil || created {
		t.Fatalf("historical replay: %v, %v", created, err)
	}
	current, err := capability.Current(t.Context(), first.Evidence().Scope)
	if err != nil || current.Fingerprint() != second.Fingerprint() || len(current.Evidence().Resources) != 0 {
		t.Fatalf("replay repointed current or empty refresh retained old membership: %v", err)
	}
	history, err := capability.Get(t.Context(), first.Evidence().Scope, first.Evidence().ID)
	if err != nil || history.Fingerprint() != first.Fingerprint() {
		t.Fatalf("lost history: %v", err)
	}
	wrong := first.Evidence().Scope
	wrong.PrincipalID = "other"
	if _, err := capability.Current(t.Context(), wrong); !errors.Is(err, catalogsync.ErrNotFound) {
		t.Fatalf("cross-principal read: %v", err)
	}
}

func TestPerformancePublicationRejectsConflictOlderAndExpired(t *testing.T) {
	store := migratedTestStore(t)
	capability := store.runtime.Performance()
	first := performanceFixture(t, store, "pages", "first", -10*time.Minute)
	if _, err := capability.Publish(t.Context(), first); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		change func(*catalogsync.Evidence)
		want   error
	}{
		{"conflict", func(e *catalogsync.Evidence) { e.Revision = "changed" }, catalogsync.ErrConflict},
		{"equal", func(e *catalogsync.Evidence) { e.ID = "equal" }, catalogsync.ErrOlder},
		{"older", func(e *catalogsync.Evidence) { e.ID = "older"; e.GeneratedAt = e.GeneratedAt.Add(-time.Minute) }, catalogsync.ErrOlder},
		{"expired", func(e *catalogsync.Evidence) { e.ID = "expired"; e.ValidUntil = e.GeneratedAt.Add(time.Minute) }, catalogsync.ErrExpired},
		{"future", func(e *catalogsync.Evidence) { e.ID = "future"; e.GeneratedAt = e.ValidUntil.Add(-time.Minute) }, catalogsync.ErrExpired},
	} {
		t.Run(test.name, func(t *testing.T) {
			data := first.Evidence()
			test.change(&data)
			snapshot, err := catalogsync.NewSnapshot(data)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := capability.Publish(t.Context(), snapshot); !errors.Is(err, test.want) {
				t.Fatalf("publication: %v, want %v", err, test.want)
			}
		})
	}
	current, err := capability.Current(t.Context(), first.Evidence().Scope)
	if err != nil || current.Fingerprint() != first.Fingerprint() {
		t.Fatalf("failed refresh changed current: %v", err)
	}
	var count int
	if err := store.pool.QueryRow(t.Context(), "SELECT count(*) FROM argus_catalog.performance_catalog_snapshots").Scan(&count); err != nil || count != 1 {
		t.Fatalf("failed publication left partial evidence: count=%d err=%v", count, err)
	}
}

func TestPerformanceConcurrentPublicationKeepsNewest(t *testing.T) {
	store := migratedTestStore(t)
	first := performanceFixture(t, store, "pages", "first", -10*time.Minute)
	second := performanceFixture(t, store, "empty", "second", -5*time.Minute)
	start := make(chan struct{})
	errorsFound := make(chan error, 2)
	var writers sync.WaitGroup
	for _, snapshot := range []catalogsync.Snapshot{first, second} {
		writers.Go(func() {
			<-start
			_, err := store.runtime.Performance().Publish(t.Context(), snapshot)
			errorsFound <- err
		})
	}
	close(start)
	writers.Wait()
	close(errorsFound)
	for err := range errorsFound {
		if err != nil && !errors.Is(err, catalogsync.ErrOlder) {
			t.Fatal(err)
		}
	}
	current, err := store.runtime.Performance().Current(t.Context(), first.Evidence().Scope)
	if err != nil || current.Fingerprint() != second.Fingerprint() {
		t.Fatalf("older concurrent writer won: %v", err)
	}
}

func TestPerformanceWriteFailureRollsBackAndSealsPublishedEvidence(t *testing.T) {
	store := migratedTestStore(t)
	first := performanceFixture(t, store, "pages", "first", -10*time.Minute)
	if _, err := store.pool.Exec(t.Context(), `CREATE FUNCTION argus_catalog.fail_performance_test() RETURNS TRIGGER LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected failure'; END $$;
        CREATE TRIGGER fail_performance_test BEFORE INSERT ON argus_catalog.performance_catalog_resources FOR EACH STATEMENT EXECUTE FUNCTION argus_catalog.fail_performance_test()`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.runtime.Performance().Publish(t.Context(), first); !errors.Is(err, catalogsync.ErrUnavailable) {
		t.Fatalf("injected failure: %v", err)
	}
	if _, err := store.runtime.Performance().Current(t.Context(), first.Evidence().Scope); !errors.Is(err, catalogsync.ErrNotFound) {
		t.Fatalf("failed transaction published a head: %v", err)
	}
	var count int
	if err := store.pool.QueryRow(t.Context(), "SELECT count(*) FROM argus_catalog.performance_catalog_snapshots").Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed transaction retained header: %d, %v", count, err)
	}
	if _, err := store.pool.Exec(t.Context(), "DROP TRIGGER fail_performance_test ON argus_catalog.performance_catalog_resources; DROP FUNCTION argus_catalog.fail_performance_test()"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.runtime.Performance().Publish(t.Context(), first); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		"UPDATE argus_catalog.performance_catalog_snapshots SET source_revision='changed'",
		"DELETE FROM argus_catalog.performance_catalog_resources",
		"DELETE FROM argus_catalog.performance_catalog_current",
		`INSERT INTO argus_catalog.performance_catalog_resources SELECT snapshot_uuid,kind,issuer,resource_id || '-appended',revision,sha256,lifecycle_state FROM argus_catalog.performance_catalog_resources LIMIT 1`,
	} {
		if _, err := store.pool.Exec(t.Context(), query); err == nil {
			t.Fatalf("allowed mutation: %s", query)
		}
	}
}

func TestPerformanceDeferredPublicationGuards(t *testing.T) {
	store := migratedTestStore(t)
	first := performanceFixture(t, store, "pages", "first", -10*time.Minute)
	if _, err := store.runtime.Performance().Publish(t.Context(), first); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"incomplete", "expired", "older", "scope"} {
		t.Run(name, func(t *testing.T) {
			rejectPerformanceTransaction(t, store, first.Evidence().Scope, name)
		})
	}
	current, err := store.runtime.Performance().Current(t.Context(), first.Evidence().Scope)
	if err != nil || current.Fingerprint() != first.Fingerprint() {
		t.Fatalf("deferred failure changed head: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := store.runtime.Performance().Publish(ctx, first); !errors.Is(err, context.Canceled) {
		t.Fatalf("lost cancellation: %v", err)
	}
}

func rejectPerformanceTransaction(t *testing.T, store *integratedStore, scope catalogsync.Scope, name string) {
	t.Helper()
	tx, err := store.pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(t.Context()) }() //nolint:errcheck // Best effort after expected commit failure.
	var id string
	err = tx.QueryRow(t.Context(), `INSERT INTO argus_catalog.performance_catalog_snapshots
                (snapshot_uuid,issuer,principal_issuer,principal_id,source_id,source_revision,generated_at,valid_until,
                 fingerprint,content_sha256,schema_sha256,source_pages,resource_count)
                SELECT gen_random_uuid(),issuer,principal_issuer,principal_id,$1,source_revision,
                    CASE WHEN $1='older' THEN generated_at - interval '1 minute' ELSE generated_at + interval '1 minute' END,
                    CASE WHEN $1='expired' THEN clock_timestamp() - interval '1 second' ELSE valid_until END,
                    fingerprint,content_sha256,schema_sha256,source_pages,CASE WHEN $1='incomplete' THEN 1 ELSE 0 END
                FROM argus_catalog.performance_catalog_snapshots WHERE source_id='first'
                RETURNING snapshot_uuid::text`, name).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	// Completeness must also hold for an aggregate with no head update.
	if name != "incomplete" {
		principal := scope.PrincipalID
		if name == "scope" {
			principal = "another-principal"
		}
		_, err = tx.Exec(t.Context(), `INSERT INTO argus_catalog.performance_catalog_current
                    (issuer,principal_issuer,principal_id,snapshot_uuid) VALUES ($1,$2,$3,$4)
                    ON CONFLICT (issuer,principal_issuer,principal_id) DO UPDATE SET snapshot_uuid=EXCLUDED.snapshot_uuid`, scope.Issuer, scope.PrincipalIssuer, principal, id)
		if name == "scope" {
			if err == nil {
				t.Fatal("allowed cross-scope head")
			}
			return
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(t.Context()); err == nil {
		t.Fatalf("deferred %s guard accepted invalid publication", name)
	}
}
