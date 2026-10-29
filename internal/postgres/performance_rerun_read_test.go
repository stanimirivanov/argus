package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

func TestRerunReadsRejectInvalidAndCancelledInputsBeforeStorage(t *testing.T) {
	store := &PerformanceStore{}
	scope := requestintent.Scope{Issuer: "provider", PrincipalIssuer: "auth", PrincipalID: "owner"}
	for _, read := range []func(context.Context, requestintent.Scope, int64) (requestintent.RerunRecord, error){store.RerunSuccessor, store.RerunPredecessor} {
		for _, position := range []int64{0, -1} {
			if got, err := read(t.Context(), scope, position); !errors.Is(err, requestintent.ErrInvalid) || got.Evidence.Fingerprint() != "" {
				t.Fatal("invalid cursor opened storage", err)
			}
		}
		if _, err := read(t.Context(), requestintent.Scope{}, 1); !errors.Is(err, requestintent.ErrInvalid) {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if _, err := read(ctx, scope, 1); !errors.Is(err, context.Canceled) {
			t.Fatal("cancelled read opened storage", err)
		}
	}
}
