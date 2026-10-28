//go:build integration

package postgres

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/performance/adapters/perfeng"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

func TestCancellationConsumerPersistsProgressWithoutChangingIntentOrAdmission(t *testing.T) {
	store, scope, claim, admission, verify := lifecycleStoreFixture(t, "accepted")
	original := lifecycleReplyForStore(t, claim.Pending.Child.Payload)
	message := workerMessages(t, "cancelled")[2]
	message.ID, message.Source, message.Time = "operator-stop", original.Source, claim.Pending.Child.Deadline.Format(time.RFC3339Nano)
	message.Data.Context = original.Data.Context
	raw, err := perfeng.EncodeIntegration(message)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := perfeng.BindCancellation(claim.Pending.Child.Payload, raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.RequestCancellation(t.Context(), scope, claim.Pending.After, intent, perfeng.BindCancellation); err != nil {
		t.Fatal(err)
	}
	wire := map[string][]byte{}
	for i, state := range []string{"RUNNING", "CANCELLING", "CANCELLED"} {
		reply := lifecycleMessage(t, scope, claim, "provider-"+state, int64(i+1), state)
		reply.Time = message.Time
		wire[state], err = perfeng.EncodeIntegration(reply)
		if err != nil {
			t.Fatal(err)
		}
	}
	var gets, posts atomic.Int32
	server := httptest.NewServer(cancellationStoreHandler(t, claim, wire, intent, &gets, &posts))
	defer server.Close()
	source := perfeng.IntegrationHTTP{Endpoint: server.URL + "/integration/v1/requests", Token: "cancel-test", Provider: scope.Issuer, Principal: perfeng.IntegrationIdentity{Issuer: scope.PrincipalIssuer, ID: scope.PrincipalID}}
	for _, state := range []requestintent.LifecycleState{requestintent.LifecycleCancelling, requestintent.LifecycleCancelled, requestintent.LifecycleCancelled} {
		event, err := source.ConsumeCancellation(t.Context(), store, scope, claim.Pending)
		if err != nil || event.Data().State != state || !event.Late() {
			t.Fatal("cancellation progression", state, err)
		}
	}
	history, err := store.LifecycleHistory(t.Context(), scope, claim.Pending.After, admission, verify)
	if err != nil || len(history) != 2 || gets.Load() != 2 || posts.Load() != 1 {
		t.Fatal("unexpected network/history effects", len(history), gets.Load(), posts.Load(), err)
	}
	retained, err := store.RequestOutcome(t.Context(), scope, claim.Pending.After)
	if err != nil || retained.Fingerprint() != admission.Data().Evidence.Fingerprint() {
		t.Fatal("cancellation replaced admission", err)
	}
	retainedIntent, err := store.GetCancellation(t.Context(), scope, claim.Pending.After, perfeng.BindCancellation)
	if err != nil || retainedIntent.Fingerprint() != intent.Fingerprint() {
		t.Fatal("cancellation rewrote intent", err)
	}
}

func cancellationStoreHandler(t *testing.T, claim requestintent.Claim, wire map[string][]byte, intent requestintent.Cancellation, gets, posts *atomic.Int32) http.Handler {
	t.Helper()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer cancel-test" || r.URL.Query().Get("issuer") != claim.Pending.Child.Request.Issuer {
			t.Error("missing authentication/scope")
		}
		state, status := "CANCELLED", http.StatusOK
		if r.Method == http.MethodGet {
			if gets.Add(1) == 1 {
				state = "RUNNING"
			}
		} else {
			posts.Add(1)
			body, err := io.ReadAll(r.Body)
			if err != nil || r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/cancellation") || !bytes.Equal(body, intent.Data().Payload) {
				t.Error("changed cancellation or original submission")
			}
			state, status = "CANCELLING", http.StatusAccepted
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if _, err := w.Write(wire[state]); err != nil {
			t.Error(err)
		}
	})
}
