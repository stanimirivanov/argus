//go:build integration

package postgres

import (
	"io/fs"
	"testing"
	"testing/fstest"
	"time"
)

func TestPerformanceClaimsUpgradeRetainsReleasedRequestEvidence(t *testing.T) {
	url := newTestDatabase(t)
	migrator := openTestMigrator(t, url)
	entries, err := fs.ReadDir(embeddedMigrations, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	released := fstest.MapFS{}
	for _, entry := range entries {
		if entry.Name() >= "20261026092008_add_performance_dispatch_claims.sql" {
			continue
		}
		path := "migrations/" + entry.Name()
		raw, err := fs.ReadFile(embeddedMigrations, path)
		if err != nil {
			t.Fatal(err)
		}
		released[path] = &fstest.MapFile{Data: raw}
	}
	if err := applyMigrationChain(t.Context(), migrator, released); err != nil {
		t.Fatal(err)
	}
	store := openTestStore(t, url)
	snapshot := performanceFixture(t, store, "pages", "released", -time.Minute)
	if _, err := store.runtime.Performance().Publish(t.Context(), snapshot); err != nil {
		t.Fatal(err)
	}
	data := requestFixture(t, store, snapshot.Fingerprint(), "released")
	intent := inwardRequest(t, data)
	if _, err := store.runtime.Performance().CreateRequest(t.Context(), intent); err != nil {
		t.Fatal(err)
	}
	if err := migrator.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	got, err := store.runtime.Performance().GetRequest(t.Context(), data.Scope, data.ReviewSHA256)
	if err != nil || got.Fingerprint() != intent.Fingerprint() {
		t.Fatal("upgrade changed released evidence", err)
	}
	if _, err := store.runtime.Performance().CreateRequest(t.Context(), inwardRequest(t, requestFixture(t, store, snapshot.Fingerprint(), "after-upgrade"))); err != nil {
		t.Fatal("existing creation path failed after expansion", err)
	}
	if claim, err := store.runtime.Performance().ClaimRequest(t.Context(), data.Scope, time.Minute); err != nil || claim.Fence != 1 {
		t.Fatal("upgraded child cannot be claimed", err)
	}
}
