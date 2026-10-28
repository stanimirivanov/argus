package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

func workerEnvironment() map[string]string {
	return map[string]string{
		"ENABLED": "true", "URL": "https://provider.example/integration/v1/requests", "TOKEN": "worker-secret",
		"PROVIDER": "https://provider.example", "PRINCIPAL_ISSUER": "https://auth.example", "PRINCIPAL_ID": "argus", "REQUEST_ISSUER": "https://argus.example",
	}
}

func workerConfiguration(t *testing.T, values map[string]string) performanceWorkerConfig {
	t.Helper()
	c, err := loadPerformanceWorker(func(name string) string { return values[strings.TrimPrefix(name, "ARGUS_PERFORMANCE_DISPATCH_")] })
	if err != nil {
		t.Fatal(err)
	}

	return c
}

func TestPerformanceWorkerConfigurationIsExplicitAndBounded(t *testing.T) {
	for _, enabled := range []string{"", "false"} {
		c, err := loadPerformanceWorker(func(name string) string {
			if name != "ARGUS_PERFORMANCE_DISPATCH_ENABLED" {
				t.Fatal("disabled worker loaded credentials")
			}
			return enabled
		})
		if err != nil || c.enabled || c.start(t.Context(), nil, nil) != nil {
			t.Fatal("default dispatch activated", err)
		}
	}
	c := workerConfiguration(t, workerEnvironment())
	if c.dispatch.Lease != 30*time.Second || c.poll != 5*time.Second || c.dispatch.Scope.PrincipalID != "argus" || c.dispatch.RequestIssuer != "https://argus.example" {
		t.Fatal("wrong trusted binding/defaults")
	}
	for _, change := range [][2]string{{"ENABLED", "yes"}, {"URL", "http://untrusted.example"}, {"TOKEN", ""}, {"PROVIDER", ""}, {"PRINCIPAL_ISSUER", ""}, {"PRINCIPAL_ID", ""}, {"REQUEST_ISSUER", ""}, {"REQUEST_ISSUER", "relative"}, {"LEASE", "501ms"}, {"LEASE", "5m1s"}, {"LEASE", "1.000000001s"}, {"POLL_INTERVAL", "0s"}, {"POLL_INTERVAL", "2m"}, {"POLL_INTERVAL", "secret-invalid"}} {
		values := workerEnvironment()
		values[change[0]] = change[1]
		_, err := loadPerformanceWorker(func(name string) string { return values[strings.TrimPrefix(name, "ARGUS_PERFORMANCE_DISPATCH_")] })
		if err == nil || strings.Contains(err.Error(), "worker-secret") || strings.Contains(err.Error(), "secret-invalid") {
			t.Fatal("invalid/leaking configuration", change[0], err)
		}
	}
}

type lifecycleCandidates struct {
	workerClaim
	next func(context.Context, requestintent.Scope, string, int64) (requestintent.Pending, error)
}

func (s lifecycleCandidates) NextLifecycle(ctx context.Context, scope requestintent.Scope, issuer string, after int64) (requestintent.Pending, error) {
	return s.next(ctx, scope, issuer, after)
}

func TestLifecycleWorkerOptInCursorRotationAndFailureFairness(t *testing.T) {
	values := workerEnvironment()
	if workerConfiguration(t, values).lifecycle {
		t.Fatal("lifecycle enabled by default")
	}
	values["LIFECYCLE_ENABLED"] = "true"
	c := workerConfiguration(t, values)
	if !c.lifecycle {
		t.Fatal("explicit lifecycle flag ignored")
	}
	store := lifecycleCandidates{next: func(_ context.Context, scope requestintent.Scope, issuer string, after int64) (requestintent.Pending, error) {
		if scope != c.dispatch.Scope || issuer != c.dispatch.RequestIssuer {
			t.Fatal("scope changed")
		}
		if after == 9 {
			return requestintent.Pending{}, requestintent.ErrNotFound
		}

		return requestintent.Pending{After: 9}, nil // Bad child must fail without pinning the cursor.
	}}
	after, err := c.consumeLifecycle(t.Context(), store, 0)
	if after != 9 || err == nil {
		t.Fatal("bad candidate blocked traversal", after, err)
	}
	after, err = c.consumeLifecycle(t.Context(), store, after)
	if after != 0 || !errors.Is(err, requestintent.ErrNotFound) {
		t.Fatal("cursor did not wrap", after, err)
	}
	values["LIFECYCLE_ENABLED"] = "yes"
	if _, err := loadPerformanceWorker(func(name string) string { return values[strings.TrimPrefix(name, "ARGUS_PERFORMANCE_DISPATCH_")] }); err == nil {
		t.Fatal("invalid lifecycle flag accepted")
	}
}

func TestLifecycleWorkerCancelsTraversalAndJoins(t *testing.T) {
	values := workerEnvironment()
	values["LIFECYCLE_ENABLED"] = "true"
	entered := make(chan struct{})
	store := lifecycleCandidates{
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
		t.Fatal("lifecycle traversal did not start")
	}
	worker.close()
	select {
	case <-worker.done:
	default:
		t.Fatal("lifecycle worker was not joined")
	}
}

type workerClaim struct {
	performanceRuntimeStore
	claim func(context.Context) (requestintent.Claim, error)
}

func (s workerClaim) ClaimRequestForIssuer(ctx context.Context, _ requestintent.Scope, _ string, _ time.Duration) (requestintent.Claim, error) {
	return s.claim(ctx)
}

func TestPerformanceWorkerCancelsIOAndJoins(t *testing.T) {
	entered := make(chan struct{})
	store := workerClaim{claim: func(ctx context.Context) (requestintent.Claim, error) {
		close(entered)
		<-ctx.Done()
		return requestintent.Claim{}, ctx.Err()
	}}
	worker := workerConfiguration(t, workerEnvironment()).start(t.Context(), store, slog.Default())
	defer worker.close()
	select {
	case <-entered:
	case <-time.After(testTimeout):
		t.Fatal("worker did not acquire")
	}
	worker.close()
	select {
	case <-worker.done:
	default:
		t.Fatal("worker was not joined")
	}
}

func TestPerformanceWorkerRedactsOperationalFailures(t *testing.T) {
	entries := make(chan []byte, 2)
	store := workerClaim{claim: func(context.Context) (requestintent.Claim, error) {
		return requestintent.Claim{}, errors.New("worker-secret private provider body")
	}}
	worker := workerConfiguration(t, workerEnvironment()).start(t.Context(), store, slog.New(slog.NewJSONHandler(channelWriter{entries: entries}, nil)))
	defer worker.close()
	entry := waitForLogEntry(t, entries)
	assertLogValue(t, entry, "msg", "performance dispatch unresolved")
	raw, err := json.Marshal(entry)
	if err != nil || strings.Contains(string(raw), "worker-secret") || strings.Contains(string(raw), "private") {
		t.Fatal("worker leaked diagnostics", err)
	}
}
