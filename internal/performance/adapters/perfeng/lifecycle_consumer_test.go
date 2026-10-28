package perfeng

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

type lifecycleMemory struct {
	retained           requestintent.Outcome
	history            []requestintent.LifecycleEvent
	reads, appends     int
	readErr, appendErr error
	commit             bool
	replacement        []requestintent.LifecycleEvent
}

func (s *lifecycleMemory) RequestOutcome(context.Context, requestintent.Scope, int64) (requestintent.Outcome, error) {
	return s.retained, nil
}

func (s *lifecycleMemory) LifecycleHistory(_ context.Context, _ requestintent.Scope, _ int64, _ requestintent.LifecycleAdmission, _ requestintent.LifecycleVerifier) ([]requestintent.LifecycleEvent, error) {
	s.reads++
	return s.history, s.readErr
}

func (s *lifecycleMemory) AppendLifecycle(_ context.Context, _ requestintent.Scope, _ int64, _ requestintent.LifecycleAdmission, event requestintent.LifecycleEvent, verify requestintent.LifecycleVerifier) (bool, error) {
	s.appends++
	bound, err := verify(event.Data().Evidence)
	if err != nil || !bound.Data().Initial.Equal(event) {
		return false, requestintent.ErrInvalid
	}
	if s.commit {
		s.history = append(s.history, event)
	}
	if s.replacement != nil {
		s.history = s.replacement
	}

	return s.commit, s.appendErr
}

func TestLifecycleConsumptionAuthenticatesAndReconcilesExactPersistence(t *testing.T) {
	for _, name := range []string{"append", "lost-exact", "lost-absent", "lost-newer", "provider-failure", "wrong-run", "history-failure", "invalid-child", "cancelled"} {
		t.Run(name, func(t *testing.T) {
			source, scope, child := recoveryHTTPFixture(t)
			messages := integrationExchange(t, "completed")
			accepted := lifecycleReply(t, messages[1])
			retained, err := DispatchOutcome(child.Payload, accepted, source.Provider)
			if err != nil {
				t.Fatal(err)
			}
			store := &lifecycleMemory{retained: retained, commit: true}
			reply := lifecycleReply(t, messages[len(messages)-1])
			reply.Time = reply.Data.Context.Deadline
			calls := 0
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			configureLifecycleScenario(t, name, store, &reply, &child, source.Provider, cancel)
			encoded, err := EncodeIntegration(reply)
			if err != nil {
				t.Fatal(err)
			}
			source.Client = &http.Client{Transport: integrationRoundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer secret" || r.URL.Query().Get("issuer") != child.Request.Issuer {
					t.Fatal("missing authentication or read-only original identity")
				}
				if name == "provider-failure" {
					return nil, errors.New("secret remote failure")
				}

				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(string(encoded)))}, nil
			})}
			got, err := source.ConsumeLifecycle(ctx, store, scope, requestintent.Pending{After: 7, Child: child})
			assertLifecycleConsumption(t, name, store, got, err, calls)
		})
	}
}

func configureLifecycleScenario(t *testing.T, name string, store *lifecycleMemory, reply *IntegrationMessage, child *requestintent.Child, provider string, cancel context.CancelFunc) {
	t.Helper()
	messages := integrationExchange(t, "completed")
	switch name {
	case "lost-exact", "lost-absent":
		store.appendErr = requestintent.ErrUnavailable
		store.commit = name == "lost-exact"
	case "lost-newer":
		newer, err := LifecycleObservation(child.Payload, *reply, provider)
		if err != nil {
			t.Fatal(err)
		}
		store.appendErr, store.commit, store.replacement = requestintent.ErrUnavailable, false, []requestintent.LifecycleEvent{newer}
		*reply = lifecycleReply(t, messages[3])
	case "wrong-run":
		reply.Data.Run.ID = "another-run"
	case "history-failure":
		store.readErr = requestintent.ErrUnavailable
	case "invalid-child":
		child.Request.ID = "other"
	case "cancelled":
		cancel()
	}
}

func assertLifecycleConsumption(t *testing.T, name string, store *lifecycleMemory, got requestintent.LifecycleEvent, err error, calls int) {
	t.Helper()
	success := name == "append" || name == "lost-exact"
	if success && (err != nil || !got.Late() || got.Data().State != requestintent.LifecycleCompleted) {
		t.Fatal("valid late evidence unresolved", err)
	}
	if !success && (err == nil || got.Data().Evidence.Fingerprint() != "") {
		t.Fatal("failure accepted", err)
	}
	if name == "invalid-child" || name == "cancelled" || name == "history-failure" {
		if calls != 0 || store.appends != 0 {
			t.Fatal("invalid/cancelled evidence performed I/O")
		}
	} else if calls != 1 {
		t.Fatal("provider request retried", calls)
	}
	if strings.Contains(strings.TrimSpace(errorText(err)), "secret") {
		t.Fatal("remote error leaked")
	}
	if strings.HasPrefix(name, "lost-") && (store.reads != 2 || store.appends != 1) {
		t.Fatal("acknowledgement was not reconciled exactly once")
	}
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestLifecycleConsumptionSeedsTerminalRecoveryAndSkipsTerminalLookup(t *testing.T) {
	source, scope, child := recoveryHTTPFixture(t)
	messages := integrationExchange(t, "completed")
	reply := lifecycleReply(t, messages[len(messages)-1])
	outcome, err := DispatchOutcome(child.Payload, reply, source.Provider)
	if err != nil {
		t.Fatal(err)
	}
	store := &lifecycleMemory{retained: outcome, commit: true}
	source.Client = &http.Client{Transport: integrationRoundTrip(func(*http.Request) (*http.Response, error) {
		t.Fatal("terminal evidence contacted provider")
		return nil, ErrIntegration
	})}
	for range 2 {
		got, err := source.ConsumeLifecycle(t.Context(), store, scope, requestintent.Pending{After: 7, Child: child})
		if err != nil || got.Data().State != requestintent.LifecycleCompleted {
			t.Fatal("terminal anchor failed", err)
		}
	}
	if store.appends != 1 || len(store.history) != 1 {
		t.Fatal("terminal anchor was not retained exactly once")
	}
}
