package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/stanimirivanov/argus/internal/performance/adapters/perfeng"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

type performanceWorkerConfig struct {
	enabled              bool
	dispatch             requestintent.Dispatcher
	poll                 time.Duration
	lifecycle            bool
	cancellation         bool
	cancellationInterval time.Duration
}

func loadPerformanceWorker(getenv func(string) string) (performanceWorkerConfig, error) {
	var result performanceWorkerConfig
	switch strings.TrimSpace(getenv("ARGUS_PERFORMANCE_DISPATCH_ENABLED")) {
	case "", "false":
		return result, nil
	case "true":
		result.enabled = true
	default:
		return result, errors.New("ARGUS_PERFORMANCE_DISPATCH_ENABLED must be true or false")
	}
	read := func(suffix string) string { return strings.TrimSpace(getenv("ARGUS_PERFORMANCE_DISPATCH_" + suffix)) }
	switch read("CANCELLATION_ENABLED") {
	case "", "false":
	case "true":
		result.cancellation = true
		var err error
		result.cancellationInterval, err = workerDuration(read("CANCELLATION_INTERVAL"), "CANCELLATION_INTERVAL", 5*time.Second, time.Minute)
		if err != nil {
			return performanceWorkerConfig{}, err
		}
		if result.cancellationInterval%time.Second != 0 {
			return performanceWorkerConfig{}, errors.New("ARGUS_PERFORMANCE_DISPATCH_CANCELLATION_INTERVAL must use whole seconds")
		}
	default:
		return performanceWorkerConfig{}, errors.New("ARGUS_PERFORMANCE_DISPATCH_CANCELLATION_ENABLED must be true or false")
	}
	switch read("LIFECYCLE_ENABLED") {
	case "", "false":
	case "true":
		result.lifecycle = true
	default:
		return performanceWorkerConfig{}, errors.New("ARGUS_PERFORMANCE_DISPATCH_LIFECYCLE_ENABLED must be true or false")
	}
	source := perfeng.IntegrationHTTP{Endpoint: read("URL"), Token: read("TOKEN"), Provider: read("PROVIDER"), Principal: perfeng.IntegrationIdentity{Issuer: read("PRINCIPAL_ISSUER"), ID: read("PRINCIPAL_ID")}}
	result.dispatch = requestintent.Dispatcher{Provider: source, Scope: requestintent.Scope{Issuer: source.Provider, PrincipalIssuer: source.Principal.Issuer, PrincipalID: source.Principal.ID}, RequestIssuer: read("REQUEST_ISSUER"), Now: time.Now}
	issuer, issuerErr := url.Parse(result.dispatch.RequestIssuer)
	if issuerErr != nil || !issuer.IsAbs() || issuer.User != nil || issuer.RawQuery != "" || issuer.Fragment != "" || !requestintent.ValidRequestIssuer(result.dispatch.RequestIssuer) || source.ValidateDispatch(result.dispatch.Scope) != nil {
		return performanceWorkerConfig{}, errors.New("invalid performance dispatch configuration")
	}
	var err error
	result.dispatch.Lease, err = workerDuration(read("LEASE"), "LEASE", 30*time.Second, 5*time.Minute)
	if err != nil {
		return performanceWorkerConfig{}, err
	}
	result.poll, err = workerDuration(read("POLL_INTERVAL"), "POLL_INTERVAL", 5*time.Second, time.Minute)

	return result, err
}

func workerDuration(raw, name string, fallback, maximum time.Duration) (time.Duration, error) {
	if raw == "" {
		return fallback, nil
	}
	duration, err := time.ParseDuration(raw)
	if err != nil || duration < time.Second || duration > maximum || duration%time.Microsecond != 0 {
		return 0, fmt.Errorf("ARGUS_PERFORMANCE_DISPATCH_%s must be 1s..%s in whole microseconds", name, maximum)
	}

	return duration, nil
}

type performanceWorker struct {
	stop context.CancelFunc
	done <-chan struct{}
}

func (worker *performanceWorker) close() {
	worker.stop()
	<-worker.done
}

// start is called once by the composition root, after all resources open. Only
// configured scope/credentials are used; retained messages cannot reroute I/O.
func (c performanceWorkerConfig) start(ctx context.Context, store performanceRuntimeStore, logger *slog.Logger) *performanceWorker {
	if !c.enabled {
		return nil
	}
	ctx, cancel := context.WithCancel(ctx)
	ticker := time.NewTicker(c.poll)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer ticker.Stop()
		var state performanceWorkerState
		for ctx.Err() == nil {
			c.step(ctx, store, logger, &state)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()

	return &performanceWorker{stop: cancel, done: done}
}

type performanceRuntimeStore interface {
	requestintent.DispatchStore
	perfeng.CancellationPollStore
	NextLifecycle(context.Context, requestintent.Scope, string, int64) (requestintent.Pending, error)
}

func (c performanceWorkerConfig) pollLifecycle(ctx context.Context, store performanceRuntimeStore, after int64, logger *slog.Logger) (int64, error) {
	next, err := c.consumeLifecycle(ctx, store, after)
	if err == nil {
		logger.Info("performance lifecycle retained")
	} else if !errors.Is(err, requestintent.ErrNotFound) && ctx.Err() == nil {
		logger.Warn("performance lifecycle unresolved")
	}

	return next, err
}

type performanceWorkerState struct {
	lifecycleAfter, cancellationAfter int64
	retryAt                           time.Time
}

// Recovery runs before cancellation: only retained admission is selectable. A
// previously acquired cancelled request stays lookup-only at a higher fence;
// cancelled work with no claim stays behind the storage first-send cutoff.
func (c performanceWorkerConfig) step(ctx context.Context, store performanceRuntimeStore, logger *slog.Logger, state *performanceWorkerState) {
	if ctx.Err() != nil || (c.cancellation && c.dispatch.Now().Before(state.retryAt)) {
		return
	}
	dispatch := c.dispatch
	dispatch.Store = store
	outcome, err := dispatch.One(ctx)
	if ctx.Err() != nil {
		return
	}
	if err == nil {
		logger.Info("performance dispatch retained", "kind", outcome.Data().Kind)
	} else if !errors.Is(err, requestintent.ErrNotFound) {
		logger.Warn("performance dispatch unresolved")
	}
	if c.cancellation {
		if c.pauseProvider(err, state, logger) || !c.pollCancellation(ctx, store, logger, state) {
			return
		}
	}
	if c.lifecycle && ctx.Err() == nil {
		state.lifecycleAfter, err = c.pollLifecycle(ctx, store, state.lifecycleAfter, logger)
		if c.cancellation {
			c.pauseProvider(err, state, logger)
		}
	}
}

// False suppresses later stages on cancellation errors, preventing a
// same-iteration lifecycle bypass of failed cooldown retention.
func (c performanceWorkerConfig) pollCancellation(ctx context.Context, store performanceRuntimeStore, logger *slog.Logger, state *performanceWorkerState) bool {
	source, ok := c.dispatch.Provider.(perfeng.IntegrationHTTP)
	if !ok {
		logger.Warn("performance cancellation unresolved")
		return false
	}
	poll := perfeng.CancellationPoller{Source: source, Store: store, Scope: c.dispatch.Scope, RequestIssuer: c.dispatch.RequestIssuer, Interval: c.cancellationInterval}
	var err error
	state.cancellationAfter, _, err = poll.One(ctx, state.cancellationAfter)
	if ctx.Err() != nil {
		return false
	}
	if err == nil {
		logger.Info("performance cancellation retained")
	} else if !errors.Is(err, requestintent.ErrNotFound) {
		logger.Warn("performance cancellation unresolved")
	}

	return !c.pauseProvider(err, state, logger) && (err == nil || errors.Is(err, requestintent.ErrNotFound))
}

// Process-local pacing also covers unresolved admission (which has no durable
// cancellation cooldown yet). Admitted per-request cooldown remains in storage.
// Never log raw errors, identities, payloads or credentials from any stage.
func (c performanceWorkerConfig) pauseProvider(err error, state *performanceWorkerState, logger *slog.Logger) bool {
	var status *perfeng.IntegrationHTTPStatusError
	if !errors.As(err, &status) || (status.Code != http.StatusTooManyRequests && status.Code != http.StatusServiceUnavailable) || status.RetryAfter < time.Second || status.RetryAfter%time.Second != 0 {
		return false
	}
	state.retryAt = c.dispatch.Now().Add(status.RetryAfter)
	logger.Warn("performance provider paused", "retry_after_seconds", int64(status.RetryAfter/time.Second))

	return true
}

// One candidate per tick; advancing even on provider failure prevents starvation.
// Reset only at end of traversal. No terminal/deadline inference affects admission.
func (c performanceWorkerConfig) consumeLifecycle(ctx context.Context, store performanceRuntimeStore, after int64) (int64, error) {
	pending, err := store.NextLifecycle(ctx, c.dispatch.Scope, c.dispatch.RequestIssuer, after)
	if errors.Is(err, requestintent.ErrNotFound) {
		return 0, err
	}
	if pending.After > after {
		after = pending.After
	}
	if err != nil {
		return after, err
	}
	source, ok := c.dispatch.Provider.(perfeng.IntegrationHTTP)
	if !ok {
		return after, requestintent.ErrInvalid
	}
	_, err = source.ConsumeLifecycle(ctx, store, c.dispatch.Scope, pending)

	return after, err
}
