//go:build integration

package postgres

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

func issuerClaimFixture(t *testing.T) (*integratedStore, requestintent.Data, requestintent.Data) {
	t.Helper()
	store, original := claimFixture(t, 0)
	other := requestFixture(t, store, original.CatalogSHA256, "other-request-issuer")
	for i := range other.Children {
		other.Children[i].Request.Issuer = "https://other-argus.example"
		other.Children[i].Attempt.Issuer = "https://other-argus.example"
	}
	// Inward storage allows distinct issuer identities. The filtered API must
	// leave this mixed pair untouched instead of consuming first-send eligibility.
	other.Children[0].Attempt.Issuer = "https://different-attempt.example"
	if _, err := store.runtime.Performance().CreateRequest(t.Context(), inwardRequest(t, other)); err != nil {
		t.Fatal(err)
	}

	return store, original, other
}

func TestPerformanceIssuerClaimsPreserveOtherWorkAndFences(t *testing.T) {
	store, original, other := issuerClaimFixture(t)
	capability := store.runtime.Performance()
	issuer := other.Children[1].Request.Issuer
	if _, err := capability.ClaimRequestForIssuer(t.Context(), original.Scope, "https://absent.example", time.Minute); !errors.Is(err, requestintent.ErrNotFound) {
		t.Fatal("missing issuer acquired work", err)
	}
	wrongScope := original.Scope
	wrongScope.PrincipalID = "other-principal"
	if _, err := capability.ClaimRequestForIssuer(t.Context(), wrongScope, issuer, time.Minute); !errors.Is(err, requestintent.ErrNotFound) {
		t.Fatal("principal scope escaped", err)
	}
	claim, err := capability.ClaimRequestForIssuer(t.Context(), original.Scope, issuer, time.Minute)
	if err != nil || claim.Fence != 1 || claim.Pending.Child.Request != other.Children[1].Request || claim.Pending.Child.Attempt.Issuer != issuer {
		t.Fatal("issuer selection changed evidence or consumed wrong work", err)
	}
	legacy, err := capability.ClaimRequest(t.Context(), original.Scope, time.Minute)
	if err != nil || legacy.Fence != 1 || legacy.Pending.Child.Request.Issuer != original.Children[0].Request.Issuer {
		t.Fatal("other issuer lost first acquisition or legacy behavior changed", err)
	}
	if err := capability.ReleaseRequest(t.Context(), original.Scope, claim, 0); err != nil {
		t.Fatal(err)
	}
	reclaimed, err := capability.ClaimRequestForIssuer(t.Context(), original.Scope, issuer, time.Minute)
	if err != nil || reclaimed.Pending.After != claim.Pending.After || reclaimed.Fence != 2 || reclaimed.Token == claim.Token {
		t.Fatal("reacquisition lost fencing", err)
	}
	if err := capability.ReleaseRequest(t.Context(), original.Scope, claim, 0); !errors.Is(err, requestintent.ErrConflict) {
		t.Fatal("stale owner released filtered claim", err)
	}
}

func TestPerformanceIssuerClaimsRemainDisjointUnderConcurrency(t *testing.T) {
	store, original, other := issuerClaimFixture(t)
	capability := store.runtime.Performance()
	claims := make(chan requestintent.Claim, 4)
	var workers sync.WaitGroup
	for _, issuer := range []string{original.Children[0].Request.Issuer, other.Children[1].Request.Issuer} {
		for range 2 {
			workers.Go(func() {
				claim, err := capability.ClaimRequestForIssuer(t.Context(), original.Scope, issuer, time.Minute)
				if err == nil {
					claims <- claim
				} else if !errors.Is(err, requestintent.ErrNotFound) {
					t.Error(err)
				}
			})
		}
	}
	workers.Wait()
	close(claims)
	seen := map[int64]bool{}
	for claim := range claims {
		if seen[claim.Pending.After] || claim.Fence != 1 || claim.Pending.Child.Request.Issuer != claim.Pending.Child.Attempt.Issuer {
			t.Fatal("duplicate or mixed-issuer acquisition")
		}
		seen[claim.Pending.After] = true
	}
	if len(seen) != 3 {
		t.Fatal("eligible work lost", len(seen))
	}
	legacy, err := capability.ClaimRequest(t.Context(), original.Scope, time.Minute)
	if err != nil || legacy.Fence != 1 || legacy.Pending.Child.Request != other.Children[0].Request {
		t.Fatal("mixed issuer row was consumed or legacy API became filtered", err)
	}
}

func TestPerformanceIssuerClaimValidationPrecedesStorage(t *testing.T) {
	var store *PerformanceStore
	scope := requestintent.Scope{Issuer: "provider", PrincipalIssuer: "auth", PrincipalID: "principal"}
	for _, issuer := range []string{"", " ", "issuer\x00", string([]byte{0xff}), strings.Repeat("\U0001f600", 513)} {
		if _, err := store.ClaimRequestForIssuer(t.Context(), scope, issuer, time.Minute); !errors.Is(err, requestintent.ErrInvalid) {
			t.Fatal("malformed issuer reached storage", err)
		}
	}
	for _, lease := range []time.Duration{0, time.Second + time.Nanosecond, 6 * time.Minute} {
		if _, err := store.ClaimRequestForIssuer(t.Context(), scope, "issuer", lease); !errors.Is(err, requestintent.ErrInvalid) {
			t.Fatal("invalid lease reached storage", err)
		}
	}
	if _, err := store.ClaimRequestForIssuer(t.Context(), requestintent.Scope{}, "issuer", time.Minute); !errors.Is(err, requestintent.ErrInvalid) {
		t.Fatal("invalid scope reached storage", err)
	}
}
