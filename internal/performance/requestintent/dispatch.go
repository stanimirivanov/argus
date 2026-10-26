package requestintent

import (
	"context"
	"strings"
	"time"
)

// DispatchStore acquires fresh scoped ownership and retains fenced outcomes.
// Acquisition must use database time and monotonic fences; errors grant no claim.
type DispatchStore interface {
	RecoveryStore
	ClaimRequestForIssuer(context.Context, Scope, string, time.Duration) (Claim, error)
}

// DispatchProvider validates/authenticates frozen child evidence at the adapter.
// SubmitOutcome sends exact bytes once; any failure can leave admission uncertain.
type DispatchProvider interface {
	RecoveryProvider
	// ValidateDispatch checks trusted configuration without I/O or side effects.
	ValidateDispatch(Scope) error
	SubmitOutcome(context.Context, Scope, Child) (Outcome, error)
}

// Dispatcher is an opt-in embedding boundary, not a background worker. Configuration
// must be independently authorized and immutable during calls; Now is a trusted clock.
// Concurrent use requires concurrency-safe ports/clock. No claim grants approval:
// storage must contain only previously validated, explicitly approved frozen work.
type Dispatcher struct {
	Store         DispatchStore
	Provider      DispatchProvider
	Scope         Scope
	RequestIssuer string
	Lease         time.Duration
	Now           func() time.Time
}

// One acquires at most one child. Only the first acquisition may submit, while both
// request and lease are live. Reacquisitions and expired work use lookup recovery.
// A crash before sending can leave absent work unresolved; never infer permission
// to POST from absence. There is no automatic retry, release or identity generation.
// Context bounds reduce lease/deadline overrun; database fencing and provider
// admission remain authoritative. Network I/O stays outside storage transactions.
func (d Dispatcher) One(ctx context.Context) (Outcome, error) {
	if !ValidScope(d.Scope) || !ValidRequestIssuer(d.RequestIssuer) || d.Lease < time.Second || d.Lease > 5*time.Minute || d.Lease%time.Microsecond != 0 || d.Store == nil || d.Provider == nil || d.Now == nil {
		return Outcome{}, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return Outcome{}, err
	}
	if err := d.Provider.ValidateDispatch(d.Scope); err != nil {
		return Outcome{}, err
	}
	claim, err := d.Store.ClaimRequestForIssuer(ctx, d.Scope, d.RequestIssuer, d.Lease)
	if ctx.Err() != nil {
		return Outcome{}, ctx.Err()
	}
	if err != nil {
		return Outcome{}, err
	}
	c := claim.Pending.Child
	if !c.Valid() || c.Request.Issuer != d.RequestIssuer || c.Attempt.Issuer != d.RequestIssuer || claim.Pending.After <= 0 || claim.Fence <= 0 || strings.TrimSpace(claim.Token) == "" || len(claim.Token) > 64 || claim.Until.IsZero() {
		return Outcome{}, ErrInvalid
	}
	now := d.Now()
	if now.IsZero() || now.Before(c.RequestedAt) {
		return Outcome{}, ErrInvalid
	}
	if claim.Fence != 1 || !now.Before(c.Deadline) || !now.Before(claim.Until) {
		return Recover(ctx, d.Store, d.Provider, d.Scope, claim)
	}
	sendCtx, cancel := context.WithTimeout(ctx, min(c.Deadline.Sub(now), claim.Until.Sub(now)))
	defer cancel()
	outcome, err := d.Provider.SubmitOutcome(sendCtx, d.Scope, c)
	if ctx.Err() != nil {
		return Outcome{}, ctx.Err()
	}
	if err != nil {
		return Outcome{}, err
	}
	kind := outcome.Data().Kind
	if _, err := recoveryEvidence(outcome, d.Scope, c); err != nil || (kind != "accepted" && kind != "rejected") {
		return Outcome{}, ErrInvalid
	}

	return settleOutcome(ctx, d.Store, d.Scope, claim, outcome)
}
