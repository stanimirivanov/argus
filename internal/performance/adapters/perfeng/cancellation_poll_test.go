package perfeng

import (
	"context"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

type cancellationPollMemory struct {
	*cancellationConsumerMemory
	next  func(context.Context, requestintent.Scope, string, int64) (requestintent.Pending, error)
	delay func(context.Context, requestintent.Scope, string, int64, time.Duration) (time.Time, error)
}

func (s cancellationPollMemory) NextCancellation(ctx context.Context, scope requestintent.Scope, issuer string, after int64) (requestintent.Pending, error) {
	return s.next(ctx, scope, issuer, after)
}
func (s cancellationPollMemory) DeferCancellation(ctx context.Context, scope requestintent.Scope, issuer string, position int64, delay time.Duration) (time.Time, error) {
	return s.delay(ctx, scope, issuer, position, delay)
}

func cancellationPollFixture(t *testing.T, name string) (CancellationPoller, *cancellationPollMemory, *cancellationScenario, *[]time.Duration) {
	t.Helper()
	s := cancellationConsumptionFixture(t)
	configureCancellationConsumption(t, s, name)
	var delays []time.Duration
	store := &cancellationPollMemory{cancellationConsumerMemory: s.store}
	store.next = func(_ context.Context, scope requestintent.Scope, issuer string, _ int64) (requestintent.Pending, error) {
		if scope != s.scope || issuer != s.pending.Child.Request.Issuer {
			t.Fatal("untrusted selection binding")
		}
		return s.pending, nil
	}
	store.delay = func(_ context.Context, scope requestintent.Scope, issuer string, position int64, delay time.Duration) (time.Time, error) {
		if scope != s.scope || issuer != s.pending.Child.Request.Issuer || position != s.pending.After {
			t.Fatal("untrusted deferral binding")
		}
		delays = append(delays, delay)

		return time.Unix(1, 0).UTC(), nil // Server-owned instant; no client clock comparison.
	}
	client := cancellationConsumerClient(t, s, name)
	s.source.Client = &http.Client{Transport: integrationRoundTrip(func(r *http.Request) (*http.Response, error) {
		if len(delays) != 1 {
			t.Fatal("I/O preceded durable baseline or pass retried")
		}
		return client.Transport.RoundTrip(r)
	})}

	return CancellationPoller{Source: s.source, Store: store, Scope: s.scope, RequestIssuer: s.pending.Child.Request.Issuer, Interval: 5 * time.Second}, store, s, &delays
}

func TestCancellationPollBoundsConsumptionAndDefersBeforeIO(t *testing.T) {
	for _, name := range []string{"cancel", "terminal-history", "unadmitted", "lost-exact", "lost-absent", "rate-limited"} {
		t.Run(name, func(t *testing.T) {
			poll, _, s, delays := cancellationPollFixture(t, name)
			next, event, err := poll.One(t.Context(), 0)
			failure := name == "unadmitted" || name == "lost-absent" || name == "rate-limited"
			if next != s.pending.After || (err != nil) != failure || (event.Data().Evidence.Fingerprint() == "") != failure {
				t.Fatal("unexpected result", next, err)
			}
			want := []time.Duration{5 * time.Second}
			if name == "rate-limited" {
				want = append(want, 23*time.Second)
			}
			calls := 2
			if name == "terminal-history" || name == "unadmitted" {
				calls = 0
			} else if name == "rate-limited" {
				calls = 1
			}
			if !slices.Equal(*delays, want) || s.calls != calls {
				t.Fatal("unbounded calls/wrong deferral", s.calls, *delays)
			}
		})
	}
}

func TestCancellationPollFairCursorAndPartialSelectionFailures(t *testing.T) {
	poll, store, s, delays := cancellationPollFixture(t, "cancel")
	selections := 0
	store.next = func(_ context.Context, _ requestintent.Scope, _ string, after int64) (requestintent.Pending, error) {
		selections++
		switch after {
		case 0:
			return requestintent.Pending{After: 3}, requestintent.ErrUnavailable
		case 3:
			return requestintent.Pending{After: 4}, nil // Corrupt child must not pin traversal.
		case 4:
			return s.pending, nil
		default:
			return requestintent.Pending{}, requestintent.ErrNotFound
		}
	}
	var after int64
	for _, want := range []int64{3, 4, 7, 0} {
		next, _, err := poll.One(t.Context(), after)
		if next != want || (err == nil) != (want == 7) {
			t.Fatal("cursor fairness lost", next, err)
		}
		after = next
	}
	if selections != 4 || s.calls != 2 || len(*delays) != 1 {
		t.Fatal("pass rescanned or consumed invalid work")
	}
}

func TestCancellationPollRejectsInvalidInputsWithoutEffects(t *testing.T) {
	for _, name := range []string{"nil-store", "issuer", "scope", "source", "zero", "fractional", "negative", "cursor", "cancelled"} {
		t.Run(name, func(t *testing.T) {
			poll, store, s, _ := cancellationPollFixture(t, "cancel")
			store.next = func(context.Context, requestintent.Scope, string, int64) (requestintent.Pending, error) {
				t.Fatal("invalid configuration reached selection")
				return requestintent.Pending{}, nil
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			after := int64(0)
			switch name {
			case "nil-store":
				poll.Store = nil
			case "issuer":
				poll.RequestIssuer = ""
			case "scope":
				poll.Scope.PrincipalID = "other"
			case "source":
				poll.Source.Token = ""
			case "zero":
				poll.Interval = 0
			case "fractional":
				poll.Interval = 1500 * time.Millisecond
			case "negative":
				poll.Interval = -time.Second
			case "cursor":
				after = -1
			case "cancelled":
				cancel()
			}
			_, _, err := poll.One(ctx, after)
			if err == nil || s.calls != 0 {
				t.Fatal("invalid invocation permitted effects", err)
			}
		})
	}
}

func TestCancellationPollCandidateAndDeferralFailuresPermitNoIO(t *testing.T) {
	for _, name := range []string{"nonadvancing", "request-issuer", "attempt-issuer", "malformed", "selection", "deferral", "zero-deadline", "cancel-selection", "cancel-deferral"} {
		t.Run(name, func(t *testing.T) {
			poll, store, s, _ := cancellationPollFixture(t, "cancel")
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			nextFn, delayFn := store.next, store.delay
			store.next = func(ctx context.Context, scope requestintent.Scope, issuer string, after int64) (requestintent.Pending, error) {
				pending, err := nextFn(ctx, scope, issuer, after)
				switch name {
				case "nonadvancing":
					pending.After = after
				case "request-issuer":
					pending.Child.Request.Issuer = "https://other.example"
				case "attempt-issuer":
					pending.Child.Attempt.Issuer = "https://other.example"
				case "malformed":
					pending.Child.Payload = []byte(`{}`)
				case "selection":
					return requestintent.Pending{}, requestintent.ErrUnavailable
				case "cancel-selection":
					cancel()
				}

				return pending, err
			}
			store.delay = func(ctx context.Context, scope requestintent.Scope, issuer string, pos int64, delay time.Duration) (time.Time, error) {
				switch name {
				case "deferral":
					return time.Time{}, requestintent.ErrUnavailable
				case "zero-deadline":
					return time.Time{}, nil
				case "cancel-deferral":
					cancel()
				}

				return delayFn(ctx, scope, issuer, pos, delay)
			}
			_, _, err := poll.One(ctx, 0)
			if err == nil || s.calls != 0 {
				t.Fatal("unsafe candidate/uncertain deferral permitted I/O", err)
			}
		})
	}
}

func TestCancellationPollPreservesRetryHintAndDeferralErrors(t *testing.T) {
	deadline := time.Unix(1, 0).UTC()
	for _, test := range []struct {
		name      string
		interval  time.Duration
		until     time.Time
		err       error
		deferrals int
	}{
		{"shorter", 30 * time.Second, deadline, nil, 1},
		{"longer", 5 * time.Second, deadline, nil, 2},
		{"failed", 5 * time.Second, time.Time{}, requestintent.ErrUnavailable, 2},
		{"zero-deadline", 5 * time.Second, time.Time{}, nil, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			poll, store, _, delays := cancellationPollFixture(t, "rate-limited")
			poll.Interval = test.interval
			original := store.delay
			store.delay = func(ctx context.Context, scope requestintent.Scope, issuer string, pos int64, delay time.Duration) (time.Time, error) {
				until, err := original(ctx, scope, issuer, pos, delay)
				if len(*delays) == 2 {
					return test.until, test.err
				}

				return until, err
			}
			_, _, err := poll.One(t.Context(), 0)
			var status *IntegrationHTTPStatusError
			wantUnavailable := test.until.IsZero()
			if !errors.As(err, &status) || status.Code != 429 || status.RetryAfter != 23*time.Second || errors.Is(err, requestintent.ErrUnavailable) != wantUnavailable {
				t.Fatal("error evidence lost", err)
			}
			if len(*delays) != test.deferrals {
				t.Fatal("hint shortened cooldown or extension omitted", *delays)
			}
		})
	}
}

func TestCancellationPollUnavailableHintsAndCancelledIO(t *testing.T) {
	for _, test := range []struct {
		name      string
		status    int
		hint      string
		cancel    bool
		deferrals int
	}{
		{"503", 503, "23", false, 2},
		{"invalid-hint", 503, "1.5", false, 1},
		{"generic-failure", 400, "23", false, 1},
		{"cancelled-io", 0, "", true, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			poll, _, _, delays := cancellationPollFixture(t, "cancel")
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			calls := 0
			poll.Source.Client = &http.Client{Transport: integrationRoundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				if len(*delays) != 1 || r.Method != http.MethodGet {
					t.Fatal("missing baseline or retried provider call")
				}
				if test.cancel {
					cancel()
					return nil, ctx.Err()
				}

				return &http.Response{StatusCode: test.status, Header: http.Header{"Retry-After": {test.hint}}, Body: io.NopCloser(strings.NewReader("private diagnostic"))}, nil
			})}
			_, _, err := poll.One(ctx, 0)
			if err == nil || calls != 1 || len(*delays) != test.deferrals {
				t.Fatal("wrong provider/error backoff", err, calls, *delays)
			}
			if test.cancel && !errors.Is(err, context.Canceled) {
				t.Fatal("cancellation lost", err)
			}
		})
	}
}

func TestCancellationPollDefersAfterUncertainCancellationPOST(t *testing.T) {
	poll, _, _, delays := cancellationPollFixture(t, "cancel")
	client := poll.Source.Client
	calls := 0
	poll.Source.Client = &http.Client{Transport: integrationRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method == http.MethodPost {
			if !strings.HasSuffix(r.URL.Path, "/cancellation") {
				t.Fatal("original submission")
			}
			return &http.Response{StatusCode: 503, Header: http.Header{"Retry-After": {"41"}}, Body: io.NopCloser(strings.NewReader("private"))}, nil
		}

		return client.Transport.RoundTrip(r)
	})}
	_, event, err := poll.One(t.Context(), 0)
	var status *IntegrationHTTPStatusError
	if !errors.As(err, &status) || status.Code != 503 || calls != 2 || event.Data().Evidence.Fingerprint() != "" || !slices.Equal(*delays, []time.Duration{5 * time.Second, 41 * time.Second}) {
		t.Fatal("uncertain POST retried or hint lost", err, calls, *delays)
	}
}
