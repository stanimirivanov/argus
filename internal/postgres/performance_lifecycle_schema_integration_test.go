//go:build integration

package postgres

import (
	"context"
	"errors"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stanimirivanov/argus/internal/performance"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

func lifecycleSchemaFixture(t *testing.T, kind string) (*PerformanceStore, requestintent.Scope, requestintent.Claim, requestintent.Outcome) {
	t.Helper()
	url := newTestDatabase(t)
	migrator := openTestMigrator(t, url)
	chain, err := loadMigrations(embeddedMigrations)
	if err != nil {
		t.Fatal(err)
	}
	released := fstest.MapFS{}
	for _, migration := range chain {
		if migration.name < "20261027081424_add_performance_lifecycle_history.sql" {
			released["migrations/"+migration.name] = &fstest.MapFile{Data: migration.contents}
		}
	}
	if err := applyMigrationChain(t.Context(), migrator, released); err != nil {
		t.Fatal(err)
	}
	store := openTestStore(t, url)
	snapshot := performanceFixture(t, store, "pages", "lifecycle-upgrade", -time.Minute)
	capability := store.runtime.Performance()
	if _, err := capability.Publish(t.Context(), snapshot); err != nil {
		t.Fatal(err)
	}
	data := requestFixture(t, store, snapshot.Fingerprint(), "lifecycle-upgrade")
	if _, err := capability.CreateRequest(t.Context(), inwardRequest(t, data)); err != nil {
		t.Fatal(err)
	}
	claim, err := seedReleasedClaim(t, capability, data.Scope)
	if err != nil {
		t.Fatal(err)
	}
	admission := outcomeFixture(t, data.Scope, claim, kind)
	if _, err := capability.SettleRequest(t.Context(), data.Scope, claim, admission); err != nil {
		t.Fatal(err)
	}
	if err := migrator.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got, err := capability.RequestOutcome(t.Context(), data.Scope, claim.Pending.After); err != nil || got.Fingerprint() != admission.Fingerprint() {
		t.Fatal("upgrade changed released settlement", err)
	}

	return capability, data.Scope, claim, admission
}

func schemaLifecycle(t *testing.T, scope requestintent.Scope, claim requestintent.Claim, id string, revision int64, state requestintent.LifecycleState, run string) requestintent.LifecycleEvent {
	t.Helper()
	outcome, err := requestintent.NewOutcome(requestintent.OutcomeData{Request: claim.Pending.Child.Request, Attempt: claim.Pending.Child.Attempt, Source: scope.Issuer, EventID: id, Kind: "lifecycle", Payload: []byte(`{"validated":"provider"}`)})
	if err != nil {
		t.Fatal(err)
	}
	event, err := requestintent.NewLifecycleEvent(requestintent.LifecycleData{Evidence: outcome, Run: performance.Identity{Issuer: scope.Issuer, ID: run}, RequestSHA256: claim.Pending.Child.SHA256, Revision: revision, State: state, OccurredAt: claim.Pending.Child.Deadline, Deadline: claim.Pending.Child.Deadline})
	if err != nil {
		t.Fatal(err)
	}

	return event
}

func insertLifecycleSchema(ctx context.Context, store *PerformanceStore, scope requestintent.Scope, position int64, admission requestintent.Outcome, event requestintent.LifecycleEvent) error {
	d, evidence := event.Data(), event.Data().Evidence.Data()
	_, err := store.runtime.pool.Exec(ctx, `INSERT INTO argus_catalog.performance_lifecycle_events
        (position,issuer,principal_issuer,principal_id,admission_fingerprint,request_sha256,run_id,event_id,fingerprint,revision,state,occurred_at,payload)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, position, scope.Issuer, scope.PrincipalIssuer, scope.PrincipalID,
		admission.Fingerprint(), d.RequestSHA256, d.Run.ID, evidence.EventID, d.Evidence.Fingerprint(), d.Revision, d.State, d.OccurredAt, evidence.Payload)

	return err
}

func rejectLifecycleEventReuse(t *testing.T, store *PerformanceStore, scope requestintent.Scope) {
	t.Helper()
	second, err := store.ClaimRequest(t.Context(), scope, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	otherAdmission := outcomeFixture(t, scope, second, "accepted")
	collisionData := otherAdmission.Data()
	collisionData.EventID = "running"
	collision, err := requestintent.NewOutcome(collisionData)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SettleRequest(t.Context(), scope, second, collision); err == nil {
		t.Fatal("later admission reused lifecycle event")
	}
	if _, err := store.SettleRequest(t.Context(), scope, second, otherAdmission); err != nil {
		t.Fatal(err)
	}
	if err := insertLifecycleSchema(t.Context(), store, scope, second.Pending.After, otherAdmission, schemaLifecycle(t, scope, second, "running", 0, requestintent.LifecycleRunning, "run")); err == nil {
		t.Fatal("provider event reused across requests")
	}
}

func TestPerformanceLifecycleSchemaOrdersConcurrentHistoryAndPreservesUpgrade(t *testing.T) {
	store, scope, claim, admission := lifecycleSchemaFixture(t, "accepted")
	first := schemaLifecycle(t, scope, claim, "running", 0, requestintent.LifecycleRunning, "run")
	if err := insertLifecycleSchema(t.Context(), store, scope, claim.Pending.After, admission, first); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	for _, state := range []requestintent.LifecycleState{requestintent.LifecycleCompleted, requestintent.LifecycleFailed} {
		event := schemaLifecycle(t, scope, claim, string(state), 2, state, "run")
		go func() {
			results <- insertLifecycleSchema(t.Context(), store, scope, claim.Pending.After, admission, event)
		}()
	}
	a, b := <-results, <-results
	var conflict *pgconn.PgError
	if a == nil {
		a, b = b, a
	}
	if b != nil || !errors.As(a, &conflict) || conflict.ConstraintName != "ck_performance_lifecycle_transition" {
		t.Fatal("concurrent terminal conflict", a, b)
	}
	stale := schemaLifecycle(t, scope, claim, "stale", 1, requestintent.LifecycleRunning, "run")
	if err := insertLifecycleSchema(t.Context(), store, scope, claim.Pending.After, admission, stale); err != nil {
		t.Fatal("valid late/stale observation rejected", err)
	}
	for _, event := range []requestintent.LifecycleEvent{first, schemaLifecycle(t, scope, claim, "other-run", 0, requestintent.LifecycleRunning, "other"), schemaLifecycle(t, scope, claim, "backwards", 1, requestintent.LifecycleAccepted, "run"), schemaLifecycle(t, scope, claim, "changed", 3, requestintent.LifecycleCompleted, "run")} {
		if err := insertLifecycleSchema(t.Context(), store, scope, claim.Pending.After, admission, event); err == nil {
			t.Fatal("conflicting history accepted")
		}
	}
	wrong := scope
	wrong.PrincipalID += "-wrong"
	if err := insertLifecycleSchema(t.Context(), store, wrong, claim.Pending.After, admission, stale); err == nil {
		t.Fatal("cross-scope history accepted")
	}
	rejectLifecycleEventReuse(t, store, scope)
	for _, statement := range []string{"UPDATE argus_catalog.performance_lifecycle_events SET run_id='changed'", "DELETE FROM argus_catalog.performance_lifecycle_events", "TRUNCATE argus_catalog.performance_lifecycle_events"} {
		if _, err := store.runtime.pool.Exec(t.Context(), statement); err == nil {
			t.Fatal("mutable lifecycle evidence")
		}
	}
	url := store.runtime.pool.Config().ConnString()
	store.runtime.Close()
	reopened := openTestStore(t, url)
	var count int
	if err := reopened.pool.QueryRow(t.Context(), `SELECT count(*) FROM argus_catalog.performance_lifecycle_events`).Scan(&count); err != nil || count != 3 {
		t.Fatal("reopen lost atomic history", count, err)
	}
}

func TestPerformanceLifecycleSchemaRequiresRecoveryAnchorAndRejectsRejection(t *testing.T) {
	store, scope, claim, admission := lifecycleSchemaFixture(t, "lifecycle")
	event := schemaLifecycle(t, scope, claim, "later", 1, requestintent.LifecycleCompleted, "run")
	if err := insertLifecycleSchema(t.Context(), store, scope, claim.Pending.After, admission, event); err == nil {
		t.Fatal("recovery anchor omitted")
	}
	anchor := schemaLifecycle(t, scope, claim, admission.Data().EventID, 0, requestintent.LifecycleRunning, "run")
	if err := insertLifecycleSchema(t.Context(), store, scope, claim.Pending.After, admission, anchor); err != nil {
		t.Fatal("recovery anchor rejected", err)
	}
	second, err := store.ClaimRequest(t.Context(), scope, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	rejected := outcomeFixture(t, scope, second, "rejected")
	if _, err := store.SettleRequest(t.Context(), scope, second, rejected); err != nil {
		t.Fatal(err)
	}
	if err := insertLifecycleSchema(t.Context(), store, scope, second.Pending.After, rejected, schemaLifecycle(t, scope, second, "impossible", 0, requestintent.LifecycleRunning, "run")); err == nil {
		t.Fatal("rejection acquired lifecycle")
	}
	// Exercise structural capacity in one set-based insert; wire validation is
	// separately covered by the adapter conformance tests, not this SQL boundary.
	_, err = store.runtime.pool.Exec(t.Context(), `INSERT INTO argus_catalog.performance_lifecycle_events
        (position,issuer,principal_issuer,principal_id,admission_fingerprint,request_sha256,run_id,event_id,fingerprint,revision,state,occurred_at,payload)
        SELECT position,issuer,principal_issuer,principal_id,admission_fingerprint,request_sha256,run_id,'repeat-'||n,
            encode(sha256(convert_to(n::text,'UTF8')),'hex'),revision,state,occurred_at,payload
        FROM argus_catalog.performance_lifecycle_events CROSS JOIN generate_series(1,999) n WHERE position=$1`, claim.Pending.After)
	if err != nil {
		t.Fatal("history bound reached early", err)
	}
	if err := insertLifecycleSchema(t.Context(), store, scope, claim.Pending.After, admission, event); err == nil {
		t.Fatal("unbounded history admitted")
	}
}
