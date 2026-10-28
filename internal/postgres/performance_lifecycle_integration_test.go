//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/performance/adapters/perfeng"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

func TestAuthenticatedLifecycleRuntimeTraversesAndPersistsScopedEvidence(t *testing.T) {
	store, scope, claim, admission, verify := lifecycleStoreFixture(t, "accepted")
	issuer := claim.Pending.Child.Request.Issuer
	for _, wrong := range []requestintent.Scope{
		{Issuer: scope.Issuer + "/other", PrincipalIssuer: scope.PrincipalIssuer, PrincipalID: scope.PrincipalID},
		{Issuer: scope.Issuer, PrincipalIssuer: scope.PrincipalIssuer + "/other", PrincipalID: scope.PrincipalID},
		{Issuer: scope.Issuer, PrincipalIssuer: scope.PrincipalIssuer, PrincipalID: scope.PrincipalID + "-other"},
	} {
		if _, err := store.NextLifecycle(t.Context(), wrong, issuer, 0); !errors.Is(err, requestintent.ErrNotFound) {
			t.Fatal("cross-scope traversal", err)
		}
	}
	if _, err := store.NextLifecycle(t.Context(), scope, issuer+"/other", 0); !errors.Is(err, requestintent.ErrNotFound) {
		t.Fatal("cross-issuer traversal", err)
	}
	pending, err := store.NextLifecycle(t.Context(), scope, issuer, 0)
	if err != nil || pending.After != claim.Pending.After {
		t.Fatal("settled traversal", err)
	}
	if _, err := store.NextLifecycle(t.Context(), scope, issuer, pending.After); !errors.Is(err, requestintent.ErrNotFound) {
		t.Fatal("unsettled child was eligible", err)
	}
	reply := lifecycleMessage(t, scope, claim, "runtime-completed", 4, "COMPLETED")
	reply.Time = claim.Pending.Child.Deadline.Format(time.RFC3339Nano)
	raw, err := perfeng.EncodeIntegration(reply)
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer runtime-test" || r.URL.Query().Get("issuer") != issuer {
			t.Error("unauthenticated or changed lookup")
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write(raw); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	source := perfeng.IntegrationHTTP{Endpoint: server.URL + "/integration/v1/requests", Token: "runtime-test", Provider: scope.Issuer, Principal: perfeng.IntegrationIdentity{Issuer: scope.PrincipalIssuer, ID: scope.PrincipalID}}
	for range 2 {
		event, err := source.ConsumeLifecycle(t.Context(), store, scope, pending)
		if err != nil || !event.Late() || event.Data().State != requestintent.LifecycleCompleted {
			t.Fatal("runtime consumption", err)
		}
	}
	history, err := store.LifecycleHistory(t.Context(), scope, pending.After, admission, verify)
	if err != nil || len(history) != 1 || calls.Load() != 1 {
		t.Fatal("terminal polling/replay", len(history), calls.Load(), err)
	}
}

func lifecycleMessage(t *testing.T, scope requestintent.Scope, claim requestintent.Claim, id string, revision int64, state string) perfeng.IntegrationMessage {
	t.Helper()
	original := lifecycleReplyForStore(t, claim.Pending.Child.Payload)
	reply := workerMessages(t, "completed")[2]
	reply.ID, reply.Source, reply.Time = id, scope.Issuer, original.Time
	reply.Data.Context = original.Data.Context
	reply.Data.Run = &perfeng.IntegrationIdentity{Issuer: scope.Issuer, ID: "run-" + claim.Pending.Child.Request.ID}
	number := json.Number(strconv.FormatInt(revision, 10))
	reply.Data.Revision, reply.Data.State = &number, state

	return reply
}

func lifecycleReplyForStore(t *testing.T, raw []byte) perfeng.IntegrationMessage {
	t.Helper()
	message, err := perfeng.DecodeIntegration(raw)
	if err != nil {
		t.Fatal(err)
	}

	return message
}

func lifecycleStoreFixture(t *testing.T, kind string) (*PerformanceStore, requestintent.Scope, requestintent.Claim, requestintent.LifecycleAdmission, requestintent.LifecycleVerifier) {
	t.Helper()
	store, data, _ := workerFixture(t)
	claim, err := store.ClaimRequest(t.Context(), data.Scope, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	reply := lifecycleMessage(t, data.Scope, claim, "admission", 0, "RUNNING")
	if kind == "accepted" {
		reply.Type, reply.Data.State, reply.Data.Revision = "dev.perfeng.integration.accepted.v1", "", nil
	}
	outcome, err := perfeng.DispatchOutcome(claim.Pending.Child.Payload, reply, data.Scope.Issuer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SettleRequest(t.Context(), data.Scope, claim, outcome); err != nil {
		t.Fatal(err)
	}
	verify := func(retained requestintent.Outcome) (requestintent.LifecycleAdmission, error) {
		return perfeng.BindLifecycleAdmission(claim.Pending.Child.Payload, retained, data.Scope.Issuer)
	}
	admission, err := verify(outcome)
	if err != nil {
		t.Fatal(err)
	}

	return store, data.Scope, claim, admission, verify
}

func storeLifecycleEvent(t *testing.T, scope requestintent.Scope, claim requestintent.Claim, id string, revision int64, state string) requestintent.LifecycleEvent {
	t.Helper()
	reply := lifecycleMessage(t, scope, claim, id, revision, state)
	reply.Time = claim.Pending.Child.Deadline.Format(time.RFC3339Nano) // Valid late evidence.
	event, err := perfeng.LifecycleObservation(claim.Pending.Child.Payload, reply, scope.Issuer)
	if err != nil {
		t.Fatal(err)
	}

	return event
}

func TestPerformanceLifecycleAppendConcurrentReplayScopedProjectionAndReopen(t *testing.T) {
	store, scope, claim, admission, verify := lifecycleStoreFixture(t, "accepted")
	position := claim.Pending.After
	if history, err := store.LifecycleHistory(t.Context(), scope, position, admission, verify); err != nil || len(history) != 0 {
		t.Fatal("new history", err)
	}
	if _, err := store.LifecycleProjection(t.Context(), scope, position, admission, verify); !errors.Is(err, requestintent.ErrNotFound) {
		t.Fatal("empty projection fabricated progress", err)
	}
	running := storeLifecycleEvent(t, scope, claim, "running", 0, "RUNNING")
	results := make(chan error, 2)
	var writes atomic.Int32
	for range 2 {
		go func() {
			created, err := store.AppendLifecycle(t.Context(), scope, position, admission, running, verify)
			if created {
				writes.Add(1)
			}
			results <- err
		}()
	}
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if writes.Load() != 1 {
		t.Fatal("concurrent replay duplicated writes")
	}
	forged := admission.Data()
	forged.Run.ID += "-forged"
	invalid, err := requestintent.NewLifecycleAdmission(forged)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.LifecycleHistory(t.Context(), scope, position, invalid, verify); !errors.Is(err, requestintent.ErrInvalid) {
		t.Fatal("forged admission run accepted", err)
	}
	for _, event := range []requestintent.LifecycleEvent{storeLifecycleEvent(t, scope, claim, "completed", 4, "COMPLETED"), storeLifecycleEvent(t, scope, claim, "stale", 2, "RUNNING")} {
		if created, err := store.AppendLifecycle(t.Context(), scope, position, admission, event, verify); err != nil || !created {
			t.Fatal("valid lifecycle append", err)
		}
	}
	url := store.runtime.pool.Config().ConnString()
	store.runtime.Close()
	store = openTestStore(t, url).runtime.Performance()
	projection, err := store.LifecycleProjection(t.Context(), scope, position, admission, verify)
	if err != nil || projection.Data().State != requestintent.LifecycleCompleted || !projection.Late() {
		t.Fatal("reopen/regression", err)
	}
	assertLifecycleScopeAndCorruption(t, store, scope, position, admission, verify)
}

func assertLifecycleScopeAndCorruption(t *testing.T, store *PerformanceStore, scope requestintent.Scope, position int64, admission requestintent.LifecycleAdmission, verify requestintent.LifecycleVerifier) {
	t.Helper()
	for _, field := range []string{"issuer", "principal-issuer", "principal-id"} {
		wrong := scope
		switch field {
		case "issuer":
			wrong.Issuer += "-wrong"
		case "principal-issuer":
			wrong.PrincipalIssuer += "-wrong"
		default:
			wrong.PrincipalID += "-wrong"
		}
		if _, err := store.LifecycleHistory(t.Context(), wrong, position, admission, verify); !errors.Is(err, requestintent.ErrNotFound) {
			t.Fatal("scope leaked", err)
		}
	}
	if _, err := store.runtime.pool.Exec(t.Context(), `ALTER TABLE argus_catalog.performance_lifecycle_events DISABLE TRIGGER trg_performance_lifecycle_immutable;
        UPDATE argus_catalog.performance_lifecycle_events SET state='ACCEPTED' WHERE revision=4`); err != nil {
		t.Fatal(err)
	}
	if history, err := store.LifecycleHistory(t.Context(), scope, position, admission, verify); !errors.Is(err, requestintent.ErrUnavailable) || history != nil {
		t.Fatal("corrupt metadata became a projection", err)
	}
	if _, err := store.runtime.pool.Exec(t.Context(), `UPDATE argus_catalog.performance_lifecycle_events SET state='COMPLETED',payload=convert_to('{}','UTF8') WHERE revision=4`); err != nil {
		t.Fatal(err)
	}
	if history, err := store.LifecycleHistory(t.Context(), scope, position, admission, verify); !errors.Is(err, requestintent.ErrUnavailable) || history != nil {
		t.Fatal("corrupt payload became a projection", err)
	}
}

func TestPerformanceLifecycleAppendSeedsRecoveryAtomicallyAndReconcilesReplay(t *testing.T) {
	store, scope, claim, admission, verify := lifecycleStoreFixture(t, "lifecycle")
	event := storeLifecycleEvent(t, scope, claim, "completed", 1, "COMPLETED")
	if _, err := store.runtime.pool.Exec(t.Context(), `CREATE TRIGGER reject_test_lifecycle AFTER INSERT ON argus_catalog.performance_lifecycle_events
        FOR EACH ROW WHEN (NEW.revision=1) EXECUTE FUNCTION argus_catalog.reject_performance_catalog_mutation()`); err != nil {
		t.Fatal(err)
	}
	if created, err := store.AppendLifecycle(t.Context(), scope, claim.Pending.After, admission, event, verify); created || !errors.Is(err, requestintent.ErrUnavailable) {
		t.Fatal("failed append exposed success", err)
	}
	if history, err := store.LifecycleHistory(t.Context(), scope, claim.Pending.After, admission, verify); err != nil || len(history) != 0 {
		t.Fatal("rollback leaked anchor", err)
	}
	if _, err := store.runtime.pool.Exec(t.Context(), `DROP TRIGGER reject_test_lifecycle ON argus_catalog.performance_lifecycle_events`); err != nil {
		t.Fatal(err)
	}
	if created, err := store.AppendLifecycle(t.Context(), scope, claim.Pending.After, admission, event, verify); err != nil || !created {
		t.Fatal("atomic recovery append", err)
	}
	if created, err := store.AppendLifecycle(t.Context(), scope, claim.Pending.After, admission, event, verify); err != nil || created {
		t.Fatal("lost-acknowledgement retry", err)
	}
	history, err := store.LifecycleHistory(t.Context(), scope, claim.Pending.After, admission, verify)
	if err != nil || len(history) != 2 || !history[0].Equal(admission.Data().Initial) {
		t.Fatal("anchor not retained", err)
	}
	copied := history[0].Data().Evidence.Data()
	copied.Payload[0] = '!'
	if history[0].Data().Evidence.Data().Payload[0] != '{' {
		t.Fatal("aliased history")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := store.AppendLifecycle(ctx, scope, claim.Pending.After, admission, event, verify); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost", err)
	}
}
