//go:build integration

package postgres

import (
	"context"
	"errors"
	"maps"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

type analysisSchemaFixture struct {
	store     *PerformanceStore
	scope     requestintent.Scope
	claim     requestintent.Claim
	admission requestintent.Outcome
	row       pgx.NamedArgs
}

func newAnalysisSchemaFixture(t *testing.T, kind string) analysisSchemaFixture {
	t.Helper()
	url := newTestDatabase(t)
	migrator := openTestMigrator(t, url)
	chain, err := loadMigrations(embeddedMigrations)
	if err != nil {
		t.Fatal(err)
	}
	released := fstest.MapFS{}
	for _, migration := range chain {
		if migration.name < "20261030195441_add_performance_analysis_evidence.sql" {
			released["migrations/"+migration.name] = &fstest.MapFile{Data: migration.contents}
		}
	}
	if err := applyMigrationChain(t.Context(), migrator, released); err != nil {
		t.Fatal(err)
	}
	persisted := openTestStore(t, url)
	store := persisted.runtime.Performance()
	snapshot := performanceFixture(t, persisted, "pages", "analysis-schema", -time.Minute)
	if _, err := store.Publish(t.Context(), snapshot); err != nil {
		t.Fatal(err)
	}
	data := requestFixture(t, persisted, snapshot.Fingerprint(), "analysis-schema")
	if _, err := store.CreateRequest(t.Context(), inwardRequest(t, data)); err != nil {
		t.Fatal(err)
	}
	claim, err := store.ClaimRequest(t.Context(), data.Scope, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	admission := outcomeFixture(t, data.Scope, claim, kind)
	if _, err := store.SettleRequest(t.Context(), data.Scope, claim, admission); err != nil {
		t.Fatal(err)
	}
	if err := migrator.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got, err := store.RequestOutcome(t.Context(), data.Scope, claim.Pending.After); err != nil || got.Fingerprint() != admission.Fingerprint() {
		t.Fatal("upgrade changed admission", err)
	}
	row := pgx.NamedArgs{"position": claim.Pending.After, "issuer": data.Scope.Issuer, "principal_issuer": data.Scope.PrincipalIssuer, "principal_id": data.Scope.PrincipalID,
		"admission_fingerprint": admission.Fingerprint(), "request_sha256": claim.Pending.Child.SHA256, "run_id": "run", "event_id": "analysis", "fingerprint": strings.Repeat("f", 64),
		"published_at": claim.Pending.Child.Deadline.Truncate(time.Microsecond), "published_submicro_ns": int16(789), "publication": []byte(" {} "), "review": []byte(" {} "), "report": []byte(" {} ")}

	return analysisSchemaFixture{store, data.Scope, claim, admission, row}
}

// These are structural SQL fixtures. Native wire/conformance verification stays
// in adapter tests; publication/review/report bytes are deliberately opaque JSON.
func insertAnalysisSchema(ctx context.Context, executor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, row pgx.NamedArgs) error {
	_, err := executor.Exec(ctx, `INSERT INTO argus_catalog.performance_analysis_events
 (position,issuer,principal_issuer,principal_id,admission_fingerprint,request_sha256,run_id,event_id,fingerprint,
 published_at,published_submicro_ns,publication,review,report,publication_sha256,review_sha256,report_sha256)
 VALUES (@position,@issuer,@principal_issuer,@principal_id,@admission_fingerprint,@request_sha256,@run_id,@event_id,@fingerprint,
 @published_at,@published_submicro_ns,@publication,@review,@report,encode(sha256(@publication::bytea),'hex'),encode(sha256(@review::bytea),'hex'),encode(sha256(@report::bytea),'hex'))`, row)

	return err
}

func assertAnalysisSQL(t *testing.T, err error, code, constraint string) {
	t.Helper()
	var failure *pgconn.PgError
	if !errors.As(err, &failure) || failure.Code != code || (constraint != "" && failure.ConstraintName != constraint) {
		t.Fatal("unexpected analysis SQL result", err)
	}
}

func TestAnalysisSchemaGuardsBindingsBytesAndRollback(t *testing.T) {
	f := newAnalysisSchemaFixture(t, "accepted")
	for _, test := range []struct {
		field      string
		value      any
		constraint string
	}{
		{"principal_id", "other", "ck_performance_analysis_binding"},
		{"admission_fingerprint", strings.Repeat("a", 64), "ck_performance_analysis_binding"},
		{"request_sha256", strings.Repeat("b", 64), "ck_performance_analysis_binding"},
		{"published_at", f.claim.Pending.Child.RequestedAt.Add(-time.Second), "ck_performance_analysis_binding"},
		{"published_submicro_ns", int16(1000), "ck_performance_analysis_bounds"},
		{"fingerprint", "bad", "ck_performance_analysis_bounds"},
		{"event_id", " ", "ck_performance_analysis_bounds"},
		{"run_id", " ", "ck_performance_analysis_bounds"},
		{"publication", []byte(`"` + strings.Repeat("x", 65536) + `"`), "ck_performance_analysis_bounds"},
		{"review", []byte(`"` + strings.Repeat("x", 2097152) + `"`), "ck_performance_analysis_bounds"},
		{"report", []byte(`"` + strings.Repeat("x", 1048576) + `"`), "ck_performance_analysis_bounds"},
	} {
		row := maps.Clone(f.row)
		row[test.field] = test.value
		code := "23514"
		if test.constraint == "ck_performance_analysis_binding" {
			code = "55000"
		}
		assertAnalysisSQL(t, insertAnalysisSchema(t.Context(), f.store.runtime.pool, row), code, test.constraint)
	}
	row := maps.Clone(f.row)
	row["report"] = []byte(`{`)
	assertAnalysisSQL(t, insertAnalysisSchema(t.Context(), f.store.runtime.pool, row), "22P02", "")
	tx, err := f.store.runtime.pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	cleanupAnalysisSchemaTx(t, tx)
	if err := insertAnalysisSchema(t.Context(), tx, f.row); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(t.Context()); err != nil {
		t.Fatal(err)
	}
	stale, err := f.store.runtime.pool.BeginTx(t.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	cleanupAnalysisSchemaTx(t, stale)
	assertAnalysisSQL(t, insertAnalysisSchema(t.Context(), stale, f.row), "25000", "ck_performance_analysis_isolation")
	if err := stale.Rollback(t.Context()); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := f.store.runtime.pool.QueryRow(t.Context(), `SELECT count(*) FROM argus_catalog.performance_analysis_events`).Scan(&count); err != nil || count != 0 {
		t.Fatal("rollback leaked evidence", err, count)
	}
}

func TestAnalysisSchemaConcurrentUniquenessPrecisionAndReopen(t *testing.T) {
	f := newAnalysisSchemaFixture(t, "accepted")
	results := make(chan error, 2)
	for range 2 {
		go func() { results <- insertAnalysisSchema(t.Context(), f.store.runtime.pool, maps.Clone(f.row)) }()
	}
	a, b := <-results, <-results
	if a == nil {
		a, b = b, a
	}
	if b != nil {
		t.Fatal("no insert succeeded", b)
	}
	assertAnalysisSQL(t, a, "23505", "")
	for _, statement := range []string{`UPDATE argus_catalog.performance_analysis_events SET fingerprint=fingerprint`, `DELETE FROM argus_catalog.performance_analysis_events`, `TRUNCATE argus_catalog.performance_analysis_events`} {
		_, err := f.store.runtime.pool.Exec(t.Context(), statement)
		assertAnalysisSQL(t, err, "55000", "")
	}
	_, err := f.store.runtime.pool.Exec(t.Context(), `INSERT INTO argus_catalog.performance_analysis_events
 SELECT position,issuer,principal_issuer,principal_id,admission_fingerprint,request_sha256,run_id,'bad-digest',event_sha256,fingerprint,
 published_at,published_submicro_ns,publication,review,report,repeat('0',64),review_sha256,report_sha256,recorded_at FROM argus_catalog.performance_analysis_events`)
	assertAnalysisSQL(t, err, "23514", "ck_performance_analysis_digests")
	changed := schemaLifecycle(t, f.scope, f.claim, "later-run", 0, requestintent.LifecycleRunning, "other")
	assertAnalysisSQL(t, insertLifecycleSchema(t.Context(), f.store, f.scope, f.claim.Pending.After, f.admission, changed), "55000", "ck_performance_analysis_binding")
	url := f.store.runtime.pool.Config().ConnString()
	f.store.runtime.Close()
	reopened := openTestStore(t, url)
	var at time.Time
	var remainder int16
	var report []byte
	if err := reopened.pool.QueryRow(t.Context(), `SELECT published_at,published_submicro_ns,report FROM argus_catalog.performance_analysis_events`).Scan(&at, &remainder, &report); err != nil || !at.Equal(f.row["published_at"].(time.Time)) || remainder != 789 || string(report) != " {} " {
		t.Fatal("precision/original bytes lost", err)
	}
}

func TestAnalysisSchemaRejectsCrossFamilyEventsAndRunChanges(t *testing.T) {
	f := newAnalysisSchemaFixture(t, "accepted")
	row := maps.Clone(f.row)
	row["event_id"] = f.admission.Data().EventID
	assertAnalysisSQL(t, insertAnalysisSchema(t.Context(), f.store.runtime.pool, row), "23505", "uq_performance_analysis_event")
	event := schemaLifecycle(t, f.scope, f.claim, "running", 0, requestintent.LifecycleRunning, "run")
	if err := insertLifecycleSchema(t.Context(), f.store, f.scope, f.claim.Pending.After, f.admission, event); err != nil {
		t.Fatal(err)
	}
	row["event_id"] = "running"
	assertAnalysisSQL(t, insertAnalysisSchema(t.Context(), f.store.runtime.pool, row), "23505", "uq_performance_analysis_event")
	row["event_id"] = "other-run"
	row["run_id"] = "other"
	assertAnalysisSQL(t, insertAnalysisSchema(t.Context(), f.store.runtime.pool, row), "55000", "ck_performance_analysis_binding")
	if err := insertAnalysisSchema(t.Context(), f.store.runtime.pool, f.row); err != nil {
		t.Fatal(err)
	}
	collision := schemaLifecycle(t, f.scope, f.claim, "analysis", 1, requestintent.LifecycleRunning, "run")
	assertAnalysisSQL(t, insertLifecycleSchema(t.Context(), f.store, f.scope, f.claim.Pending.After, f.admission, collision), "23505", "uq_performance_analysis_event")
	changed := schemaLifecycle(t, f.scope, f.claim, "new-run", 1, requestintent.LifecycleRunning, "other")
	assertAnalysisSQL(t, insertLifecycleSchema(t.Context(), f.store, f.scope, f.claim.Pending.After, f.admission, changed), "55000", "")
	second, err := f.store.ClaimRequest(t.Context(), f.scope, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	data := outcomeFixture(t, f.scope, second, "accepted").Data()
	data.EventID = "analysis"
	admission, err := requestintent.NewOutcome(data)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.store.SettleRequest(t.Context(), f.scope, second, admission)
	if !errors.Is(err, requestintent.ErrConflict) {
		t.Fatal("analysis event reuse not classified as conflict", err)
	}
}

func TestAnalysisSchemaRejectsRejectedAdmissionAndBoundsHistory(t *testing.T) {
	rejected := newAnalysisSchemaFixture(t, "rejected")
	assertAnalysisSQL(t, insertAnalysisSchema(t.Context(), rejected.store.runtime.pool, rejected.row), "55000", "ck_performance_analysis_binding")
	f := newAnalysisSchemaFixture(t, "accepted")
	unadmitted, err := f.store.ClaimRequest(t.Context(), f.scope, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	missing := maps.Clone(f.row)
	missing["position"], missing["request_sha256"], missing["published_at"] = unadmitted.Pending.After, unadmitted.Pending.Child.SHA256, unadmitted.Pending.Child.Deadline
	assertAnalysisSQL(t, insertAnalysisSchema(t.Context(), f.store.runtime.pool, missing), "55000", "ck_performance_analysis_binding")
	if err := insertAnalysisSchema(t.Context(), f.store.runtime.pool, f.row); err != nil {
		t.Fatal(err)
	}
	_, err = f.store.runtime.pool.Exec(t.Context(), `INSERT INTO argus_catalog.performance_analysis_events
 SELECT position,issuer,principal_issuer,principal_id,admission_fingerprint,request_sha256,run_id,'repeat-'||n,event_sha256,fingerprint,
 published_at,published_submicro_ns,publication,review,report,publication_sha256,review_sha256,report_sha256,recorded_at
 FROM argus_catalog.performance_analysis_events CROSS JOIN generate_series(1,999) n`)
	if err != nil {
		t.Fatal(err)
	}
	row := maps.Clone(f.row)
	row["event_id"] = "overflow"
	assertAnalysisSQL(t, insertAnalysisSchema(t.Context(), f.store.runtime.pool, row), "55000", "ck_performance_analysis_capacity")
}

func cleanupAnalysisSchemaTx(t *testing.T, tx pgx.Tx) {
	t.Helper()
	t.Cleanup(func() {
		if err := tx.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Error(err)
		}
	})
}
