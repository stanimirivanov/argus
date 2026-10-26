//go:build integration

package postgres

import (
	"testing"
	"testing/fstest"
	"time"

	"github.com/google/uuid"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

func TestPerformanceOutcomesUpgradePreservesReleasedClaims(t *testing.T) {
	url := newTestDatabase(t)
	migrator := openTestMigrator(t, url)
	chain, err := loadMigrations(embeddedMigrations)
	if err != nil {
		t.Fatal(err)
	}
	released := fstest.MapFS{}
	for _, migration := range chain {
		if migration.name < "20261026141742_add_performance_dispatch_outcomes.sql" {
			released["migrations/"+migration.name] = &fstest.MapFile{Data: migration.contents}
		}
	}
	if err := applyMigrationChain(t.Context(), migrator, released); err != nil {
		t.Fatal(err)
	}
	store := openTestStore(t, url)
	snapshot := performanceFixture(t, store, "pages", "released-claims", -time.Minute)
	if _, err := store.runtime.Performance().Publish(t.Context(), snapshot); err != nil {
		t.Fatal(err)
	}
	data := requestFixture(t, store, snapshot.Fingerprint(), "released-claims")
	intent := inwardRequest(t, data)
	if _, err := store.runtime.Performance().CreateRequest(t.Context(), intent); err != nil {
		t.Fatal(err)
	}
	pending, err := store.runtime.Performance().PendingRequests(t.Context(), data.Scope, 0, 1)
	if err != nil || len(pending) != 1 {
		t.Fatal("released read failed", err)
	}
	claim := requestintent.Claim{Pending: pending[0], Token: uuid.NewString(), Fence: 1}
	// Simulate the released writer against schema 17, before outcome queries exist.
	_, err = store.pool.Exec(t.Context(), `INSERT INTO argus_catalog.performance_dispatch_claims
        SELECT position,issuer,principal_issuer,principal_id,1,$2,clock_timestamp()+INTERVAL '5 minutes'
        FROM argus_catalog.performance_pending_dispatches WHERE position=$1`, claim.Pending.After, claim.Token)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrator.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got, err := store.runtime.Performance().GetRequest(t.Context(), data.Scope, data.ReviewSHA256); err != nil || got.Fingerprint() != intent.Fingerprint() {
		t.Fatal("upgrade changed evidence", err)
	}
	if created, err := store.runtime.Performance().SettleRequest(t.Context(), data.Scope, claim, outcomeFixture(t, data.Scope, claim, "accepted")); err != nil || !created {
		t.Fatal("upgrade lost claim", err)
	}
	if _, err := store.runtime.Performance().CreateRequest(t.Context(), inwardRequest(t, requestFixture(t, store, snapshot.Fingerprint(), "after-upgrade"))); err != nil {
		t.Fatal("old creation failed", err)
	}
}
