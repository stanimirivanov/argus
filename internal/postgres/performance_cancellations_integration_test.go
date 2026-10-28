//go:build integration

package postgres

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

// Structural storage fixtures exercise the pure verifier port; pinned wire truth
// is tested by the Perfeng adapter. No synthetic payload grants runtime authority.
func cancellationForStore(t *testing.T, child requestintent.Child, id string) requestintent.Cancellation {
	t.Helper()
	raw, err := json.Marshal(id)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := verifyStoreCancellation(child.Payload, raw)
	if err != nil {
		t.Fatal(err)
	}

	return intent
}

func verifyStoreCancellation(frozen, raw []byte) (requestintent.Cancellation, error) {
	var id string
	if err := json.Unmarshal(raw, &id); err != nil {
		return requestintent.Cancellation{}, err
	}

	return requestintent.NewCancellation(requestintent.CancellationData{RequestSHA256: requestDigest(string(frozen)), EventID: id, Payload: raw})
}

func TestCancellationIntentReplayScopesEventReuseAndImmutability(t *testing.T) {
	store, data := claimFixture(t, 0)
	c := store.runtime.Performance()
	pending, err := c.PendingRequests(t.Context(), data.Scope, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	intent := cancellationForStore(t, pending[0].Child, "cancel-owner")
	for i := 0; i < 2; i++ {
		created, err := c.RequestCancellation(t.Context(), data.Scope, pending[0].After, intent, verifyStoreCancellation)
		if err != nil || created != (i == 0) {
			t.Fatal("intent replay", created, err)
		}
	}
	for _, wrong := range []requestintent.Scope{{Issuer: "other", PrincipalIssuer: data.Scope.PrincipalIssuer, PrincipalID: data.Scope.PrincipalID}, {Issuer: data.Scope.Issuer, PrincipalIssuer: "other", PrincipalID: data.Scope.PrincipalID}, {Issuer: data.Scope.Issuer, PrincipalIssuer: data.Scope.PrincipalIssuer, PrincipalID: "other"}} {
		if _, err := c.GetCancellation(t.Context(), wrong, pending[0].After, verifyStoreCancellation); !errors.Is(err, requestintent.ErrNotFound) {
			t.Fatal("cross-scope read", err)
		}
		if _, err := c.RequestCancellation(t.Context(), wrong, pending[0].After, intent, verifyStoreCancellation); !errors.Is(err, requestintent.ErrNotFound) {
			t.Fatal("cross-scope write", err)
		}
	}
	for _, test := range []struct {
		position int64
		intent   requestintent.Cancellation
	}{
		{pending[0].After, cancellationForStore(t, pending[0].Child, "changed")},
		{pending[1].After, cancellationForStore(t, pending[1].Child, "cancel-owner")},
		{pending[1].After, cancellationForStore(t, pending[1].Child, pending[0].Child.EventID)},
	} {
		if _, err := c.RequestCancellation(t.Context(), data.Scope, test.position, test.intent, verifyStoreCancellation); !errors.Is(err, requestintent.ErrConflict) {
			t.Fatal("changed/reused intent", err)
		}
	}
	claim, err := c.ClaimRequest(t.Context(), data.Scope, time.Minute)
	if err != nil || claim.Pending.After != pending[1].After {
		t.Fatal("cutoff starved unrelated work", err)
	}
	if _, err := c.ClaimRequestForIssuer(t.Context(), data.Scope, pending[0].Child.Request.Issuer, time.Minute); !errors.Is(err, requestintent.ErrNotFound) {
		t.Fatal("cancelled first claim", err)
	}
	assertCancellationDatabaseGuards(t, c, data, pending, intent)
	assertCancellationSnapshotGuard(t, c, data.Scope, pending[0].After)
}

func assertCancellationSnapshotGuard(t *testing.T, c *PerformanceStore, scope requestintent.Scope, position int64) {
	t.Helper()
	tx, err := c.runtime.pool.BeginTx(t.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(t.Context()) }() //nolint:errcheck // Best effort after the deliberately rejected write.
	_, err = tx.Exec(t.Context(), `INSERT INTO argus_catalog.performance_dispatch_claims (position,issuer,principal_issuer,principal_id,fence,token,available_at) VALUES ($1,$2,$3,$4,1,$5,clock_timestamp()+INTERVAL '1 minute')`, position, scope.Issuer, scope.PrincipalIssuer, scope.PrincipalID, uuid.New())
	var sqlErr *pgconn.PgError
	if !errors.As(err, &sqlErr) || sqlErr.Code != "25000" {
		t.Fatal("old-snapshot writer was not rejected", err)
	}
}

func assertCancellationDatabaseGuards(t *testing.T, c *PerformanceStore, data requestintent.Data, pending []requestintent.Pending, intent requestintent.Cancellation) {
	t.Helper()
	_, err := c.runtime.pool.Exec(t.Context(), `INSERT INTO argus_catalog.performance_dispatch_claims (position,issuer,principal_issuer,principal_id,fence,token,available_at) VALUES ($1,$2,$3,$4,1,$5,clock_timestamp()+INTERVAL '1 minute')`, pending[0].After, data.Scope.Issuer, data.Scope.PrincipalIssuer, data.Scope.PrincipalID, uuid.New())
	var sqlErr *pgconn.PgError
	if !errors.As(err, &sqlErr) || sqlErr.ConstraintName != "ck_performance_cancellation_cutoff" {
		t.Fatal("native first claim bypassed cutoff", err)
	}
	for _, mutation := range []struct{ sql, code string }{
		{`UPDATE argus_catalog.performance_cancellations SET fingerprint=fingerprint`, "55000"},
		{`DELETE FROM argus_catalog.performance_cancellations`, "55000"},
		{`TRUNCATE argus_catalog.performance_cancellations`, "0A000"},
		{`TRUNCATE argus_catalog.performance_cancellations CASCADE`, "55000"},
	} {
		_, err := c.runtime.pool.Exec(t.Context(), mutation.sql)
		if !errors.As(err, &sqlErr) || sqlErr.Code != mutation.code {
			t.Fatal("mutable cancellation", err)
		}
	}
	data.ReviewSHA256 = requestDigest("cancel-event-reuse")
	data.Children = []requestintent.Child{pending[1].Child}
	data.Children[0].Request.ID += "-new"
	data.Children[0].Attempt.ID += "-new"
	data.Children[0].EventID = intent.Data().EventID
	if _, err := c.CreateRequest(t.Context(), inwardRequest(t, data)); !errors.Is(err, requestintent.ErrConflict) {
		t.Fatal("request reused cancellation event", err)
	}
	reopened := openTestStore(t, c.runtime.pool.Config().ConnString()).runtime.Performance()
	got, err := reopened.GetCancellation(t.Context(), data.Scope, pending[0].After, verifyStoreCancellation)
	if err != nil || got.Fingerprint() != intent.Fingerprint() {
		t.Fatal("intent did not survive reopen", err)
	}
	if _, err := c.RequestOutcome(t.Context(), data.Scope, pending[0].After); !errors.Is(err, requestintent.ErrNotFound) {
		t.Fatal("fabricated settlement", err)
	}
}

func TestCancellationAndFirstAcquisitionSerializeAndPreserveRecovery(t *testing.T) {
	store, data := claimFixture(t, 0)
	c := store.runtime.Performance()
	first, err := c.ClaimRequest(t.Context(), data.Scope, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := c.PendingRequests(t.Context(), data.Scope, first.Pending.After, 1)
	if err != nil || len(pending) != 1 {
		t.Fatal("race candidate", err)
	}
	intent := cancellationForStore(t, pending[0].Child, "cancel-race")
	start := make(chan struct{})
	var wait sync.WaitGroup
	var claim requestintent.Claim
	var claimErr, cancelErr error
	wait.Go(func() { <-start; claim, claimErr = c.ClaimRequest(t.Context(), data.Scope, time.Minute) })
	wait.Go(func() {
		<-start
		_, cancelErr = c.RequestCancellation(t.Context(), data.Scope, pending[0].After, intent, verifyStoreCancellation)
	})
	close(start)
	wait.Wait()
	if cancelErr != nil {
		t.Fatal("cancellation failed", cancelErr)
	}
	if claimErr == nil {
		if claim.Pending.After != pending[0].After || claim.Fence != 1 {
			t.Fatal("invalid winning claim")
		}
		if err := c.ReleaseRequest(t.Context(), data.Scope, claim, 0); err != nil {
			t.Fatal(err)
		}
		recovered, err := c.ClaimRequest(t.Context(), data.Scope, time.Minute)
		if err != nil || recovered.Fence != 2 || recovered.Pending.After != claim.Pending.After {
			t.Fatal("cutoff destroyed uncertain recovery", err)
		}
	} else if !errors.Is(claimErr, requestintent.ErrNotFound) && !errors.Is(claimErr, requestintent.ErrConflict) {
		t.Fatal("unexpected acquisition error", claimErr)
	}
	if _, err := c.ClaimRequest(t.Context(), data.Scope, time.Minute); !errors.Is(err, requestintent.ErrNotFound) {
		t.Fatal("new ownership after cutoff", err)
	}
}

func cancellationUpgradeFixture(t *testing.T) (*integratedStore, requestintent.Data, requestintent.Claim, requestintent.Pending) {
	t.Helper()
	url := newTestDatabase(t)
	migrator := openTestMigrator(t, url)
	chain, err := loadMigrations(embeddedMigrations)
	if err != nil {
		t.Fatal(err)
	}
	released := fstest.MapFS{}
	for _, migration := range chain {
		if migration.name < "20261028113837_add_performance_cancellation_intents.sql" {
			released["migrations/"+migration.name] = &fstest.MapFile{Data: migration.contents}
		}
	}
	if err := applyMigrationChain(t.Context(), migrator, released); err != nil {
		t.Fatal(err)
	}
	store := openTestStore(t, url)
	snapshot := performanceFixture(t, store, "pages", "cancel-upgrade", -time.Minute)
	c := store.runtime.Performance()
	if _, err := c.Publish(t.Context(), snapshot); err != nil {
		t.Fatal(err)
	}
	data := requestFixture(t, store, snapshot.Fingerprint(), "cancel-upgrade")
	if _, err := c.CreateRequest(t.Context(), inwardRequest(t, data)); err != nil {
		t.Fatal(err)
	}
	seeded, err := seedReleasedClaim(t, c, data.Scope)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrator.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	got, err := c.GetRequest(t.Context(), data.Scope, data.ReviewSHA256)
	if err != nil || got.Fingerprint() != inwardRequest(t, data).Fingerprint() {
		t.Fatal("upgrade changed frozen intent", err)
	}
	pending, err := c.PendingRequests(t.Context(), data.Scope, seeded.Pending.After, 1)
	if err != nil || len(pending) != 1 {
		t.Fatal(err)
	}

	return store, data, seeded, pending[0]
}

func TestCancellationRollbackUpgradeAndCorruptReadFailClosed(t *testing.T) {
	store, data, seeded, pending := cancellationUpgradeFixture(t)
	c := store.runtime.Performance()
	intent := cancellationForStore(t, pending.Child, "cancel-rollback")
	_, err := store.pool.Exec(t.Context(), `CREATE FUNCTION argus_catalog.fail_cancellation_test() RETURNS TRIGGER LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test rollback'; END $$; CREATE TRIGGER zz_fail_cancellation AFTER INSERT ON argus_catalog.performance_cancellations FOR EACH ROW EXECUTE FUNCTION argus_catalog.fail_cancellation_test()`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.RequestCancellation(t.Context(), data.Scope, pending.After, intent, verifyStoreCancellation); !errors.Is(err, requestintent.ErrUnavailable) {
		t.Fatal("forced write succeeded", err)
	}
	if _, err := c.GetCancellation(t.Context(), data.Scope, pending.After, verifyStoreCancellation); !errors.Is(err, requestintent.ErrNotFound) {
		t.Fatal("rollback leaked intent", err)
	}
	claim, err := c.ClaimRequest(t.Context(), data.Scope, time.Minute)
	if err != nil || claim.Fence != 1 || claim.Pending.After != pending.After {
		t.Fatal("rollback leaked cutoff", err)
	}
	_, err = store.pool.Exec(t.Context(), `DROP TRIGGER zz_fail_cancellation ON argus_catalog.performance_cancellations`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.RequestCancellation(t.Context(), data.Scope, pending.After, intent, verifyStoreCancellation); err != nil {
		t.Fatal(err)
	}
	first := cancellationForStore(t, seeded.Pending.Child, "cancel-acquired")
	if _, err := c.RequestCancellation(t.Context(), data.Scope, seeded.Pending.After, first, verifyStoreCancellation); err != nil {
		t.Fatal(err)
	}
	if err := c.ReleaseRequest(t.Context(), data.Scope, seeded, 0); err != nil {
		t.Fatal(err)
	}
	recovered, err := c.ClaimRequest(t.Context(), data.Scope, time.Minute)
	if err != nil || recovered.Fence != 2 || recovered.Pending.After != seeded.Pending.After {
		t.Fatal("upgrade lost uncertain recovery", err)
	}
	_, err = store.pool.Exec(t.Context(), `ALTER TABLE argus_catalog.performance_cancellations DISABLE TRIGGER trg_performance_cancellation_immutable; UPDATE argus_catalog.performance_cancellations SET payload='{}'::bytea; ALTER TABLE argus_catalog.performance_cancellations ENABLE TRIGGER trg_performance_cancellation_immutable`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.GetCancellation(t.Context(), data.Scope, pending.After, verifyStoreCancellation)
	if !errors.Is(err, requestintent.ErrUnavailable) || got.Fingerprint() != "" {
		t.Fatal("corrupt evidence returned", err)
	}
}

// Seed the released claim schema without invoking a post-migration selection query.
func seedReleasedClaim(t *testing.T, c *PerformanceStore, scope requestintent.Scope) (requestintent.Claim, error) {
	t.Helper()
	pending, err := c.PendingRequests(t.Context(), scope, 0, 1)
	if err != nil {
		return requestintent.Claim{}, err
	}
	if len(pending) != 1 {
		return requestintent.Claim{}, requestintent.ErrNotFound
	}
	claim := requestintent.Claim{Pending: pending[0], Fence: 1, Token: uuid.NewString()}
	err = c.runtime.pool.QueryRow(t.Context(), `INSERT INTO argus_catalog.performance_dispatch_claims (position,issuer,principal_issuer,principal_id,fence,token,available_at) VALUES ($1,$2,$3,$4,1,$5,clock_timestamp()+INTERVAL '1 minute') RETURNING available_at`, claim.Pending.After, scope.Issuer, scope.PrincipalIssuer, scope.PrincipalID, claim.Token).Scan(&claim.Until)

	return claim, err
}
