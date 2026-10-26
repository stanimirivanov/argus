package postgres

import (
	"errors"
	"testing"

	"github.com/stanimirivanov/argus/internal/performance"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

func TestPerformanceOutcomeValidationPrecedesStorage(t *testing.T) {
	t.Parallel()
	store := &PerformanceStore{}
	if _, err := store.SettleRequest(t.Context(), requestintent.Scope{}, requestintent.Claim{}, requestintent.Outcome{}); !errors.Is(err, requestintent.ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := store.RequestOutcome(t.Context(), requestintent.Scope{}, 1); !errors.Is(err, requestintent.ErrInvalid) {
		t.Fatal(err)
	}
	scope := requestintent.Scope{Issuer: "provider", PrincipalIssuer: "authority", PrincipalID: "operator"}
	outcome, err := requestintent.NewOutcome(requestintent.OutcomeData{Request: performance.Identity{Issuer: "argus", ID: "request"}, Attempt: performance.Identity{Issuer: "argus", ID: "attempt"}, Source: scope.Issuer, EventID: "event", Kind: "accepted", Payload: []byte(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	for _, claim := range []requestintent.Claim{
		{Pending: requestintent.Pending{After: 1}, Fence: 1, Token: "invalid"},
		{Pending: requestintent.Pending{After: 1}, Fence: 1, Token: "00000000-0000-0000-0000-000000000000"},
		{Pending: requestintent.Pending{After: 0}, Fence: 1, Token: "00000000-0000-0000-0000-000000000001"},
		{Pending: requestintent.Pending{After: 1}, Fence: 0, Token: "00000000-0000-0000-0000-000000000001"},
	} {
		if created, err := store.SettleRequest(t.Context(), scope, claim, outcome); created || !errors.Is(err, requestintent.ErrInvalid) {
			t.Fatal("invalid ownership reached storage", err)
		}
	}
	if _, err := store.RequestOutcome(t.Context(), scope, 0); !errors.Is(err, requestintent.ErrInvalid) {
		t.Fatal(err)
	}
}
