package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/performance"
	"github.com/stanimirivanov/argus/internal/performance/adapters/perfeng"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

type cancellationWorkerCandidates struct {
	workerClaim
	next func(context.Context, requestintent.Scope, string, int64) (requestintent.Pending, error)
}

func (s cancellationWorkerCandidates) NextCancellation(ctx context.Context, scope requestintent.Scope, issuer string, after int64) (requestintent.Pending, error) {
	return s.next(ctx, scope, issuer, after)
}

func TestCancellationWorkerConfigurationIsExplicit(t *testing.T) {
	values := workerEnvironment()
	values["CANCELLATION_INTERVAL"] = "private-invalid"
	if workerConfiguration(t, values).cancellation {
		t.Fatal("default activated cancellation")
	}
	values["CANCELLATION_ENABLED"] = "true"
	for _, interval := range []string{"0s", "61s", "1.5s", "private-invalid"} {
		values["CANCELLATION_INTERVAL"] = interval
		_, err := loadPerformanceWorker(func(name string) string { return values[strings.TrimPrefix(name, "ARGUS_PERFORMANCE_DISPATCH_")] })
		if err == nil || strings.Contains(err.Error(), "private-invalid") {
			t.Fatal("invalid/leaking interval", err)
		}
	}
	values["CANCELLATION_INTERVAL"] = ""
	values["POLL_INTERVAL"] = "1500ms"
	c := workerConfiguration(t, values)
	if !c.cancellation || c.cancellationInterval != 5*time.Second || c.poll != 1500*time.Millisecond {
		t.Fatal("wrong cancellation defaults")
	}
	values["CANCELLATION_ENABLED"] = "yes"
	if _, err := loadPerformanceWorker(func(name string) string { return values[strings.TrimPrefix(name, "ARGUS_PERFORMANCE_DISPATCH_")] }); err == nil {
		t.Fatal("invalid flag accepted")
	}
}

func TestCancellationWorkerPacesRecoveryAndPreservesFairCursor(t *testing.T) {
	c := workerConfiguration(t, workerEnvironment())
	c.cancellation, c.lifecycle, c.cancellationInterval = true, true, 5*time.Second
	now := time.Unix(1, 0)
	c.dispatch.Now = func() time.Time { return now }
	claims, selections := 0, 0
	store := cancellationWorkerCandidates{
		workerClaim: workerClaim{claim: func(context.Context) (requestintent.Claim, error) {
			claims++
			if claims == 1 {
				return requestintent.Claim{}, &perfeng.IntegrationHTTPStatusError{Code: 429, RetryAfter: 23 * time.Second}
			}

			return requestintent.Claim{}, requestintent.ErrNotFound
		}},
		next: func(_ context.Context, scope requestintent.Scope, issuer string, after int64) (requestintent.Pending, error) {
			selections++
			if scope != c.dispatch.Scope || issuer != c.dispatch.RequestIssuer {
				t.Fatal("untrusted worker scope")
			}
			if after == 0 {
				return requestintent.Pending{After: 7}, nil
			}
			if after != 7 {
				t.Fatal("cursor lost on failure")
			}

			return requestintent.Pending{}, requestintent.ErrNotFound
		},
	}
	state := performanceWorkerState{lifecycleAfter: 99}
	var logs strings.Builder
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	c.step(t.Context(), store, logger, &state)
	c.step(t.Context(), store, logger, &state)
	if claims != 1 || selections != 0 {
		t.Fatal("recovery hint bypassed")
	}
	if !strings.Contains(logs.String(), "retry_after_seconds=23") {
		t.Fatal("safe pause telemetry missing")
	}
	now = now.Add(23 * time.Second)
	c.step(t.Context(), store, logger, &state)
	if state.cancellationAfter != 7 || state.lifecycleAfter != 99 {
		t.Fatal("independent cursors/failed candidate bypass lost")
	}
	// The next step wraps cancellation; cancel before lifecycle can run.
	ctx, cancel := context.WithCancel(t.Context())
	original := store.next
	store.next = func(ctx context.Context, scope requestintent.Scope, issuer string, after int64) (requestintent.Pending, error) {
		pending, err := original(ctx, scope, issuer, after)
		cancel()
		return pending, err
	}
	c.step(ctx, store, logger, &state)
	if state.cancellationAfter != 0 || claims != 3 {
		t.Fatal("cursor did not wrap")
	}
}

func TestCancellationWorkerShutdownCancelsAndJoinsSelection(t *testing.T) {
	values := workerEnvironment()
	values["CANCELLATION_ENABLED"] = "true"
	entered := make(chan struct{})
	store := cancellationWorkerCandidates{
		workerClaim: workerClaim{claim: func(context.Context) (requestintent.Claim, error) {
			return requestintent.Claim{}, requestintent.ErrNotFound
		}},
		next: func(ctx context.Context, _ requestintent.Scope, _ string, _ int64) (requestintent.Pending, error) {
			close(entered)
			<-ctx.Done()
			return requestintent.Pending{}, ctx.Err()
		},
	}
	worker := workerConfiguration(t, values).start(t.Context(), store, slog.Default())
	defer worker.close()
	select {
	case <-entered:
	case <-time.After(testTimeout):
		t.Fatal("cancellation selector did not start")
	}
	worker.close()
	select {
	case <-worker.done:
	default:
		t.Fatal("worker not joined")
	}
}

type cancellationWorkerEvidence struct {
	cancellationWorkerCandidates
	outcome requestintent.Outcome
	intent  requestintent.Cancellation
	trace   []string
}

func (s *cancellationWorkerEvidence) RequestOutcome(context.Context, requestintent.Scope, int64) (requestintent.Outcome, error) {
	if s.outcome.Fingerprint() == "" {
		return requestintent.Outcome{}, requestintent.ErrNotFound
	}
	return s.outcome, nil
}
func (s *cancellationWorkerEvidence) SettleRequest(_ context.Context, _ requestintent.Scope, claim requestintent.Claim, outcome requestintent.Outcome) (bool, error) {
	if claim.Fence != 2 {
		return false, requestintent.ErrInvalid
	}
	s.trace = append(s.trace, "settle")
	s.outcome = outcome

	return true, nil
}
func (s *cancellationWorkerEvidence) GetCancellation(context.Context, requestintent.Scope, int64, requestintent.CancellationVerifier) (requestintent.Cancellation, error) {
	return s.intent, nil
}
func (s *cancellationWorkerEvidence) LifecycleHistory(context.Context, requestintent.Scope, int64, requestintent.LifecycleAdmission, requestintent.LifecycleVerifier) ([]requestintent.LifecycleEvent, error) {
	return nil, nil
}
func (s *cancellationWorkerEvidence) AppendLifecycle(context.Context, requestintent.Scope, int64, requestintent.LifecycleAdmission, requestintent.LifecycleEvent, requestintent.LifecycleVerifier) (bool, error) {
	s.trace = append(s.trace, "history")
	return true, nil
}
func (s *cancellationWorkerEvidence) DeferCancellation(context.Context, requestintent.Scope, string, int64, time.Duration) (time.Time, error) {
	s.trace = append(s.trace, "defer")
	return time.Unix(1, 0), nil
}

type cancellationWorkerTransport func(*http.Request) (*http.Response, error)

func (f cancellationWorkerTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func cancellationRecoveryFixture(t *testing.T) (performanceWorkerConfig, *cancellationWorkerEvidence) {
	t.Helper()
	raw, err := os.ReadFile("../../internal/performance/adapters/perfeng/snapshot/integration/cancelled.json")
	if err != nil {
		t.Fatal(err)
	}
	var messages []json.RawMessage
	if err := json.Unmarshal(raw, &messages); err != nil {
		t.Fatal(err)
	}
	original, err := perfeng.DecodeIntegration(messages[0])
	if err != nil {
		t.Fatal(err)
	}
	reply, err := perfeng.DecodeIntegration(messages[4])
	if err != nil {
		t.Fatal(err)
	}
	intent, err := perfeng.BindCancellation(messages[0], messages[2])
	if err != nil {
		t.Fatal(err)
	}
	requested, err := time.Parse(time.RFC3339Nano, original.Data.Context.RequestedAt)
	if err != nil {
		t.Fatal(err)
	}
	deadline, err := time.Parse(time.RFC3339Nano, original.Data.Context.Deadline)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(messages[0])
	child := requestintent.Child{Request: performance.Identity(original.Data.Context.Request), Attempt: performance.Identity(original.Data.Context.Attempt), EventID: original.ID, SHA256: hex.EncodeToString(digest[:]), Payload: messages[0], RequestedAt: requested, Deadline: deadline, Reasons: []requestintent.Reason{{MappingID: "reviewed", Capability: "checkout", Reason: "approved"}}}
	store := &cancellationWorkerEvidence{intent: intent}
	c := workerConfiguration(t, workerEnvironment())
	c.cancellation, c.cancellationInterval = true, 5*time.Second
	c.dispatch.Scope = requestintent.Scope{Issuer: reply.Source, PrincipalIssuer: original.Data.Context.Principal.Issuer, PrincipalID: original.Data.Context.Principal.ID}
	c.dispatch.RequestIssuer = child.Request.Issuer
	c.dispatch.Now = func() time.Time { return requested }
	c.dispatch.Provider = perfeng.IntegrationHTTP{Endpoint: "https://provider.example/integration/v1/requests", Token: "worker-secret", Provider: reply.Source, Principal: original.Data.Context.Principal, Client: &http.Client{Transport: cancellationWorkerTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer worker-secret" {
			t.Fatal("recovery submitted or lacked authentication")
		}
		store.trace = append(store.trace, "lookup")

		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(string(messages[4])))}, nil
	})}}
	store.claim = func(context.Context) (requestintent.Claim, error) {
		return requestintent.Claim{Pending: requestintent.Pending{After: 7, Child: child}, Fence: 2, Token: "recovered", Until: deadline}, nil
	}
	store.next = func(context.Context, requestintent.Scope, string, int64) (requestintent.Pending, error) {
		if store.outcome.Fingerprint() == "" {
			t.Fatal("cancellation preceded retained admission")
		}
		store.trace = append(store.trace, "select")

		return requestintent.Pending{After: 7, Child: child}, nil
	}

	return c, store
}

func TestCancellationWorkerReconcilesAdmissionBeforeConsumingTerminalCancellation(t *testing.T) {
	c, store := cancellationRecoveryFixture(t)
	var state performanceWorkerState
	c.step(t.Context(), store, slog.New(slog.NewTextHandler(io.Discard, nil)), &state)
	if !slices.Equal(store.trace, []string{"lookup", "settle", "select", "defer", "history"}) || state.cancellationAfter != 7 {
		t.Fatal("wrong admission/cancellation order", store.trace)
	}
}

func TestCancellationWorkerUnresolvedRecoveryDoesNotGrantAdmission(t *testing.T) {
	c, store := cancellationRecoveryFixture(t)
	source := c.dispatch.Provider.(perfeng.IntegrationHTTP)
	source.Client = &http.Client{Transport: cancellationWorkerTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet {
			t.Fatal("absence authorized original POST")
		}
		store.trace = append(store.trace, "absent")

		return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("worker-secret private"))}, nil
	})}
	c.dispatch.Provider = source
	store.next = func(context.Context, requestintent.Scope, string, int64) (requestintent.Pending, error) {
		return requestintent.Pending{}, requestintent.ErrNotFound
	}
	var output strings.Builder
	c.step(t.Context(), store, slog.New(slog.NewJSONHandler(&output, nil)), &performanceWorkerState{})
	if !slices.Equal(store.trace, []string{"absent"}) || store.outcome.Fingerprint() != "" || strings.Contains(output.String(), "worker-secret") || !strings.Contains(output.String(), "dispatch unresolved") {
		t.Fatal("unresolved recovery granted effects or leaked logs")
	}
}

func TestCancellationWorkerHintFromPollStopsLaterStages(t *testing.T) {
	c, store := cancellationRecoveryFixture(t)
	c.lifecycle = true
	store.claim = func(context.Context) (requestintent.Claim, error) {
		return requestintent.Claim{}, requestintent.ErrNotFound
	}
	store.next = func(context.Context, requestintent.Scope, string, int64) (requestintent.Pending, error) {
		return requestintent.Pending{After: 7}, &perfeng.IntegrationHTTPStatusError{Code: 503, RetryAfter: time.Minute}
	}
	var state performanceWorkerState
	c.step(t.Context(), store, slog.New(slog.NewTextHandler(io.Discard, nil)), &state)
	if state.retryAt != c.dispatch.Now().Add(time.Minute) || state.cancellationAfter != 7 {
		t.Fatal("poll hint lost")
	}
	store.claim = func(context.Context) (requestintent.Claim, error) {
		t.Fatal("provider work resumed too soon")
		return requestintent.Claim{}, nil
	}
	c.step(t.Context(), store, slog.Default(), &state)
}
