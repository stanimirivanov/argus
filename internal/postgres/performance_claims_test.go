package postgres

import (
	"errors"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

func TestPerformanceClaimValidationPrecedesStorage(t *testing.T) {
	t.Parallel()
	store := &PerformanceStore{}
	scope := requestintent.Scope{Issuer: "provider", PrincipalIssuer: "authority", PrincipalID: "principal"}
	for _, lease := range []time.Duration{0, time.Second - 1, 5*time.Minute + 1} {
		if _, err := store.ClaimRequest(t.Context(), scope, lease); !errors.Is(err, requestintent.ErrInvalid) {
			t.Fatal(err)
		}
	}
	if _, err := store.ClaimRequest(t.Context(), requestintent.Scope{}, time.Minute); !errors.Is(err, requestintent.ErrInvalid) {
		t.Fatal(err)
	}
	if err := store.ReleaseRequest(t.Context(), scope, requestintent.Claim{}, 0); !errors.Is(err, requestintent.ErrInvalid) {
		t.Fatal(err)
	}
	claim := requestintent.Claim{Pending: requestintent.Pending{After: 1}, Fence: 1, Token: "00000000-0000-0000-0000-000000000001"}
	for _, delay := range []time.Duration{-1, 1, time.Hour + 1} {
		if err := store.ReleaseRequest(t.Context(), scope, claim, delay); !errors.Is(err, requestintent.ErrInvalid) {
			t.Fatal(err)
		}
	}
}
