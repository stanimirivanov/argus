//go:build integration

package postgres

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

func atomicRerunFixture(t *testing.T) (rerunSchemaFixture, requestintent.Intent, []requestintent.RerunCreation) {
	t.Helper()
	f := releasedRerunSchema(t)
	successor := inwardRequest(t, requestFixture(t, f.store, f.data.CatalogSHA256, "atomic"))
	bindings := make([]requestintent.RerunCreation, 2)
	for i, state := range []requestintent.LifecycleState{requestintent.LifecycleCompleted, requestintent.LifecycleCancelled} {
		claim := f.parents[i]
		event := schemaLifecycle(t, f.data.Scope, claim, "terminal-"+claim.Pending.Child.EventID, 2, state, "run-"+claim.Pending.Child.Request.ID)
		outcome := outcomeFixture(t, f.data.Scope, claim, "accepted")
		admission, err := requestintent.NewLifecycleAdmission(requestintent.LifecycleAdmissionData{Evidence: outcome, Run: event.Data().Run, RequestSHA256: claim.Pending.Child.SHA256})
		if err != nil {
			t.Fatal(err)
		}
		parent := requestintent.RerunParent{Pending: claim.Pending, Intent: inwardRequest(t, f.data), Admission: admission, History: []requestintent.LifecycleEvent{event}}
		proof, err := requestintent.NewRerun(parent, successor, i)
		if err != nil {
			t.Fatal(err)
		}
		bindings[i] = requestintent.RerunCreation{Parent: parent, Evidence: proof, Verify: func(e requestintent.Outcome) (requestintent.LifecycleAdmission, error) {
			if e.Fingerprint() == outcome.Fingerprint() {
				return admission, nil
			}
			if e.Fingerprint() == event.Data().Evidence.Fingerprint() {
				return requestintent.NewLifecycleAdmission(requestintent.LifecycleAdmissionData{Evidence: e, Run: event.Data().Run, RequestSHA256: event.Data().RequestSHA256, Initial: event})
			}

			return requestintent.LifecycleAdmission{}, requestintent.ErrInvalid
		}}
	}

	return f, successor, bindings
}

func TestAtomicRerunCreatesCompleteBatchAndReplaysOnSingleConnection(t *testing.T) {
	f, successor, bindings := atomicRerunFixture(t)
	config := f.store.pool.Config()
	config.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	c := (&Runtime{pool: pool}).Performance()
	start := make(chan struct{})
	results := make(chan error, 2)
	created := make(chan bool, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Go(func() {
			<-start
			fresh, err := c.CreateReruns(t.Context(), successor, bindings)
			created <- fresh
			results <- err
		})
	}
	close(start)
	wait.Wait()
	close(results)
	close(created)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	count := 0
	for fresh := range created {
		if fresh {
			count++
		}
	}
	if count != 1 {
		t.Fatal("concurrent replay created twice", count)
	}
	for _, b := range bindings {
		got, err := c.RerunSuccessor(t.Context(), f.data.Scope, b.Evidence.Data().ParentPosition)
		if err != nil || got.Evidence.Fingerprint() != b.Evidence.Fingerprint() {
			t.Fatal("incomplete atomic batch", err)
		}
	}
	// Later valid evidence is outside the original proof: replay must not rebind it.
	claim := f.parents[0]
	event := schemaLifecycle(t, f.data.Scope, claim, "zzz-new-observation", 2, requestintent.LifecycleCompleted, "run-"+claim.Pending.Child.Request.ID)
	if err := insertLifecycleSchema(t.Context(), c, f.data.Scope, claim.Pending.After, outcomeFixture(t, f.data.Scope, claim, "accepted"), event); err != nil {
		t.Fatal(err)
	}
	replacement := performanceFixture(t, f.store, "pages", "rerun-replacement", -time.Minute)
	if _, err := c.Publish(t.Context(), replacement); err != nil {
		t.Fatal(err)
	}
	if fresh, err := c.CreateReruns(t.Context(), successor, bindings); err != nil || fresh {
		t.Fatal("historical replay became new work", err)
	}
	reopened := openTestStore(t, f.store.pool.Config().ConnString()).runtime.Performance()
	if fresh, err := reopened.CreateReruns(t.Context(), successor, bindings); err != nil || fresh {
		t.Fatal("reopen lost replay", err)
	}
}

func TestAtomicRerunRejectsInvalidBindingsBeforeStorage(t *testing.T) {
	f, successor, bindings := atomicRerunFixture(t)
	store := &PerformanceStore{}
	for _, candidate := range [][]requestintent.RerunCreation{nil, bindings[:1], {bindings[0], bindings[0]}, {{}, bindings[1]}} {
		if _, err := store.CreateReruns(t.Context(), successor, candidate); !errors.Is(err, requestintent.ErrInvalid) {
			t.Fatal("invalid batch reached storage", err)
		}
	}
	broken := append([]requestintent.RerunCreation(nil), bindings...)
	broken[0].Verify = nil
	if _, err := store.CreateReruns(t.Context(), successor, broken); !errors.Is(err, requestintent.ErrInvalid) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := store.CreateReruns(ctx, successor, bindings); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled batch reached storage", err)
	}
	c := f.store.runtime.Performance()
	if _, err := c.CreateRequest(t.Context(), successor); err != nil {
		t.Fatal(err)
	}
	if fresh, err := c.CreateReruns(t.Context(), successor, bindings); !errors.Is(err, requestintent.ErrConflict) || fresh {
		t.Fatal("unlinked legacy intent treated as rerun replay", err)
	}
}

func TestAtomicRerunForkConflictRollsBackSuccessorWork(t *testing.T) {
	f, successor, bindings := atomicRerunFixture(t)
	c := f.store.runtime.Performance()
	data := requestFixture(t, f.store, f.data.CatalogSHA256, "competing")
	fork := inwardRequest(t, data)
	forkBindings := append([]requestintent.RerunCreation(nil), bindings...)
	for i := range forkBindings {
		proof, err := requestintent.NewRerun(forkBindings[i].Parent, fork, i)
		if err != nil {
			t.Fatal(err)
		}
		forkBindings[i].Evidence = proof
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, candidate := range []struct {
		intent   requestintent.Intent
		bindings []requestintent.RerunCreation
	}{{successor, bindings}, {fork, forkBindings}} {
		go func() {
			<-start
			_, err := c.CreateReruns(t.Context(), candidate.intent, candidate.bindings)
			results <- err
		}()
	}
	close(start)
	one, two := <-results, <-results
	if one == nil {
		one, two = two, one
	}
	if !errors.Is(one, requestintent.ErrConflict) || two != nil {
		t.Fatal("concurrent fork results", one, two)
	}
	retained, err := c.RerunSuccessor(t.Context(), data.Scope, bindings[0].Evidence.Data().ParentPosition)
	if err != nil {
		t.Fatal(err)
	}
	loser := successor
	if retained.Evidence.Data().SuccessorIntentSHA256 == successor.Fingerprint() {
		loser = fork
	}
	if _, err := c.GetRequest(t.Context(), data.Scope, loser.Data().ReviewSHA256); !errors.Is(err, requestintent.ErrNotFound) {
		t.Fatal("fork leaked intent", err)
	}
}

func TestAtomicRerunSeedsRecoveryAnchorAndRollsItBackOnExpiry(t *testing.T) {
	c, scope, claim, outcome := lifecycleSchemaFixture(t, "lifecycle")
	event := schemaLifecycle(t, scope, claim, outcome.Data().EventID, 0, requestintent.LifecycleCancelled, "anchor-run")
	admission, err := requestintent.NewLifecycleAdmission(requestintent.LifecycleAdmissionData{Evidence: outcome, Run: event.Data().Run, RequestSHA256: event.Data().RequestSHA256, Initial: event})
	if err != nil {
		t.Fatal(err)
	}
	var review string
	if err := c.runtime.pool.QueryRow(t.Context(), `SELECT i.review_sha256 FROM argus_catalog.performance_pending_dispatches p JOIN argus_catalog.performance_request_intents i ON i.intent_uuid=p.intent_uuid WHERE p.position=$1`, claim.Pending.After).Scan(&review); err != nil {
		t.Fatal(err)
	}
	previous, err := c.GetRequest(t.Context(), scope, review)
	if err != nil {
		t.Fatal(err)
	}
	parent := requestintent.RerunParent{Pending: claim.Pending, Intent: previous, Admission: admission}
	data := requestFixture(t, &integratedStore{pool: c.runtime.pool}, previous.Data().CatalogSHA256, "anchor-successor")
	data.Children = data.Children[:1]
	verify := func(e requestintent.Outcome) (requestintent.LifecycleAdmission, error) {
		if e.Fingerprint() != outcome.Fingerprint() {
			return requestintent.LifecycleAdmission{}, requestintent.ErrInvalid
		}
		return admission, nil
	}
	expired := data
	expired.Children = append([]requestintent.Child(nil), data.Children...)
	expired.Children[0].RequestedAt = claim.Pending.Child.RequestedAt.Add(-time.Hour)
	expired.Children[0].Deadline = claim.Pending.Child.RequestedAt.Add(-time.Minute)
	for _, candidate := range []requestintent.Data{expired, data} {
		successor := inwardRequest(t, candidate)
		proof, err := requestintent.NewRerun(parent, successor, 0)
		if err != nil {
			t.Fatal(err)
		}
		fresh, err := c.CreateReruns(t.Context(), successor, []requestintent.RerunCreation{{Parent: parent, Evidence: proof, Verify: verify}})
		wantSuccess := candidate.Children[0].Deadline.Equal(data.Children[0].Deadline)
		var expected = requestintent.ErrExpired
		if wantSuccess {
			expected = nil
		}
		if !errors.Is(err, expected) || fresh != wantSuccess {
			t.Fatal("wrong anchor creation result", err)
		}
		var count int
		if err := c.runtime.pool.QueryRow(t.Context(), `SELECT count(*) FROM argus_catalog.performance_lifecycle_events WHERE position=$1`, claim.Pending.After).Scan(&count); err != nil || count != boolInt(wantSuccess) {
			t.Fatal("anchor not atomic", count, err)
		}
	}
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func TestAtomicRerunRechecksCurrentHistoryBeforeNewWork(t *testing.T) {
	f, successor, bindings := atomicRerunFixture(t)
	c := f.store.runtime.Performance()
	claim := f.parents[0]
	later := schemaLifecycle(t, f.data.Scope, claim, "zzz-before-creation", 2, requestintent.LifecycleCompleted, "run-"+claim.Pending.Child.Request.ID)
	if err := insertLifecycleSchema(t.Context(), c, f.data.Scope, claim.Pending.After, outcomeFixture(t, f.data.Scope, claim, "accepted"), later); err != nil {
		t.Fatal(err)
	}
	original := bindings[0].Verify
	bindings[0].Verify = func(e requestintent.Outcome) (requestintent.LifecycleAdmission, error) {
		if e.Fingerprint() == later.Data().Evidence.Fingerprint() {
			return requestintent.NewLifecycleAdmission(requestintent.LifecycleAdmissionData{Evidence: e, Run: later.Data().Run, RequestSHA256: later.Data().RequestSHA256, Initial: later})
		}
		return original(e)
	}
	if fresh, err := c.CreateReruns(t.Context(), successor, bindings); !errors.Is(err, requestintent.ErrConflict) || fresh {
		t.Fatal("stale proof authorized new work", err)
	}
	if _, err := c.GetRequest(t.Context(), f.data.Scope, successor.Data().ReviewSHA256); !errors.Is(err, requestintent.ErrNotFound) {
		t.Fatal("stale proof leaked work", err)
	}
}
