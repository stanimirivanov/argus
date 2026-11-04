//go:build integration

package postgres

import (
	"context"
	"net/url"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

func TestEvidenceRuntimeRequiresRestrictedCredentialsAndPreservesSnapshot(t *testing.T) {
	f, event, verify := analysisStorageFixture(t)
	admin := f.store.runtime.pool.Config().ConnString()
	if opened, err := OpenEvidenceRuntime(t.Context(), admin); err == nil {
		opened.Close()
		t.Fatal("administrative login accepted")
	}
	role := "argus_evidence_" + randomHex(t, 8)
	password := randomHex(t, 24)
	identifier := pgx.Identifier{role}.Sanitize()
	// Role/password values are generated fixture tokens, never external input.
	if _, err := f.store.runtime.pool.Exec(t.Context(), "CREATE ROLE "+identifier+" LOGIN PASSWORD '"+password+"'"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := f.store.runtime.pool.Exec(context.Background(), "DROP OWNED BY "+identifier+"; DROP ROLE "+identifier); err != nil {
			t.Error(err)
		}
	})
	if _, err := f.store.runtime.pool.Exec(t.Context(), "GRANT USAGE ON SCHEMA argus_catalog TO "+identifier+"; GRANT SELECT ON argus_catalog.performance_request_intents,argus_catalog.performance_pending_dispatches,argus_catalog.performance_dispatch_outcomes,argus_catalog.performance_lifecycle_events,argus_catalog.performance_analysis_events TO "+identifier); err != nil {
		t.Fatal(err)
	}
	endpoint, err := url.Parse(admin)
	if err != nil {
		t.Fatal(err)
	}
	endpoint.User = url.UserPassword(role, password)
	reader, err := OpenEvidenceRuntime(t.Context(), endpoint.String())
	if err != nil {
		t.Fatal("restricted role refused", err)
	}
	admit := func(child requestintent.Child, outcome requestintent.Outcome) (requestintent.LifecycleAdmission, error) {
		if child.SHA256 != f.claim.Pending.Child.SHA256 || outcome.Fingerprint() != event.Data().Admission.Data().Evidence.Fingerprint() {
			return requestintent.LifecycleAdmission{}, requestintent.ErrInvalid
		}
		return event.Data().Admission, nil
	}
	snapshot, err := reader.Performance().DecisionSnapshot(t.Context(), f.scope, f.claim.Pending.After, admit, verify, projectionFixture)
	if err != nil || snapshot.Child.SHA256 != f.claim.Pending.Child.SHA256 {
		reader.Close()
		t.Fatal("restricted snapshot failed", err)
	}
	if _, err := reader.pool.Exec(t.Context(), `UPDATE argus_catalog.performance_pending_dispatches SET event_id=event_id`); err == nil {
		reader.Close()
		t.Fatal("read session wrote data")
	}
	reader.Close()
	if _, err := f.store.runtime.pool.Exec(t.Context(), "GRANT UPDATE(payload) ON argus_catalog.performance_pending_dispatches TO "+identifier); err != nil {
		t.Fatal(err)
	}
	if opened, err := OpenEvidenceRuntime(t.Context(), endpoint.String()); err == nil {
		opened.Close()
		t.Fatal("column writer accepted")
	}
}
