package postgres

import (
	"errors"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

func TestCancellationScheduleInvalidInputNeverOpensStorage(t *testing.T) {
	store := &PerformanceStore{}
	scope := requestintent.Scope{Issuer: "provider", PrincipalIssuer: "auth", PrincipalID: "owner"}
	for _, delay := range []time.Duration{-time.Second, 0, time.Millisecond, 1500 * time.Millisecond} {
		if _, err := store.DeferCancellation(t.Context(), scope, "requester", 1, delay); !errors.Is(err, requestintent.ErrInvalid) {
			t.Fatal("invalid delay reached storage", err)
		}
	}
	if _, err := store.DeferCancellation(t.Context(), scope, "", 1, time.Second); !errors.Is(err, requestintent.ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := store.DeferCancellation(t.Context(), scope, "requester", 0, time.Second); !errors.Is(err, requestintent.ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := store.NextCancellation(t.Context(), scope, "requester", -1); !errors.Is(err, requestintent.ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := store.NextCancellation(t.Context(), requestintent.Scope{}, "requester", 0); !errors.Is(err, requestintent.ErrInvalid) {
		t.Fatal(err)
	}
}
