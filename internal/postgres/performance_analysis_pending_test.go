package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

func TestNextAnalysisRejectsBeforeOpeningStorage(t *testing.T) {
	store := &PerformanceStore{}
	scope := requestintent.Scope{Issuer: "provider", PrincipalIssuer: "auth", PrincipalID: "owner"}
	for _, input := range []struct {
		scope  requestintent.Scope
		issuer string
		after  int64
	}{
		{requestintent.Scope{}, "requester", 0}, {scope, "", 0}, {scope, "requester", -1},
	} {
		pending, err := store.NextAnalysis(t.Context(), input.scope, input.issuer, input.after)
		if !errors.Is(err, requestintent.ErrInvalid) || pending.After != 0 || pending.Child.Payload != nil {
			t.Fatal("invalid traversal reached storage", err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	pending, err := store.NextAnalysis(ctx, scope, "requester", 0)
	if !errors.Is(err, context.Canceled) || pending.After != 0 {
		t.Fatal("cancelled traversal reached storage", err)
	}
}
