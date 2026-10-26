//go:build integration

package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

type dispatchEvidence struct {
	t              *testing.T
	sends, lookups int
}

func (p *dispatchEvidence) ValidateDispatch(requestintent.Scope) error { return nil }

func (p *dispatchEvidence) SubmitOutcome(_ context.Context, scope requestintent.Scope, child requestintent.Child) (requestintent.Outcome, error) {
	p.sends++
	return outcomeFixture(p.t, scope, requestintent.Claim{Pending: requestintent.Pending{Child: child}}, "accepted"), nil
}
func (p *dispatchEvidence) LookupOutcome(_ context.Context, scope requestintent.Scope, child requestintent.Child) (requestintent.Outcome, error) {
	p.lookups++
	return outcomeFixture(p.t, scope, requestintent.Claim{Pending: requestintent.Pending{Child: child}}, "lifecycle"), nil
}

func TestPerformanceDispatcherRecoversClaimThenSubmitsUntouchedChild(t *testing.T) {
	store, data := claimFixture(t, 0)
	capability := store.runtime.Performance()
	claim, err := capability.ClaimRequest(t.Context(), data.Scope, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	// The first owner never sent. Release models expiry without a timing sleep;
	// the existing claim suite separately verifies database-clock expiry/fencing.
	if err := capability.ReleaseRequest(t.Context(), data.Scope, claim, 0); err != nil {
		t.Fatal(err)
	}
	provider := &dispatchEvidence{t: t}
	dispatcher := requestintent.Dispatcher{Store: capability, Provider: provider, Scope: data.Scope, RequestIssuer: data.Children[0].Request.Issuer, Lease: 5 * time.Minute, Now: time.Now}
	recovered, err := dispatcher.One(t.Context())
	if err != nil || recovered.Data().Kind != "lifecycle" || provider.sends != 0 || provider.lookups != 1 {
		t.Fatal("reacquisition submitted instead of recovering", err)
	}
	retained, err := capability.RequestOutcome(t.Context(), data.Scope, claim.Pending.After)
	if err != nil || retained.Fingerprint() != recovered.Fingerprint() {
		t.Fatal("recovery was not durable", err)
	}
	submitted, err := dispatcher.One(t.Context())
	if err != nil || submitted.Data().Kind != "accepted" || provider.sends != 1 || provider.lookups != 1 {
		t.Fatal("untouched child did not submit exactly once", err)
	}
	if got, err := dispatcher.One(t.Context()); !errors.Is(err, requestintent.ErrNotFound) || got.Fingerprint() != "" || provider.sends != 1 {
		t.Fatal("settled work was repeated", err)
	}
}
