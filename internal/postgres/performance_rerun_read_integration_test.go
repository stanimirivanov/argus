//go:build integration

package postgres

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

func rerunReadFixture(t *testing.T) (rerunSchemaFixture, requestintent.RerunRecord) {
	t.Helper()
	f := releasedRerunSchema(t)
	data := requestFixture(t, f.store, f.data.CatalogSHA256, "readback")
	data.Children = data.Children[:1]
	successor := inwardRequest(t, data)
	claim := f.parents[0]
	event := schemaLifecycle(t, f.data.Scope, claim, "terminal-"+claim.Pending.Child.EventID, 2, requestintent.LifecycleCompleted, "run-"+claim.Pending.Child.Request.ID)
	admission, err := requestintent.NewLifecycleAdmission(requestintent.LifecycleAdmissionData{Evidence: outcomeFixture(t, f.data.Scope, claim, "accepted"), Run: event.Data().Run, RequestSHA256: claim.Pending.Child.SHA256})
	if err != nil {
		t.Fatal(err)
	}
	proof, err := requestintent.NewRerun(requestintent.RerunParent{Pending: claim.Pending, Intent: inwardRequest(t, f.data), Admission: admission, History: []requestintent.LifecycleEvent{event}}, successor, 0)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := f.store.pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }() //nolint:errcheck // Best effort after commit or failed fixture.
	if err := insertRequest(t.Context(), tx, successor.Data(), successor.Fingerprint()); err != nil {
		t.Fatal(err)
	}
	row := rerunSchemaRow(t, tx, claim.Pending.After, successor.Fingerprint())
	row["fingerprint"] = proof.Fingerprint()
	if err := insertRerunSchema(t.Context(), tx, row); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}

	return f, requestintent.RerunRecord{SuccessorPosition: row["successor_position"].(int64), Evidence: proof}
}

func TestRerunReadsAreScopedHistoricalReadOnlyAndReopenable(t *testing.T) {
	f, want := rerunReadFixture(t)
	config := f.store.pool.Config()
	config.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	pool, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var readOnly string
	if err := pool.QueryRow(t.Context(), "SHOW default_transaction_read_only").Scan(&readOnly); err != nil || readOnly != "on" {
		t.Fatal("connection not read-only", err)
	}
	reader := (&Runtime{pool: pool}).Performance()
	assertScopedRerunRead(t, reader.RerunSuccessor, f.data.Scope, want.Evidence.Data().ParentPosition, want)
	assertScopedRerunRead(t, reader.RerunPredecessor, f.data.Scope, want.SuccessorPosition, want)
	claim := f.parents[0]
	event := schemaLifecycle(t, f.data.Scope, claim, "zzz-later-terminal", 2, requestintent.LifecycleCompleted, "run-"+claim.Pending.Child.Request.ID)
	if err := insertLifecycleSchema(t.Context(), f.store.runtime.Performance(), f.data.Scope, claim.Pending.After, outcomeFixture(t, f.data.Scope, claim, "accepted"), event); err != nil {
		t.Fatal(err)
	}
	reopened := openTestStore(t, f.store.pool.Config().ConnString()).runtime.Performance()
	got, err := reopened.RerunSuccessor(t.Context(), f.data.Scope, claim.Pending.After)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal("later history/reopen rewrote captured proof", err)
	}
	if got, err := reader.RerunSuccessor(t.Context(), f.data.Scope, f.parents[1].Pending.After); !errors.Is(err, requestintent.ErrNotFound) || got.SuccessorPosition != 0 {
		t.Fatal("unknown link fabricated", err)
	}
}

func assertScopedRerunRead(t *testing.T, read func(context.Context, requestintent.Scope, int64) (requestintent.RerunRecord, error), scope requestintent.Scope, position int64, want requestintent.RerunRecord) {
	t.Helper()
	if got, err := read(t.Context(), scope, position); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal("read changed evidence", err)
	}
	for _, other := range []requestintent.Scope{{Issuer: "other", PrincipalIssuer: scope.PrincipalIssuer, PrincipalID: scope.PrincipalID}, {Issuer: scope.Issuer, PrincipalIssuer: "other", PrincipalID: scope.PrincipalID}, {Issuer: scope.Issuer, PrincipalIssuer: scope.PrincipalIssuer, PrincipalID: "other"}} {
		if got, err := read(t.Context(), other, position); !errors.Is(err, requestintent.ErrNotFound) || got.Evidence.Fingerprint() != "" {
			t.Fatal("scope leaked lineage", err)
		}
	}
}

func TestRerunReadsRejectCorruptProjectionAndRelationalPins(t *testing.T) {
	for _, test := range []struct{ name, table, trigger, mutation string }{
		{"fingerprint", "performance_rerun_links", "trg_performance_rerun_immutable", `UPDATE argus_catalog.performance_rerun_links SET fingerprint=repeat('d',64)`},
		{"terminal-pin", "performance_lifecycle_events", "trg_performance_lifecycle_immutable", `UPDATE argus_catalog.performance_lifecycle_events SET run_id='corrupt'`},
	} {
		t.Run(test.name, func(t *testing.T) {
			f, want := rerunReadFixture(t)
			tx, err := f.store.pool.Begin(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback(context.Background()) }() //nolint:errcheck // Isolated corruption fixture, never runtime behavior.
			for _, sql := range []string{"ALTER TABLE argus_catalog." + test.table + " DISABLE TRIGGER " + test.trigger, test.mutation, "ALTER TABLE argus_catalog." + test.table + " ENABLE TRIGGER " + test.trigger} {
				if _, err := tx.Exec(t.Context(), sql); err != nil {
					t.Fatal(err)
				}
			}
			if err := tx.Commit(t.Context()); err != nil {
				t.Fatal(err)
			}
			got, err := f.store.runtime.Performance().RerunSuccessor(t.Context(), f.data.Scope, want.Evidence.Data().ParentPosition)
			if !errors.Is(err, requestintent.ErrUnavailable) || got.Evidence.Fingerprint() != "" || got.SuccessorPosition != 0 {
				t.Fatal("corrupt lineage returned partial output", err)
			}
		})
	}
}
