//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/performance/adapters/perfeng"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

func workerMessages(t *testing.T, name string) []perfeng.IntegrationMessage {
	t.Helper()
	raw, err := os.ReadFile("../performance/adapters/perfeng/snapshot/integration/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var messages []perfeng.IntegrationMessage
	if err := json.Unmarshal(raw, &messages); err != nil {
		t.Fatal(err)
	}

	return messages
}

func workerFixture(t *testing.T) (*PerformanceStore, requestintent.Data, []requestintent.Pending) {
	t.Helper()
	store, catalog := requestSchemaStore(t, "worker")
	data := requestFixture(t, store, catalog, "worker")
	third := data.Children[1]
	third.Request.ID, third.Attempt.ID, third.EventID = "worker-2", "attempt-worker-2", "event-worker-2"
	data.Children = append(data.Children, third)
	completed := workerMessages(t, "completed")
	for i := range data.Children {
		child := &data.Children[i]
		request := completed[0]
		request.ID, request.Source, request.Time = child.EventID, child.Request.Issuer, child.RequestedAt.Format(time.RFC3339Nano)
		request.Data.Context.Request = perfeng.IntegrationIdentity(child.Request)
		request.Data.Context.Attempt = perfeng.IntegrationIdentity(child.Attempt)
		request.Data.Context.RequestedAt, request.Data.Context.Deadline = request.Time, child.Deadline.Format(time.RFC3339Nano)
		raw, err := perfeng.EncodeIntegration(request)
		if err != nil {
			t.Fatal(err)
		}
		child.Payload, child.SHA256 = raw, requestDigest(string(raw))
	}
	capability := store.runtime.Performance()
	if _, err := capability.CreateRequest(t.Context(), inwardRequest(t, data)); err != nil {
		t.Fatal(err)
	}
	pending, err := capability.PendingRequests(t.Context(), data.Scope, 0, 100)
	if err != nil || len(pending) != 3 {
		t.Fatal("pending work", err)
	}
	claim, err := capability.ClaimRequest(t.Context(), data.Scope, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := capability.ReleaseRequest(t.Context(), data.Scope, claim, 0); err != nil {
		t.Fatal(err)
	}

	return capability, data, pending
}

type workerPeer struct {
	t                   *testing.T
	data                requestintent.Data
	completed, rejected []perfeng.IntegrationMessage
}

func (peer workerPeer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	t, data, completed, rejected := peer.t, peer.data, peer.completed, peer.rejected
	if r.Header.Get("Authorization") != "Bearer worker-secret" {
		t.Error("unauthenticated worker")
		w.WriteHeader(401)
		return
	}
	var raw []byte
	var err error
	reply, status := completed[2], http.StatusOK // GET returns a lifecycle snapshot.
	if r.Method == http.MethodGet && r.URL.Path == "/integration/v1/requests/"+data.Children[0].Request.ID && r.URL.Query().Get("issuer") == data.Children[0].Request.Issuer {
		raw = data.Children[0].Payload
	} else if r.Method == http.MethodPost && r.URL.Path == "/integration/v1/requests" {
		raw, err = io.ReadAll(r.Body)
		if string(raw) == string(data.Children[1].Payload) {
			reply, status = completed[1], http.StatusAccepted
		} else if string(raw) == string(data.Children[2].Payload) {
			reply, status = rejected[1], http.StatusUnprocessableEntity
		} else {
			t.Error("replayed or changed POST")
			w.WriteHeader(400)
			return
		}
	} else {
		t.Error("wrong method or scoped identity")
		w.WriteHeader(400)
		return
	}
	original, decodeErr := perfeng.DecodeIntegration(raw)
	if err != nil || decodeErr != nil {
		t.Error("invalid request", err, decodeErr)
		w.WriteHeader(400)
		return
	}
	// Reuse the valid database timestamp: the pinned wire allows only microseconds.
	reply.ID, reply.Source, reply.Time = "reply-"+original.ID, data.Scope.Issuer, original.Time
	reply.Data.Context = original.Data.Context
	if reply.Data.Run != nil {
		reply.Data.Run = &perfeng.IntegrationIdentity{Issuer: data.Scope.Issuer, ID: "run-" + original.ID}
	}
	body, err := perfeng.EncodeIntegration(reply)
	if err != nil {
		t.Error(err)
		w.WriteHeader(500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write(body); err != nil {
		t.Error(err)
	}
}

func TestPerformanceWorkerAuthenticatesRecoveryAndDispatch(t *testing.T) {
	capability, data, pending := workerFixture(t)
	provider := httptest.NewServer(workerPeer{t: t, data: data, completed: workerMessages(t, "completed"), rejected: workerMessages(t, "rejected")})
	defer provider.Close()
	source := perfeng.IntegrationHTTP{Endpoint: provider.URL + "/integration/v1/requests", Token: "worker-secret", Provider: data.Scope.Issuer, Principal: perfeng.IntegrationIdentity{Issuer: data.Scope.PrincipalIssuer, ID: data.Scope.PrincipalID}}
	d := requestintent.Dispatcher{Store: capability, Provider: source, Scope: data.Scope, RequestIssuer: data.Children[0].Request.Issuer, Lease: time.Minute, Now: time.Now}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	ticks, outcomes, done := make(chan time.Time), make(chan requestintent.Outcome, 1), make(chan error, 1)
	go func() {
		done <- d.Run(ctx, ticks, func(outcome requestintent.Outcome, err error) {
			if err != nil {
				t.Error(err)
			}
			outcomes <- outcome
		})
		close(done)
	}()
	defer func() { cancel(); <-done }()
	for i, kind := range []string{"lifecycle", "accepted", "rejected"} {
		if i > 0 {
			select {
			case ticks <- time.Now():
			case <-ctx.Done():
				t.Fatal("worker did not wait")
			}
		}
		select {
		case outcome := <-outcomes:
			retained, err := capability.RequestOutcome(ctx, data.Scope, pending[i].After)
			if err != nil || retained.Fingerprint() != outcome.Fingerprint() || retained.Data().Kind != kind {
				t.Fatal("outcome not durable", err)
			}
		case <-ctx.Done():
			t.Fatal("worker did not complete")
		}
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := capability.ClaimRequest(t.Context(), data.Scope, time.Minute); !errors.Is(err, requestintent.ErrNotFound) {
		t.Fatal("settled work remained claimable", err)
	}
}
