//go:build integration

package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func insertRequestSchemaChild(ctx context.Context, tx pgx.Tx, id uuid.UUID, ordinal int, principal string, at time.Time, ids [3]string) error {
	_, err := tx.Exec(ctx, `INSERT INTO argus_catalog.performance_pending_dispatches
        (intent_uuid,ordinal,issuer,principal_issuer,principal_id,request_issuer,request_id,attempt_issuer,attempt_id,event_id,requested_at,deadline,payload,payload_sha256,reasons)
        VALUES ($1,$2,'https://provider.example/identity','https://auth.example/tenant',$3,'https://argus.example',$4,'https://argus.example',$5,$6,$7,$8,$9,$10,$11)`, id, ordinal, principal, ids[0], ids[1], ids[2], at, at.Add(time.Minute), []byte(`{}`), strings.Repeat("a", 64), `[{"MappingID":"mapping","Capability":"checkout","Reason":"approved"}]`)

	return err
}

func requestSchemaIDs(name string, n int) [3]string {
	ids := [3]string{fmt.Sprintf("request-%d", n), fmt.Sprintf("attempt-%d", n), fmt.Sprintf("event-%d", n)}
	switch name {
	case "duplicate-request":
		ids[0] = "request-0"
	case "duplicate-attempt":
		ids[1] = "attempt-0"
	case "duplicate-event":
		ids[2] = "event-0"
	case "unicode-identities":
		ids = [3]string{strings.Repeat("請", 256), strings.Repeat("試", 256), strings.Repeat("事", 256)}
	}

	return ids
}

type requestSchemaCase struct {
	name             string
	count, children  int
	shift            time.Duration
	gap, wrongScope  bool
	code, constraint string
}

func TestPerformanceRequestSchemaSealsCompleteScopedAssembly(t *testing.T) {
	for _, test := range []requestSchemaCase{
		{"complete", 2, 2, 0, false, false, "", ""},
		{"unicode-identities", 1, 1, 0, false, false, "", ""},
		{"incomplete", 2, 1, 0, false, false, "23514", "ck_performance_request_completeness"},
		{"ordinal-gap", 2, 2, 0, true, false, "23514", "ck_performance_request_completeness"},
		{"expired", 1, 1, -time.Hour, false, false, "23514", "ck_performance_request_freshness"},
		{"future", 1, 1, time.Hour, false, false, "23514", "ck_performance_request_freshness"},
		{"cross-scope", 1, 1, 0, false, true, "23503", "fk_performance_dispatch_scope"},
		{"duplicate-request", 2, 2, 0, false, false, "23505", "uq_performance_dispatch_request"},
		{"duplicate-attempt", 2, 2, 0, false, false, "23505", "uq_performance_dispatch_attempt"},
		{"duplicate-event", 2, 2, 0, false, false, "23505", "uq_performance_dispatch_event"},
		{"changed-catalog", 1, 1, 0, false, false, "23514", "ck_performance_request_catalog"},
	} {
		t.Run(test.name, func(t *testing.T) { t.Parallel(); runRequestSchemaCase(t, test) })
	}
}

func requestSchemaStore(t *testing.T, name string) (*integratedStore, string) {
	t.Helper()
	store := migratedTestStore(t)
	snapshot := performanceFixture(t, store, "pages", "request-schema", -time.Minute)
	if _, err := store.runtime.Performance().Publish(t.Context(), snapshot); err != nil {
		t.Fatal(err)
	}
	if name == "changed-catalog" {
		newer := performanceFixture(t, store, "empty", "replacement", 0)
		if _, err := store.runtime.Performance().Publish(t.Context(), newer); err != nil {
			t.Fatal(err)
		}
	}

	return store, snapshot.Fingerprint()
}

func runRequestSchemaCase(t *testing.T, test requestSchemaCase) {
	t.Helper()
	store, fingerprint := requestSchemaStore(t, test.name)
	tx, err := store.pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(t.Context()) }() //nolint:errcheck // Best effort after commit or failure.
	id := uuid.New()
	var at time.Time
	if err := tx.QueryRow(t.Context(), "SELECT clock_timestamp()").Scan(&at); err != nil {
		t.Fatal(err)
	}
	at = at.Add(test.shift)
	_, err = tx.Exec(t.Context(), `INSERT INTO argus_catalog.performance_request_intents
                (intent_uuid,issuer,principal_issuer,principal_id,review_sha256,catalog_sha256,fingerprint,catalog_uuid,evidence,child_count)
                SELECT $1,issuer,principal_issuer,principal_id,$2,$3,$2,snapshot_uuid,$4,$5 FROM argus_catalog.performance_catalog_snapshots WHERE source_id='request-schema'`, id, strings.Repeat("b", 64), fingerprint, []byte(`{"retained":true}`), test.count)
	if err != nil {
		t.Fatal(err)
	}
	for n := 0; n < test.children; n++ {
		ordinal := n
		if test.gap {
			ordinal *= 2
		}
		principal := "argus-service"
		if test.wrongScope {
			principal = "other"
		}
		err = insertRequestSchemaChild(t.Context(), tx, id, ordinal, principal, at, requestSchemaIDs(test.name, ordinal))
		if err != nil {
			break
		}
	}
	if err == nil {
		err = tx.Commit(t.Context())
	}
	if test.code != "" {
		var database *pgconn.PgError
		if !errors.As(err, &database) || database.Code != test.code || database.ConstraintName != test.constraint {
			t.Fatalf("expected %s/%s: %v", test.code, test.constraint, err)
		}
		var remaining int
		if err := store.pool.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM argus_catalog.performance_request_intents)+(SELECT count(*) FROM argus_catalog.performance_pending_dispatches)`).Scan(&remaining); err != nil || remaining != 0 {
			t.Fatalf("partial assembly remained: %d, %v", remaining, err)
		}

		return
	}
	if err != nil {
		t.Fatal(err)
	}
	verifyRequestSealed(t, store, id, at)
}

func verifyRequestSealed(t *testing.T, store *integratedStore, id uuid.UUID, at time.Time) {
	t.Helper()
	for _, statement := range []string{`UPDATE argus_catalog.performance_request_intents SET fingerprint=fingerprint`, `DELETE FROM argus_catalog.performance_pending_dispatches`} {
		if _, err := store.pool.Exec(t.Context(), statement); err == nil {
			t.Fatal("sealed evidence changed")
		}
	}
	late, err := store.pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = late.Rollback(t.Context()) }() //nolint:errcheck // Best effort after rejected insertion.
	var database *pgconn.PgError
	if err := insertRequestSchemaChild(t.Context(), late, id, 2, "argus-service", at, requestSchemaIDs("", 2)); !errors.As(err, &database) || database.Code != "55000" {
		t.Fatalf("late child accepted: %v", err)
	}
}
