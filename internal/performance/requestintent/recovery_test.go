package requestintent

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/performance"
)

type recoveryStore struct {
	t                        *testing.T
	scope                    Scope
	claim                    Claim
	retained                 Outcome
	readErr, settleErr       error
	commit                   bool
	cancelRead, cancelSettle context.CancelFunc
	reads, writes            int
}

func (s *recoveryStore) RequestOutcome(_ context.Context, scope Scope, position int64) (Outcome, error) {
	s.reads++
	if scope != s.scope || position != s.claim.Pending.After {
		s.t.Fatal("read scope changed")
	}
	if s.cancelRead != nil {
		s.cancelRead()
	}
	if s.retained.Fingerprint() != "" {
		return s.retained, nil
	}

	return Outcome{}, s.readErr
}
func (s *recoveryStore) SettleRequest(_ context.Context, scope Scope, claim Claim, evidence Outcome) (bool, error) {
	s.writes++
	if scope != s.scope || !reflect.DeepEqual(claim, s.claim) {
		s.t.Fatal("settlement changed proof")
	}
	if s.commit {
		s.retained = evidence
	}
	if s.cancelSettle != nil {
		s.cancelSettle()
	}

	return s.settleErr == nil, s.settleErr
}

type recoveryProvider func(context.Context, Scope, Child) (Outcome, error)

func (p recoveryProvider) LookupOutcome(ctx context.Context, scope Scope, child Child) (Outcome, error) {
	return p(ctx, scope, child)
}

func recoveryFixture(t *testing.T) (*recoveryStore, Outcome) {
	t.Helper()
	scope := Scope{"https://provider.example", "https://auth.example", "principal"}
	child := Child{
		Request: performance.Identity{Issuer: "https://argus.example", ID: "request"},
		Attempt: performance.Identity{Issuer: "https://argus.example", ID: "attempt"},
		EventID: "event", Payload: []byte(`{"frozen":true}`),
		RequestedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Reasons:     []Reason{{"mapping", "capability", "reason"}},
	}
	child.Deadline, child.SHA256 = child.RequestedAt.Add(time.Minute), sum(child.Payload)
	claim := Claim{Pending: Pending{After: 42, IntentID: "intent", Child: child}, Token: "00000000-0000-4000-8000-000000000001", Fence: 3, Until: child.Deadline}
	outcome, err := NewOutcome(OutcomeData{Request: child.Request, Attempt: child.Attempt, Source: scope.Issuer, EventID: "reply", Kind: "lifecycle", Payload: []byte(`{"state":"completed"}`)})
	if err != nil {
		t.Fatal(err)
	}

	return &recoveryStore{t: t, scope: scope, claim: claim, readErr: ErrNotFound}, outcome
}

func TestRecoverRetainsLookupAndReadsLostSettlement(t *testing.T) {
	t.Parallel()
	for _, lost := range []bool{false, true} {
		store, expected := recoveryFixture(t)
		store.commit = true
		if lost {
			store.settleErr = ErrUnavailable
		}
		lookups := 0
		provider := recoveryProvider(func(_ context.Context, scope Scope, child Child) (Outcome, error) {
			lookups++
			if scope != store.scope || !reflect.DeepEqual(child, store.claim.Pending.Child) {
				t.Fatal("lookup changed frozen evidence")
			}

			return expected, nil
		})
		for range 2 {
			got, err := Recover(t.Context(), store, provider, store.scope, store.claim)
			if err != nil || got.Fingerprint() != expected.Fingerprint() {
				t.Fatal("recovery failed", err)
			}
		}
		wantReads := 2
		if lost {
			wantReads++
		}
		if store.reads != wantReads || store.writes != 1 || lookups != 1 {
			t.Fatal("hidden retry or failed readback", store.reads, store.writes, lookups)
		}
	}
}

type recoveryFailure struct {
	name                     string
	readErr, lookupErr, want error
	invalid                  bool
	cancelAt                 string
	reads, lookups, writes   int
}

func TestRecoverFailuresNeverReturnEvidence(t *testing.T) {
	t.Parallel()
	for _, tc := range []recoveryFailure{
		{name: "read", readErr: ErrUnavailable, want: ErrUnavailable, reads: 1},
		{name: "lookup absence", lookupErr: ErrNotFound, want: ErrNotFound, reads: 1, lookups: 1},
		{name: "lookup transport", lookupErr: ErrUnavailable, want: ErrUnavailable, reads: 1, lookups: 1},
		{name: "invalid reply", invalid: true, want: ErrInvalid, reads: 1, lookups: 1},
		{name: "stale settlement", want: ErrConflict, reads: 2, lookups: 1, writes: 1},
		{name: "cancel read", cancelAt: "read", want: context.Canceled, reads: 1},
		{name: "cancel lookup", cancelAt: "lookup", want: context.Canceled, reads: 1, lookups: 1},
		{name: "cancel settlement", cancelAt: "settle", want: context.Canceled, reads: 1, lookups: 1, writes: 1},
	} {
		t.Run(tc.name, tc.check)
	}
}

func (tc recoveryFailure) check(t *testing.T) {
	store, evidence := recoveryFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	store.settleErr = ErrConflict
	if tc.readErr != nil {
		store.readErr = tc.readErr
	}
	if tc.cancelAt == "read" {
		store.cancelRead = cancel
	}
	if tc.cancelAt == "settle" {
		store.cancelSettle, store.commit = cancel, true
	}
	if tc.invalid {
		evidence = Outcome{}
	}
	lookups := 0
	provider := recoveryProvider(func(context.Context, Scope, Child) (Outcome, error) {
		lookups++
		if tc.cancelAt == "lookup" {
			cancel()
		}

		return evidence, tc.lookupErr
	})
	got, err := Recover(ctx, store, provider, store.scope, store.claim)
	if !errors.Is(err, tc.want) || got.Fingerprint() != "" {
		t.Fatal("failure returned success", err)
	}
	if store.reads != tc.reads || lookups != tc.lookups || store.writes != tc.writes {
		t.Fatal("unexpected side effects", store.reads, lookups, store.writes)
	}
}

func TestRecoverRejectsMismatchedEvidence(t *testing.T) {
	t.Parallel()
	for _, fromStore := range []bool{false, true} {
		for _, field := range []string{"source", "request", "attempt"} {
			testRecoveryMismatch(t, fromStore, field)
		}
	}
}

func testRecoveryMismatch(t *testing.T, fromStore bool, field string) {
	t.Helper()
	store, evidence := recoveryFixture(t)
	d := evidence.Data()
	switch field {
	case "source":
		d.Source += "-other"
	case "request":
		d.Request.ID += "-other"
	case "attempt":
		d.Attempt.ID += "-other"
	}
	wrong, err := NewOutcome(d)
	if err != nil {
		t.Fatal(err)
	}
	if fromStore {
		store.retained = wrong
	}
	provider := recoveryProvider(func(context.Context, Scope, Child) (Outcome, error) {
		if fromStore {
			t.Fatal("retained outcome contacted provider")
		}
		return wrong, nil
	})
	if got, err := Recover(t.Context(), store, provider, store.scope, store.claim); !errors.Is(err, ErrInvalid) || got.Fingerprint() != "" || store.writes != 0 {
		t.Fatal("mismatched evidence accepted", err)
	}
}

func TestRecoverRejectsMalformedClaimsAndCancellationBeforeIO(t *testing.T) {
	t.Parallel()
	for _, change := range []func(*Scope, *Claim){
		func(s *Scope, _ *Claim) { s.PrincipalID = "" },
		func(_ *Scope, c *Claim) { c.Pending.After = 0 },
		func(_ *Scope, c *Claim) { c.Fence = 0 },
		func(_ *Scope, c *Claim) { c.Token = "" },
		func(_ *Scope, c *Claim) { c.Pending.Child.Payload = nil },
	} {
		store, _ := recoveryFixture(t)
		scope, claim := store.scope, store.claim
		change(&scope, &claim)
		if _, err := Recover(t.Context(), store, recoveryProvider(nil), scope, claim); !errors.Is(err, ErrInvalid) || store.reads != 0 {
			t.Fatal(err)
		}
	}
	store, _ := recoveryFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Recover(ctx, store, recoveryProvider(nil), store.scope, store.claim); !errors.Is(err, context.Canceled) || store.reads != 0 {
		t.Fatal(err)
	}
}
