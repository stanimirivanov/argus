package requestintent

import (
	"context"
	"errors"
	"strings"
)

// RecoveryStore owns scoped readback and atomic fenced settlement. Implementations
// must verify live ownership at commit and return redacted storage errors.
type RecoveryStore interface {
	RequestOutcome(context.Context, Scope, int64) (Outcome, error)
	SettleRequest(context.Context, Scope, Claim, Outcome) (bool, error)
}

// RecoveryProvider authenticates its independently configured provider/principal,
// validates the frozen child's complete wire context and returns owned evidence.
// LookupOutcome must perform only bounded readback, without submissions or retries.
type RecoveryProvider interface {
	LookupOutcome(context.Context, Scope, Child) (Outcome, error)
}

// Recover resolves uncertain admission for a store-issued claim in an independently
// authorized scope. Callers must not mutate the claim during the call. Expired
// request deadlines permit readback, never submission. A lease is not approval.
// Network I/O occurs between storage calls, outside transactions. No identity,
// retry schedule or lease is created here; unresolved errors leave work intact.
// A lost settlement response gets one scoped readback, not another provider call.
func Recover(ctx context.Context, store RecoveryStore, provider RecoveryProvider, scope Scope, claim Claim) (Outcome, error) {
	if !ValidScope(scope) || claim.Pending.After <= 0 || claim.Fence <= 0 || strings.TrimSpace(claim.Token) == "" || len(claim.Token) > 64 || !claim.Pending.Child.Valid() || store == nil || provider == nil {
		return Outcome{}, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return Outcome{}, err
	}
	retained, err := store.RequestOutcome(ctx, scope, claim.Pending.After)
	if ctx.Err() != nil {
		return Outcome{}, ctx.Err()
	}
	if err == nil {
		return recoveryEvidence(retained, scope, claim.Pending.Child)
	}
	if !errors.Is(err, ErrNotFound) {
		return Outcome{}, err
	}
	outcome, err := provider.LookupOutcome(ctx, scope, claim.Pending.Child)
	if ctx.Err() != nil {
		return Outcome{}, ctx.Err()
	}
	if err != nil {
		return Outcome{}, err
	}
	if _, err := recoveryEvidence(outcome, scope, claim.Pending.Child); err != nil {
		return Outcome{}, err
	}
	_, err = store.SettleRequest(ctx, scope, claim, outcome)
	if ctx.Err() != nil {
		return Outcome{}, ctx.Err()
	}
	if err == nil {
		return outcome, nil
	}
	// Commit acknowledgement can be lost, or a newer owner may have settled.
	// Read authoritative scoped evidence once; never treat an error as success.
	retained, readErr := store.RequestOutcome(ctx, scope, claim.Pending.After)
	if ctx.Err() != nil {
		return Outcome{}, ctx.Err()
	}
	if readErr != nil {
		return Outcome{}, err
	}

	return recoveryEvidence(retained, scope, claim.Pending.Child)
}

func recoveryEvidence(outcome Outcome, scope Scope, child Child) (Outcome, error) {
	d := outcome.Data()
	if outcome.Fingerprint() == "" || d.Source != scope.Issuer || d.Request != child.Request || d.Attempt != child.Attempt {
		return Outcome{}, ErrInvalid
	}

	return outcome, nil
}
