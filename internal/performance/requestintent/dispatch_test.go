package requestintent

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

type dispatchStore struct {
	*recoveryStore
	claims   int
	claimErr error
}

func (s *dispatchStore) ClaimRequestForIssuer(_ context.Context, scope Scope, issuer string, lease time.Duration) (Claim, error) {
	s.claims++
	if scope != s.scope || issuer == "" || lease != time.Minute {
		s.t.Fatal("acquisition changed trusted configuration")
	}

	return s.claim, s.claimErr
}

type dispatchProvider struct {
	recoveryProvider
	submit        func(context.Context, Scope, Child) (Outcome, error)
	validationErr error
}

func (p dispatchProvider) ValidateDispatch(Scope) error { return p.validationErr }

func (p dispatchProvider) SubmitOutcome(ctx context.Context, scope Scope, child Child) (Outcome, error) {
	return p.submit(ctx, scope, child)
}

func dispatchFixture(t *testing.T) (Dispatcher, *dispatchStore, Outcome) {
	t.Helper()
	recovery, evidence := recoveryFixture(t)
	recovery.claim.Fence, recovery.commit = 1, true
	recovery.claim.Until = recovery.claim.Pending.Child.Deadline
	data := evidence.Data()
	data.Kind = "accepted"
	outcome, err := NewOutcome(data)
	if err != nil {
		t.Fatal(err)
	}
	store := &dispatchStore{recoveryStore: recovery}
	dispatcher := Dispatcher{Store: store, Scope: store.scope, RequestIssuer: store.claim.Pending.Child.Request.Issuer, Lease: time.Minute, Now: func() time.Time { return store.claim.Pending.Child.RequestedAt.Add(time.Second) }}

	return dispatcher, store, outcome
}

func TestDispatcherSubmitsFirstAcquisitionAndReadsLostSettlement(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"accepted", "rejected"} {
		d, store, evidence := dispatchFixture(t)
		if kind == "accepted" {
			store.claim.Until = d.Now().Add(10 * time.Second) // Lease, rather than request, limits this send.
		}
		data := evidence.Data()
		data.Kind = kind
		expected, err := NewOutcome(data)
		if err != nil {
			t.Fatal(err)
		}
		store.settleErr = ErrUnavailable // Commit succeeds; acknowledgement is lost.
		sends := 0
		d.Provider = dispatchProvider{submit: func(ctx context.Context, scope Scope, child Child) (Outcome, error) {
			sends++
			deadline, bounded := ctx.Deadline()
			limit := time.Now().Add(min(child.Deadline.Sub(d.Now()), store.claim.Until.Sub(d.Now())) + time.Second)
			if !bounded || deadline.After(limit) || scope != d.Scope || !reflect.DeepEqual(child, store.claim.Pending.Child) {
				t.Fatal("unbounded or changed submission")
			}

			return expected, nil
		}}
		got, err := d.One(t.Context())
		if err != nil || got.Fingerprint() != expected.Fingerprint() || store.claims != 1 || store.writes != 1 || store.reads != 1 || sends != 1 {
			t.Fatal("submission/readback failed", err)
		}
	}
}

func TestDispatcherRecoversReacquisitionAndExpiredWorkWithoutSubmission(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"reacquired", "expired request", "expired lease", "retained"} {
		d, store, outcome := dispatchFixture(t)
		switch mode {
		case "reacquired":
			store.claim.Fence++
		case "expired request":
			d.Now = func() time.Time { return store.claim.Pending.Child.Deadline }
		case "expired lease":
			store.claim.Until = d.Now()
		case "retained":
			store.claim.Fence++
			store.retained = outcome
		}
		lookups := 0
		d.Provider = dispatchProvider{recoveryProvider: func(_ context.Context, scope Scope, child Child) (Outcome, error) {
			lookups++
			if scope != d.Scope || !reflect.DeepEqual(child, store.claim.Pending.Child) {
				t.Fatal("recovery changed frozen evidence")
			}

			return outcome, nil
		}}
		got, err := d.One(t.Context())
		if err != nil || got.Fingerprint() != outcome.Fingerprint() || store.claims != 1 {
			t.Fatal("recovery failed", err)
		}
		if (mode == "retained" && lookups != 0) || lookups > 1 {
			t.Fatal("unexpected lookup", lookups)
		}
	}
}

func TestDispatcherDoesNotRepeatUncertainSubmission(t *testing.T) {
	t.Parallel()
	d, store, _ := dispatchFixture(t)
	sends, lookups := 0, 0
	d.Provider = dispatchProvider{
		submit:           func(context.Context, Scope, Child) (Outcome, error) { sends++; return Outcome{}, ErrUnavailable },
		recoveryProvider: func(context.Context, Scope, Child) (Outcome, error) { lookups++; return Outcome{}, ErrNotFound },
	}
	if got, err := d.One(t.Context()); !errors.Is(err, ErrUnavailable) || got.Fingerprint() != "" {
		t.Fatal(err)
	}
	store.claim.Fence++ // Acquisition after lease expiry/crash; absence proves nothing.
	if got, err := d.One(t.Context()); !errors.Is(err, ErrNotFound) || got.Fingerprint() != "" || sends != 1 || lookups != 1 || store.writes != 0 {
		t.Fatal("uncertainty retried POST", err)
	}
}

func TestDispatcherCancellationAfterSubmissionRetainsUncertainty(t *testing.T) {
	t.Parallel()
	d, store, outcome := dispatchFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	d.Provider = dispatchProvider{submit: func(context.Context, Scope, Child) (Outcome, error) {
		cancel() // Provider may already have admitted; do not acknowledge locally.
		return outcome, nil
	}}
	if got, err := d.One(ctx); !errors.Is(err, context.Canceled) || got.Fingerprint() != "" || store.writes != 0 {
		t.Fatal("cancelled submission acknowledged", err)
	}
}

func TestDispatcherRejectsLifecycleReplyToSubmission(t *testing.T) {
	t.Parallel()
	d, store, _ := dispatchFixture(t)
	_, lifecycle := recoveryFixture(t)
	d.Provider = dispatchProvider{submit: func(context.Context, Scope, Child) (Outcome, error) { return lifecycle, nil }}
	if got, err := d.One(t.Context()); !errors.Is(err, ErrInvalid) || got.Fingerprint() != "" || store.writes != 0 {
		t.Fatal("lifecycle reply settled submission", err)
	}
}

func TestDispatcherFailsClosedBeforeOrAfterAcquisition(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"scope", "empty issuer", "lease", "clock", "preflight", "claim", "issuer", "attempt issuer", "future", "zero clock", "cancel", "reply"} {
		d, store, _ := dispatchFixture(t)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		want, claims := ErrInvalid, 1
		switch mode {
		case "scope", "empty issuer", "lease", "clock", "preflight":
			claims = 0
			switch mode {
			case "scope":
				d.Scope.PrincipalID = ""
			case "empty issuer":
				d.RequestIssuer = ""
			case "lease":
				d.Lease = time.Microsecond
			case "clock":
				d.Now = nil
			}
		case "claim":
			store.claimErr, want = ErrUnavailable, ErrUnavailable
		case "issuer":
			d.RequestIssuer += "-other"
		case "attempt issuer":
			store.claim.Pending.Child.Attempt.Issuer += "-other"
		case "future":
			d.Now = func() time.Time { return store.claim.Pending.Child.RequestedAt.Add(-time.Second) }
		case "zero clock":
			d.Now = func() time.Time { return time.Time{} }
		case "cancel":
			cancel()
			want, claims = context.Canceled, 0
		}
		provider := dispatchProvider{submit: func(context.Context, Scope, Child) (Outcome, error) {
			if mode != "reply" {
				t.Fatal("ineligible work submitted")
			}
			return Outcome{}, nil
		}}
		if mode == "preflight" {
			provider.validationErr = ErrInvalid
		}
		d.Provider = provider
		if got, err := d.One(ctx); !errors.Is(err, want) || got.Fingerprint() != "" || store.writes != 0 || store.claims != claims {
			t.Fatal("failure returned evidence", err)
		}
	}
}
