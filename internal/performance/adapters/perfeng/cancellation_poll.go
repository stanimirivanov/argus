package perfeng

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

// CancellationPollStore selects due admitted intent and durably defers it using
// server time. Successful deferral returns a nonzero deadline and never shortens
// an existing one. Both cancellation and lifecycle selectors must honor it.
// Selection is not a claim; implementations retain the CancellationStore guards.
type CancellationPollStore interface {
	CancellationStore
	NextCancellation(context.Context, requestintent.Scope, string, int64) (requestintent.Pending, error)
	DeferCancellation(context.Context, requestintent.Scope, string, int64, time.Duration) (time.Time, error)
}

// CancellationPoller is a callable step, not an activated worker. Configuration
// and ports must be independently authorized and immutable during calls. Interval
// is a positive whole-second baseline, not a lease or provider-wide rate limit.
// Callers own serial scheduling, cursor reuse, cancellation and joining active I/O.
type CancellationPoller struct {
	Source        IntegrationHTTP
	Store         CancellationPollStore
	Scope         requestintent.Scope
	RequestIssuer string
	Interval      time.Duration
}

// One selects at most one candidate and consumes it only after durable deferral.
// Returns the next exclusive cursor, a retained observation and any error. End
// resets to zero without another scan; candidate failures advance for fairness,
// including a usable position on partial reads. Callers must retain this cursor
// even on error. Nonadvancing results fail closed. No sleeps or original submits.
// Each consumption performs at most one GET and one cancellation POST. Longer
// valid 429/503 hints extend cooldown; errors preserve uncertain remote/local work.
// Already selected/in-flight work is not revoked by another caller's deferral.
func (p CancellationPoller) One(ctx context.Context, after int64) (int64, requestintent.LifecycleEvent, error) {
	if p.Store == nil || after < 0 || !requestintent.ValidRequestIssuer(p.RequestIssuer) || p.Interval < time.Second || p.Interval%time.Second != 0 || p.Source.ValidateDispatch(p.Scope) != nil {
		return after, requestintent.LifecycleEvent{}, requestintent.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return after, requestintent.LifecycleEvent{}, err
	}
	pending, err := p.Store.NextCancellation(ctx, p.Scope, p.RequestIssuer, after)
	if errors.Is(err, requestintent.ErrNotFound) {
		return 0, requestintent.LifecycleEvent{}, err
	}
	previous := after
	if pending.After > after {
		after = pending.After
	}
	if err != nil {
		return after, requestintent.LifecycleEvent{}, err
	}
	if err := ctx.Err(); err != nil {
		return after, requestintent.LifecycleEvent{}, err
	}
	child := pending.Child
	if pending.After <= previous || child.Request.Issuer != p.RequestIssuer || child.Attempt.Issuer != p.RequestIssuer || !p.Source.validDispatchChild(p.Scope, child) {
		return after, requestintent.LifecycleEvent{}, requestintent.ErrInvalid
	}
	until, err := p.Store.DeferCancellation(ctx, p.Scope, p.RequestIssuer, after, p.Interval)
	if err != nil {
		return after, requestintent.LifecycleEvent{}, err
	}
	if until.IsZero() {
		return after, requestintent.LifecycleEvent{}, requestintent.ErrUnavailable
	}
	event, err := p.Source.ConsumeCancellation(ctx, p.Store, p.Scope, pending)
	var status *IntegrationHTTPStatusError
	if errors.As(err, &status) && (status.Code == http.StatusTooManyRequests || status.Code == http.StatusServiceUnavailable) && status.RetryAfter > p.Interval && status.RetryAfter%time.Second == 0 {
		until, deferErr := p.Store.DeferCancellation(ctx, p.Scope, p.RequestIssuer, after, status.RetryAfter)
		if deferErr == nil && until.IsZero() {
			deferErr = requestintent.ErrUnavailable
		}
		err = errors.Join(err, deferErr)
	}

	return after, event, err
}
