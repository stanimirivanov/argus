//go:build integration

package postgres

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

type rerunSchemaFixture struct {
	store   *integratedStore
	data    requestintent.Data
	parents []requestintent.Claim
}

func releasedRerunSchema(t *testing.T) rerunSchemaFixture {
	t.Helper()
	url := newTestDatabase(t)
	migrator := openTestMigrator(t, url)
	chain, err := loadMigrations(embeddedMigrations)
	if err != nil {
		t.Fatal(err)
	}
	released := fstest.MapFS{}
	for _, migration := range chain {
		if migration.name < "20261029063757_add_performance_rerun_lineage.sql" {
			released["migrations/"+migration.name] = &fstest.MapFile{Data: migration.contents}
		}
	}
	if err := applyMigrationChain(t.Context(), migrator, released); err != nil {
		t.Fatal(err)
	}
	store := openTestStore(t, url)
	c := store.runtime.Performance()
	snapshot := performanceFixture(t, store, "pages", "rerun-upgrade", -time.Minute)
	if _, err := c.Publish(t.Context(), snapshot); err != nil {
		t.Fatal(err)
	}
	data := requestFixture(t, store, snapshot.Fingerprint(), "parent")
	intent := inwardRequest(t, data)
	if _, err := c.CreateRequest(t.Context(), intent); err != nil {
		t.Fatal(err)
	}
	fixture := rerunSchemaFixture{store: store, data: data}
	for _, state := range []requestintent.LifecycleState{requestintent.LifecycleCompleted, requestintent.LifecycleCancelled} {
		claim, err := c.ClaimRequest(t.Context(), data.Scope, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		admission := outcomeFixture(t, data.Scope, claim, "accepted")
		if _, err := c.SettleRequest(t.Context(), data.Scope, claim, admission); err != nil {
			t.Fatal(err)
		}
		event := schemaLifecycle(t, data.Scope, claim, "terminal-"+claim.Pending.Child.EventID, 2, state, "run-"+claim.Pending.Child.Request.ID)
		if err := insertLifecycleSchema(t.Context(), c, data.Scope, claim.Pending.After, admission, event); err != nil {
			t.Fatal(err)
		}
		fixture.parents = append(fixture.parents, claim)
	}
	if err := migrator.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	retained, err := c.GetRequest(t.Context(), data.Scope, data.ReviewSHA256)
	if err != nil || retained.Fingerprint() != intent.Fingerprint() {
		t.Fatal("upgrade changed parent", err)
	}

	return fixture
}

func rerunSchemaAssembly(t *testing.T, f rerunSchemaFixture, name string, count int, changes ...func(*requestintent.Data)) (pgx.Tx, map[string]any) {
	t.Helper()
	data := requestFixture(t, f.store, f.data.CatalogSHA256, name)
	data.Children = data.Children[:count]
	for _, change := range changes {
		change(&data)
	}
	intent := inwardRequest(t, data)
	tx, err := f.store.pool.BeginTx(t.Context(), pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) }) //nolint:errcheck // Best effort after commit or failed SQL.
	if err := insertRequest(t.Context(), tx, intent.Data(), intent.Fingerprint()); err != nil {
		t.Fatal(err)
	}

	return tx, rerunSchemaRow(t, tx, f.parents[0].Pending.After, intent.Fingerprint())
}

// Structural SQL fixtures keep canonical wire/fingerprint verification at the
// existing domain/adapter tests. Left joins expose missing admission/history.
func rerunSchemaRow(t *testing.T, tx pgx.Tx, parent int64, successor string, ordinals ...int) map[string]any {
	t.Helper()
	ordinal := 0
	if len(ordinals) > 0 {
		ordinal = ordinals[0]
	}
	rows, err := tx.Query(t.Context(), `SELECT p.position AS parent_position,c.position AS successor_position,p.issuer,p.principal_issuer,p.principal_id,
        pi.fingerprint AS parent_intent_sha256,p.payload_sha256 AS parent_request_sha256,
        COALESCE(a.fingerprint,repeat('e',64)) AS parent_admission_sha256,COALESCE(h.fingerprint,repeat('e',64)) AS parent_lifecycle_sha256,
        COALESCE(h.run_id,'missing') AS parent_run_id,COALESCE(h.revision,0) AS parent_revision,COALESCE(h.state,'COMPLETED') AS parent_state,
        COALESCE(h.occurred_at,p.deadline) AS parent_occurred_at,p.deadline AS parent_deadline,
        ci.intent_uuid AS successor_intent_uuid,ci.fingerprint AS successor_intent_sha256,ci.review_sha256 AS successor_review_sha256,
        c.ordinal AS successor_ordinal,c.payload_sha256 AS successor_request_sha256,repeat('f',64) AS fingerprint
        FROM argus_catalog.performance_pending_dispatches p JOIN argus_catalog.performance_request_intents pi ON pi.intent_uuid=p.intent_uuid
        JOIN argus_catalog.performance_request_intents ci ON ci.fingerprint=$2
        JOIN argus_catalog.performance_pending_dispatches c ON c.intent_uuid=ci.intent_uuid AND c.ordinal=$3
        LEFT JOIN argus_catalog.performance_dispatch_outcomes a ON a.position=p.position
        LEFT JOIN LATERAL (SELECT * FROM argus_catalog.performance_lifecycle_events WHERE position=p.position ORDER BY revision DESC,event_id COLLATE "C" DESC LIMIT 1) h ON true
        WHERE p.position=$1`, parent, successor, ordinal)
	if err != nil {
		t.Fatal(err)
	}
	row, err := pgx.CollectExactlyOneRow(rows, pgx.RowToMap)
	if err != nil {
		t.Fatal(err)
	}

	return row
}

func insertRerunSchema(ctx context.Context, tx pgx.Tx, row map[string]any) error {
	keys := make([]string, 0, len(row))
	for key := range row {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	args, binds := make([]any, 0, len(keys)), make([]string, 0, len(keys))
	for i, key := range keys {
		args = append(args, row[key])
		binds = append(binds, "$"+strconv.Itoa(i+1))
	}
	_, err := tx.Exec(ctx, `INSERT INTO argus_catalog.performance_rerun_links (`+strings.Join(keys, ",")+`) VALUES (`+strings.Join(binds, ",")+`)`, args...)

	return err
}

func assertRerunSQL(t *testing.T, err error, code, constraint string) {
	t.Helper()
	var database *pgconn.PgError
	if !errors.As(err, &database) || database.Code != code || (constraint != "" && database.ConstraintName != constraint) {
		t.Fatal("unexpected lineage result", err)
	}
}

func TestRerunSchemaGuardsPinsAssemblyAndRollback(t *testing.T) {
	f := releasedRerunSchema(t)
	for i, test := range []struct {
		column     string
		value      any
		constraint string
	}{
		{"principal_id", "other", "ck_performance_rerun_binding"},
		{"issuer", "other", "ck_performance_rerun_binding"},
		{"parent_intent_sha256", strings.Repeat("d", 64), "ck_performance_rerun_binding"},
		{"parent_admission_sha256", strings.Repeat("d", 64), "ck_performance_rerun_binding"},
		{"successor_request_sha256", strings.Repeat("d", 64), "ck_performance_rerun_binding"},
		{"successor_ordinal", int32(3), "ck_performance_rerun_binding"},
		{"parent_lifecycle_sha256", strings.Repeat("d", 64), "ck_performance_rerun_terminal"},
		{"parent_revision", int64(1), "ck_performance_rerun_terminal"},
		{"parent_state", "FAILED", "ck_performance_rerun_terminal"},
		{"parent_run_id", "wrong", "ck_performance_rerun_terminal"},
		{"fingerprint", "bad", "ck_performance_rerun_digests"},
		{"parent_position", int64(0), "ck_performance_rerun_bounds"},
	} {
		t.Run(test.column, func(t *testing.T) {
			tx, row := rerunSchemaAssembly(t, f, fmt.Sprintf("invalid-%d", i), 1)
			row[test.column] = test.value
			assertRerunSQL(t, insertRerunSchema(t.Context(), tx, row), "23514", test.constraint)
		})
	}
	t.Run("partial-batch", func(t *testing.T) {
		tx, row := rerunSchemaAssembly(t, f, "partial", 2)
		if err := insertRerunSchema(t.Context(), tx, row); err != nil {
			t.Fatal(err)
		}
		assertRerunSQL(t, tx.Commit(t.Context()), "23514", "ck_performance_rerun_assembly")
	})
	var count int
	if err := f.store.pool.QueryRow(t.Context(), `SELECT count(*) FROM argus_catalog.performance_rerun_links`).Scan(&count); err != nil || count != 0 {
		t.Fatal("failed assembly leaked lineage", err)
	}
	tx, row := rerunSchemaAssembly(t, f, "rolled-back", 1)
	if err := insertRerunSchema(t.Context(), tx, row); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(t.Context()); err != nil {
		t.Fatal(err)
	}
	var children int
	if err := f.store.pool.QueryRow(t.Context(), `SELECT count(*) FROM argus_catalog.performance_pending_dispatches`).Scan(&children); err != nil || children != 2 {
		t.Fatal("rollback leaked successor", children, err)
	}
}

func TestRerunSchemaConcurrentForksAndImmutableLinks(t *testing.T) {
	f := releasedRerunSchema(t)
	first, a := rerunSchemaAssembly(t, f, "fork-a", 1)
	second, b := rerunSchemaAssembly(t, f, "fork-b", 1)
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, candidate := range []struct {
		tx  pgx.Tx
		row map[string]any
	}{{first, a}, {second, b}} {
		go func() {
			<-start
			err := insertRerunSchema(t.Context(), candidate.tx, candidate.row)
			if err == nil {
				err = candidate.tx.Commit(t.Context())
			}
			_ = candidate.tx.Rollback(context.Background()) //nolint:errcheck // Release failed writer before readback.
			results <- err
		}()
	}
	close(start)
	one, two := <-results, <-results
	if one == nil {
		one, two = two, one
	}
	if two != nil {
		t.Fatal("neither fork retained", one, two)
	}
	assertRerunSQL(t, one, "23505", "pk_performance_rerun_links")
	for _, sql := range []string{`UPDATE argus_catalog.performance_rerun_links SET fingerprint=fingerprint`, `DELETE FROM argus_catalog.performance_rerun_links`, `TRUNCATE argus_catalog.performance_rerun_links`} {
		_, err := f.store.pool.Exec(t.Context(), sql)
		assertRerunSQL(t, err, "55000", "")
	}
	c := f.store.runtime.Performance()
	middle, err := c.ClaimRequest(t.Context(), f.data.Scope, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	admission := outcomeFixture(t, f.data.Scope, middle, "accepted")
	if _, err := c.SettleRequest(t.Context(), f.data.Scope, middle, admission); err != nil {
		t.Fatal(err)
	}
	event := schemaLifecycle(t, f.data.Scope, middle, "chain-terminal", 1, requestintent.LifecycleFailed, "chain-run")
	if err := insertLifecycleSchema(t.Context(), c, f.data.Scope, middle.Pending.After, admission, event); err != nil {
		t.Fatal(err)
	}
	tail, row := rerunSchemaAssembly(t, f, "chain-tail", 1)
	row = rerunSchemaRow(t, tail, middle.Pending.After, row["successor_intent_sha256"].(string))
	if err := insertRerunSchema(t.Context(), tail, row); err != nil {
		t.Fatal(err)
	}
	if err := tail.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	reopened := openTestStore(t, f.store.pool.Config().ConnString())
	var count int
	if err := reopened.pool.QueryRow(t.Context(), `SELECT count(*) FROM argus_catalog.performance_rerun_links`).Scan(&count); err != nil || count != 2 {
		t.Fatal("reopen lost lineage", err)
	}
}

func TestRerunSchemaRejectsMergingCyclesReusedIdentitiesAndSealedAttachment(t *testing.T) {
	f := releasedRerunSchema(t)
	t.Run("merge", func(t *testing.T) {
		tx, row := rerunSchemaAssembly(t, f, "merge", 1)
		if err := insertRerunSchema(t.Context(), tx, row); err != nil {
			t.Fatal(err)
		}
		other := rerunSchemaRow(t, tx, f.parents[1].Pending.After, row["successor_intent_sha256"].(string))
		assertRerunSQL(t, insertRerunSchema(t.Context(), tx, other), "23505", "uq_performance_rerun_successor")
	})
	t.Run("cycle", func(t *testing.T) {
		tx, row := rerunSchemaAssembly(t, f, "cycle", 1)
		row["parent_position"] = row["successor_position"]
		assertRerunSQL(t, insertRerunSchema(t.Context(), tx, row), "23514", "ck_performance_rerun_bounds")
	})
	for _, name := range []string{"parent-role", "sibling-role"} {
		t.Run(name, func(t *testing.T) {
			tx, row := rerunSchemaAssembly(t, f, name, 2, func(d *requestintent.Data) {
				if name == "parent-role" {
					d.Children[0].Attempt = f.parents[0].Pending.Child.Request
				} else {
					d.Children[1].Attempt = d.Children[0].Request
				}
			})
			assertRerunSQL(t, insertRerunSchema(t.Context(), tx, row), "23514", "ck_performance_rerun_identity")
		})
	}
	tx, row := rerunSchemaAssembly(t, f, "sealed", 1)
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal("legacy creation incompatible", err)
	}
	late, err := f.store.pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = late.Rollback(context.Background()) }() //nolint:errcheck // Failed native guard leaves no changes.
	assertRerunSQL(t, insertRerunSchema(t.Context(), late, row), "23514", "ck_performance_rerun_assembly")
}

func TestRerunSchemaRequiresAdmittedTerminalParent(t *testing.T) {
	f := releasedRerunSchema(t)
	c := f.store.runtime.Performance()
	data := requestFixture(t, f.store, f.data.CatalogSHA256, "ineligible-parent")
	if _, err := c.CreateRequest(t.Context(), inwardRequest(t, data)); err != nil {
		t.Fatal(err)
	}
	pending, err := c.PendingRequests(t.Context(), data.Scope, f.parents[1].Pending.After, 100)
	if err != nil || len(pending) != 2 {
		t.Fatal("missing ineligible parents", err)
	}
	reject := func(t *testing.T, position int64, name, constraint string) {
		t.Helper()
		tx, row := rerunSchemaAssembly(t, f, name, 1)
		row = rerunSchemaRow(t, tx, position, row["successor_intent_sha256"].(string))
		assertRerunSQL(t, insertRerunSchema(t.Context(), tx, row), "23514", constraint)
	}
	t.Run("unadmitted", func(t *testing.T) { reject(t, pending[0].After, "unadmitted", "ck_performance_rerun_binding") })
	for _, kind := range []string{"rejected", "accepted"} {
		claim, err := c.ClaimRequest(t.Context(), data.Scope, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		admission := outcomeFixture(t, data.Scope, claim, kind)
		if _, err := c.SettleRequest(t.Context(), data.Scope, claim, admission); err != nil {
			t.Fatal(err)
		}
		constraint := "ck_performance_rerun_binding"
		if kind == "accepted" {
			constraint = "ck_performance_rerun_terminal"
		}
		t.Run(kind, func(t *testing.T) { reject(t, claim.Pending.After, kind, constraint) })
		if kind == "accepted" {
			running := schemaLifecycle(t, data.Scope, claim, "still-running", 0, requestintent.LifecycleRunning, "active-run")
			if err := insertLifecycleSchema(t.Context(), c, data.Scope, claim.Pending.After, admission, running); err != nil {
				t.Fatal(err)
			}
			t.Run("running", func(t *testing.T) { reject(t, claim.Pending.After, "running", "ck_performance_rerun_terminal") })
		}
	}
}

func TestRerunSchemaCommitsCompleteMultiParentBatch(t *testing.T) {
	f := releasedRerunSchema(t)
	tx, first := rerunSchemaAssembly(t, f, "complete-batch", 2)
	second := rerunSchemaRow(t, tx, f.parents[1].Pending.After, first["successor_intent_sha256"].(string), 1)
	for _, row := range []map[string]any{first, second} {
		if err := insertRerunSchema(t.Context(), tx, row); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal("complete batch rejected", err)
	}
}
