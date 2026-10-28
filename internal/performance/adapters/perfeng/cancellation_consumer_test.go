package perfeng

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

type cancellationConsumerMemory struct {
	*lifecycleMemory
	intent                requestintent.Cancellation
	intentErr, outcomeErr error
}

func (s *cancellationConsumerMemory) GetCancellation(context.Context, requestintent.Scope, int64, requestintent.CancellationVerifier) (requestintent.Cancellation, error) {
	return s.intent, s.intentErr
}
func (s *cancellationConsumerMemory) RequestOutcome(context.Context, requestintent.Scope, int64) (requestintent.Outcome, error) {
	return s.retained, s.outcomeErr
}

type cancellationScenario struct {
	source        IntegrationHTTP
	scope         requestintent.Scope
	pending       requestintent.Pending
	store         *cancellationConsumerMemory
	lookup, reply IntegrationMessage
	calls         int
}

func cancellationConsumptionFixture(t *testing.T) *cancellationScenario {
	t.Helper()
	source, scope, child := recoveryHTTPFixture(t)
	messages := integrationExchange(t, "cancelled")
	original := lifecycleReply(t, child.Payload)
	intentMessage := lifecycleReply(t, messages[2])
	intentMessage.Data.Context = original.Data.Context
	intent, err := BindCancellation(child.Payload, cancellationBytes(t, intentMessage))
	if err != nil {
		t.Fatal(err)
	}
	accepted := lifecycleReply(t, integrationExchange(t, "completed")[1])
	retained, err := DispatchOutcome(child.Payload, accepted, source.Provider)
	if err != nil {
		t.Fatal(err)
	}
	reply := lifecycleReply(t, messages[3])
	reply.Data.Context, reply.Data.Run = original.Data.Context, accepted.Data.Run
	reply.ID, reply.Time = "cancellation-reply", original.Data.Context.Deadline
	revision := json.Number("2")
	reply.Data.Revision = &revision
	lookup := lifecycleReply(t, cancellationBytes(t, reply))
	lookup.ID, lookup.Data.State = "lookup-running", "RUNNING"
	one := json.Number("1")
	lookup.Data.Revision = &one

	return &cancellationScenario{source: source, scope: scope, pending: requestintent.Pending{After: 7, Child: child}, store: &cancellationConsumerMemory{lifecycleMemory: &lifecycleMemory{retained: retained, commit: true}, intent: intent}, lookup: lookup, reply: reply}
}

func configureCancellationConsumption(t *testing.T, s *cancellationScenario, name string) {
	t.Helper()
	switch name {
	case "already-cancelling":
		s.lookup = s.reply
	case "completed-race":
		s.reply.Data.State = "COMPLETED"
	case "terminal-history", "terminal-anchor":
		s.reply.Data.State = "CANCELLED"
		event, err := LifecycleObservation(s.pending.Child.Payload, s.reply, s.source.Provider)
		if err != nil {
			t.Fatal(err)
		}
		if name == "terminal-history" {
			s.store.history = []requestintent.LifecycleEvent{event}
		} else {
			s.store.retained = event.Data().Evidence
		}
	case "stale-running":
		event, err := LifecycleObservation(s.pending.Child.Payload, s.reply, s.source.Provider)
		if err != nil {
			t.Fatal(err)
		}
		s.store.history = []requestintent.LifecycleEvent{event}
	case "lost-exact", "lost-absent":
		s.store.appendErr = requestintent.ErrUnavailable
		s.store.commit = name == "lost-exact"
	case "wrong-lookup-run":
		s.lookup.Data.Run.ID = "another-run"
	case "wrong-cancel-run":
		s.reply.Data.Run.ID = "another-run"
	case "contradictory":
		s.reply.Data.Revision = s.lookup.Data.Revision
	case "missing-intent":
		s.store.intentErr = requestintent.ErrNotFound
	case "unadmitted":
		s.store.outcomeErr = requestintent.ErrNotFound
	case "rejected":
		rejected := lifecycleReply(t, integrationExchange(t, "rejected")[1])
		rejected.Data.Context = s.lookup.Data.Context
		var err error
		s.store.retained, err = DispatchOutcome(s.pending.Child.Payload, rejected, s.source.Provider)
		if err != nil {
			t.Fatal(err)
		}
	case "history-error":
		s.store.readErr = requestintent.ErrUnavailable
	case "scope":
		s.source.Principal.ID = "untrusted"
	}
}

func cancellationConsumerClient(t *testing.T, s *cancellationScenario, name string) *http.Client {
	t.Helper()

	return &http.Client{Transport: integrationRoundTrip(func(r *http.Request) (*http.Response, error) {
		s.calls++
		if r.Header.Get("Authorization") != "Bearer secret" || r.URL.Query().Get("issuer") != s.pending.Child.Request.Issuer {
			t.Fatal("unscoped/unauthenticated exchange")
		}
		status, reply := http.StatusOK, s.lookup
		if r.Method == http.MethodPost {
			body, err := io.ReadAll(r.Body)
			if err != nil || !strings.HasSuffix(r.URL.Path, "/cancellation") || !bytes.Equal(body, s.store.intent.Data().Payload) {
				t.Fatal("original submission or changed cancellation")
			}
			status, reply = http.StatusAccepted, s.reply
			if reply.Data.State == "COMPLETED" {
				status = http.StatusOK
			}
		} else if r.Method != http.MethodGet {
			t.Fatal("unexpected method")
		}
		if name == "rate-limited" {
			return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": {"23"}}, Body: io.NopCloser(strings.NewReader("private remote diagnostic"))}, nil
		}

		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(bytes.NewReader(cancellationBytes(t, reply)))}, nil
	})}
}

func TestCancellationConsumptionEnforcesBoundedRecoveryAndHistory(t *testing.T) {
	for _, test := range []struct {
		name           string
		calls, appends int
		success        bool
	}{
		{"cancel", 2, 1, true}, {"already-cancelling", 1, 1, true}, {"completed-race", 2, 1, true},
		{"terminal-history", 0, 0, true}, {"terminal-anchor", 0, 1, true}, {"stale-running", 1, 1, true},
		{"lost-exact", 2, 1, true}, {"lost-absent", 2, 1, false}, {"wrong-lookup-run", 1, 0, false},
		{"wrong-cancel-run", 2, 0, false}, {"contradictory", 2, 0, false}, {"missing-intent", 0, 0, false},
		{"unadmitted", 0, 0, false}, {"rejected", 0, 0, false}, {"history-error", 0, 0, false},
		{"scope", 0, 0, false}, {"cancelled", 0, 0, false}, {"rate-limited", 1, 0, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := cancellationConsumptionFixture(t)
			configureCancellationConsumption(t, s, test.name)
			s.source.Client = cancellationConsumerClient(t, s, test.name)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if test.name == "cancelled" {
				cancel()
			}
			event, err := s.source.ConsumeCancellation(ctx, s.store, s.scope, s.pending)
			assertCancellationConsumptionResult(t, test.success, event, err)
			if s.calls != test.calls || s.store.appends != test.appends {
				t.Fatal("unexpected effects", s.calls, s.store.appends)
			}
			if test.name == "rate-limited" {
				var status *IntegrationHTTPStatusError
				if !errors.As(err, &status) || status.RetryAfter != 23*time.Second {
					t.Fatal("lookup lost backoff", err)
				}
			}
		})
	}
}

func assertCancellationConsumptionResult(t *testing.T, success bool, event requestintent.LifecycleEvent, err error) {
	t.Helper()
	if success && (err != nil || event.Data().Evidence.Fingerprint() == "" || !event.Late()) {
		t.Fatal("valid late cancellation unresolved", err)
	}
	if !success && (err == nil || event.Data().Evidence.Fingerprint() != "") {
		t.Fatal("invalid cancellation accepted", err)
	}
}

func TestCancellationConsumptionRecoversLostRemoteAcknowledgementByGET(t *testing.T) {
	s := cancellationConsumptionFixture(t)
	posts, gets := 0, 0
	s.source.Client = &http.Client{Transport: integrationRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodPost {
			posts++
			return nil, errors.New("private lost response")
		}
		gets++
		reply := s.lookup
		if posts > 0 {
			reply = s.reply
		}

		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(bytes.NewReader(cancellationBytes(t, reply)))}, nil
	})}
	if event, err := s.source.ConsumeCancellation(t.Context(), s.store, s.scope, s.pending); err == nil || event.Data().Evidence.Fingerprint() != "" || s.store.appends != 0 {
		t.Fatal("lost remote acknowledgement accepted", err)
	}
	if _, err := s.source.ConsumeCancellation(t.Context(), s.store, s.scope, s.pending); err != nil {
		t.Fatal(err)
	}
	if posts != 1 || gets != 2 || s.store.appends != 1 {
		t.Fatal("uncertain cancellation POST was repeated", posts, gets)
	}
}
