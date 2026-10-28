//go:build integration

package postgres

import (
	"errors"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

// Upgrade a populated released schema; structural fixtures exercise scheduling,
// while pinned wire validation remains covered by the existing consumer tests.
func cancellationScheduleFixture(t *testing.T) (*integratedStore, requestintent.Data, []requestintent.Pending) {
	t.Helper()
	url := newTestDatabase(t)
	migrator := openTestMigrator(t, url)
	chain, err := loadMigrations(embeddedMigrations)
	if err != nil {
		t.Fatal(err)
	}
	released := fstest.MapFS{}
	for _, migration := range chain {
		if migration.name < "20261028171732_add_performance_cancellation_delays.sql" {
			released["migrations/"+migration.name] = &fstest.MapFile{Data: migration.contents}
		}
	}
	if err := applyMigrationChain(t.Context(), migrator, released); err != nil {
		t.Fatal(err)
	}
	store := openTestStore(t, url)
	snapshot := performanceFixture(t, store, "pages", "cooldown-upgrade", -time.Minute)
	c := store.runtime.Performance()
	if _, err := c.Publish(t.Context(), snapshot); err != nil {
		t.Fatal(err)
	}
	data := requestFixture(t, store, snapshot.Fingerprint(), "cooldown-upgrade")
	if _, err := c.CreateRequest(t.Context(), inwardRequest(t, data)); err != nil {
		t.Fatal(err)
	}
	pending, err := c.PendingRequests(t.Context(), data.Scope, 0, 100)
	if err != nil || len(pending) != 2 {
		t.Fatal("fixture pending", err)
	}
	for range 2 {
		claim, err := c.ClaimRequest(t.Context(), data.Scope, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := c.SettleRequest(t.Context(), data.Scope, claim, outcomeFixture(t, data.Scope, claim, "accepted")); err != nil {
			t.Fatal(err)
		}
	}
	intent := cancellationForStore(t, pending[0].Child, "cooldown-first")
	if _, err := c.RequestCancellation(t.Context(), data.Scope, pending[0].After, intent, verifyStoreCancellation); err != nil {
		t.Fatal(err)
	}
	if err := migrator.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	got, err := c.GetCancellation(t.Context(), data.Scope, pending[0].After, verifyStoreCancellation)
	if err != nil || got.Fingerprint() != intent.Fingerprint() {
		t.Fatal("upgrade changed intent", err)
	}

	return store, data, pending
}

func TestCancellationCooldownScopesBothSelectorsAndExpiredWork(t *testing.T) {
	store, data, pending := cancellationScheduleFixture(t)
	c := store.runtime.Performance()
	issuer := pending[0].Child.Request.Issuer
	for _, scope := range []requestintent.Scope{{Issuer: "other", PrincipalIssuer: data.Scope.PrincipalIssuer, PrincipalID: data.Scope.PrincipalID}, {Issuer: data.Scope.Issuer, PrincipalIssuer: "other", PrincipalID: data.Scope.PrincipalID}, {Issuer: data.Scope.Issuer, PrincipalIssuer: data.Scope.PrincipalIssuer, PrincipalID: "other"}} {
		if _, err := c.NextCancellation(t.Context(), scope, issuer, 0); !errors.Is(err, requestintent.ErrNotFound) {
			t.Fatal("cross-scope selection", err)
		}
		if _, err := c.DeferCancellation(t.Context(), scope, issuer, pending[0].After, time.Second); !errors.Is(err, requestintent.ErrNotFound) {
			t.Fatal("cross-scope deferral", err)
		}
	}
	if _, err := c.NextCancellation(t.Context(), data.Scope, "other", 0); !errors.Is(err, requestintent.ErrNotFound) {
		t.Fatal("cross-issuer selection", err)
	}
	if _, err := c.DeferCancellation(t.Context(), data.Scope, "other", pending[0].After, time.Second); !errors.Is(err, requestintent.ErrNotFound) {
		t.Fatal("cross-issuer deferral", err)
	}
	got, err := c.NextCancellation(t.Context(), data.Scope, issuer, 0)
	if err != nil || got.After != pending[0].After {
		t.Fatal("due cancellation not selected", err)
	}
	until, err := c.DeferCancellation(t.Context(), data.Scope, issuer, pending[0].After, 24*time.Hour)
	if err != nil || until.IsZero() || until.Location() != time.UTC {
		t.Fatal("deferral failed", err)
	}
	if _, err := c.NextCancellation(t.Context(), data.Scope, issuer, 0); !errors.Is(err, requestintent.ErrNotFound) {
		t.Fatal("cooldown bypassed", err)
	}
	life, err := c.NextLifecycle(t.Context(), data.Scope, issuer, 0)
	if err != nil || life.After != pending[1].After {
		t.Fatal("lifecycle bypassed cooldown or unrelated work blocked", err)
	}
	intent := cancellationForStore(t, pending[1].Child, "cooldown-second")
	if _, err := c.RequestCancellation(t.Context(), data.Scope, pending[1].After, intent, verifyStoreCancellation); err != nil {
		t.Fatal(err)
	}
	_, err = store.pool.Exec(t.Context(), `INSERT INTO argus_catalog.performance_cancellation_delays (position,issuer,principal_issuer,principal_id,not_before) VALUES ($1,$2,$3,$4,clock_timestamp()-INTERVAL '1 second')`, pending[1].After, data.Scope.Issuer, data.Scope.PrincipalIssuer, data.Scope.PrincipalID)
	if err != nil {
		t.Fatal(err)
	}
	got, err = c.NextCancellation(t.Context(), data.Scope, issuer, 0)
	if err != nil || got.After != pending[1].After {
		t.Fatal("expired cooldown not eligible", err)
	}
	if _, err := c.NextCancellation(t.Context(), data.Scope, issuer, got.After); !errors.Is(err, requestintent.ErrNotFound) {
		t.Fatal("cursor was not exclusive", err)
	}
	assertCancellationDelayGuards(t, c, data.Scope, pending[0].After, until)
}

func assertCancellationDelayGuards(t *testing.T, c *PerformanceStore, scope requestintent.Scope, position int64, until time.Time) {
	t.Helper()
	for _, test := range []struct{ sql, code string }{
		{`UPDATE argus_catalog.performance_cancellation_delays SET not_before=not_before-INTERVAL '1 second'`, "23514"},
		{`UPDATE argus_catalog.performance_cancellation_delays SET principal_id='other'`, "23514"},
		{`UPDATE argus_catalog.performance_cancellation_delays SET not_before='infinity'`, "23514"},
		{`UPDATE argus_catalog.performance_cancellation_delays SET not_before=clock_timestamp()+INTERVAL '300 years'`, "23514"},
		{`DELETE FROM argus_catalog.performance_cancellation_delays`, "55000"},
		{`TRUNCATE argus_catalog.performance_cancellation_delays`, "55000"},
	} {
		_, err := c.runtime.pool.Exec(t.Context(), test.sql)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != test.code {
			t.Fatal("native cooldown mutation accepted", test.sql, err)
		}
	}
	got, err := c.DeferCancellation(t.Context(), scope, "https://argus.example", position, time.Second)
	if err != nil || !got.Equal(until) {
		t.Fatal("shorter delay/failed mutation shortened cooldown", err)
	}
}

func TestCancellationCooldownExcludesUnadmittedRejectedAndRollsBack(t *testing.T) {
	store, data, pending := cancellationScheduleFixture(t)
	c := store.runtime.Performance()
	issuer := pending[0].Child.Request.Issuer
	more := requestFixture(t, store, data.CatalogSHA256, "ineligible-cooldown")
	if _, err := c.CreateRequest(t.Context(), inwardRequest(t, more)); err != nil {
		t.Fatal(err)
	}
	claim, err := c.ClaimRequest(t.Context(), data.Scope, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.SettleRequest(t.Context(), data.Scope, claim, outcomeFixture(t, data.Scope, claim, "rejected")); err != nil {
		t.Fatal(err)
	}
	newPending, err := c.PendingRequests(t.Context(), data.Scope, pending[1].After, 100)
	if err != nil || len(newPending) != 2 {
		t.Fatal("ineligible candidates", err)
	}
	for _, p := range newPending {
		intent := cancellationForStore(t, p.Child, "delay-"+p.Child.EventID)
		if _, err := c.RequestCancellation(t.Context(), data.Scope, p.After, intent, verifyStoreCancellation); err != nil {
			t.Fatal(err)
		}
		if _, err := c.DeferCancellation(t.Context(), data.Scope, issuer, p.After, time.Second); !errors.Is(err, requestintent.ErrNotFound) {
			t.Fatal("unadmitted/rejected work deferred", err)
		}
	}
	if _, err := c.NextCancellation(t.Context(), data.Scope, issuer, pending[1].After); !errors.Is(err, requestintent.ErrNotFound) {
		t.Fatal("unadmitted/rejected work selected", err)
	}
	_, err = store.pool.Exec(t.Context(), `INSERT INTO argus_catalog.performance_cancellation_delays (position,issuer,principal_issuer,principal_id,not_before) VALUES ($1,$2,$3,$4,clock_timestamp()+INTERVAL '1 day')`, claim.Pending.After, data.Scope.Issuer, data.Scope.PrincipalIssuer, data.Scope.PrincipalID)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.ConstraintName != "ck_performance_delay_binding" {
		t.Fatal("native rejected work deferred", err)
	}
	tx, err := store.pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(t.Context()) }() //nolint:errcheck // Best effort after explicit test rollback.
	_, err = tx.Exec(t.Context(), `INSERT INTO argus_catalog.performance_cancellation_delays (position,issuer,principal_issuer,principal_id,not_before) VALUES ($1,$2,$3,$4,clock_timestamp()+INTERVAL '1 day')`, pending[0].After, data.Scope.Issuer, data.Scope.PrincipalIssuer, data.Scope.PrincipalID)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(t.Context()); err != nil {
		t.Fatal(err)
	}
	got, err := c.NextCancellation(t.Context(), data.Scope, issuer, 0)
	if err != nil || got.After != pending[0].After {
		t.Fatal("rollback parked work", err)
	}
}

func TestCancellationCooldownConcurrentDeferralsSurviveReopen(t *testing.T) {
	store, data, pending := cancellationScheduleFixture(t)
	c := store.runtime.Performance()
	issuer := pending[0].Child.Request.Issuer
	var wait sync.WaitGroup
	results := make(chan time.Time, 2)
	start := make(chan struct{})
	for _, delay := range []time.Duration{24 * time.Hour, 48 * time.Hour} {
		wait.Go(func() {
			<-start
			until, err := c.DeferCancellation(t.Context(), data.Scope, issuer, pending[0].After, delay)
			if err != nil {
				t.Error(err)
				return
			}
			results <- until
		})
	}
	close(start)
	wait.Wait()
	close(results)
	var maximum time.Time
	for until := range results {
		if until.After(maximum) {
			maximum = until
		}
	}
	if maximum.IsZero() {
		t.Fatal("no successful deferrals")
	}
	reopened := openTestStore(t, c.runtime.pool.Config().ConnString()).runtime.Performance()
	got, err := reopened.DeferCancellation(t.Context(), data.Scope, issuer, pending[0].After, time.Second)
	if err != nil || !got.Equal(maximum) {
		t.Fatal("restart/concurrency shortened cooldown", err)
	}
	if _, err := reopened.NextCancellation(t.Context(), data.Scope, issuer, 0); !errors.Is(err, requestintent.ErrNotFound) {
		t.Fatal("restart ignored cooldown", err)
	}
}
