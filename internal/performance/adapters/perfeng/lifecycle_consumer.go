package perfeng

import (
	"context"

	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

// LifecycleStore retains and revalidates scoped evidence without provider I/O.
// Implementations must enforce authoritative admission/run binding atomically.
type LifecycleStore interface {
	RequestOutcome(context.Context, requestintent.Scope, int64) (requestintent.Outcome, error)
	LifecycleHistory(context.Context, requestintent.Scope, int64, requestintent.LifecycleAdmission, requestintent.LifecycleVerifier) ([]requestintent.LifecycleEvent, error)
	AppendLifecycle(context.Context, requestintent.Scope, int64, requestintent.LifecycleAdmission, requestintent.LifecycleEvent, requestintent.LifecycleVerifier) (bool, error)
}

// ConsumeLifecycle validates retained history then performs at most one authenticated
// GET and atomic append. Trusted configuration and frozen work must remain immutable
// during calls. Terminal history skips network I/O; late evidence remains eligible.
// Lost append acknowledgement gets one exact scoped readback, never a POST or retry.
func (source IntegrationHTTP) ConsumeLifecycle(ctx context.Context, store LifecycleStore, scope requestintent.Scope, pending requestintent.Pending) (requestintent.LifecycleEvent, error) {
	if store == nil || pending.After <= 0 || source.ValidateDispatch(scope) != nil || !source.validDispatchChild(scope, pending.Child) {
		return requestintent.LifecycleEvent{}, ErrIntegration
	}
	if err := ctx.Err(); err != nil {
		return requestintent.LifecycleEvent{}, err
	}
	retained, err := store.RequestOutcome(ctx, scope, pending.After)
	if err != nil {
		return requestintent.LifecycleEvent{}, err
	}
	verify := func(outcome requestintent.Outcome) (requestintent.LifecycleAdmission, error) {
		return BindLifecycleAdmission(pending.Child.Payload, outcome, source.Provider)
	}
	admission, err := verify(retained)
	if err != nil {
		return requestintent.LifecycleEvent{}, err
	}
	history, err := store.LifecycleHistory(ctx, scope, pending.After, admission, verify)
	if err != nil {
		return requestintent.LifecycleEvent{}, err
	}
	latest := admission.Data().Initial
	if len(history) > 0 {
		latest, err = requestintent.ProjectLifecycle(history)
		if err != nil {
			return requestintent.LifecycleEvent{}, err
		}
	}
	if lifecycleTerminal(latest) && len(history) > 0 {
		if err := ctx.Err(); err != nil {
			return requestintent.LifecycleEvent{}, err
		}

		return latest, nil
	}
	if !lifecycleTerminal(latest) {
		reply, lookupErr := source.Lookup(ctx, pending.Child.Payload)
		if lookupErr != nil {
			return requestintent.LifecycleEvent{}, lookupErr
		}
		latest, err = LifecycleObservation(pending.Child.Payload, reply, source.Provider)
		if err != nil || latest.Data().Run != admission.Data().Run {
			return requestintent.LifecycleEvent{}, ErrIntegration
		}
	}
	if err := ctx.Err(); err != nil {
		return requestintent.LifecycleEvent{}, err
	}

	return retainLifecycle(ctx, store, scope, pending.After, admission, latest, verify)
}

func retainLifecycle(ctx context.Context, store LifecycleStore, scope requestintent.Scope, position int64, admission requestintent.LifecycleAdmission, event requestintent.LifecycleEvent, verify requestintent.LifecycleVerifier) (requestintent.LifecycleEvent, error) {
	_, err := store.AppendLifecycle(ctx, scope, position, admission, event, verify)
	if ctx.Err() != nil {
		return requestintent.LifecycleEvent{}, ctx.Err()
	}
	if err != nil {
		// A later revision is not evidence that this particular append committed.
		history, readErr := store.LifecycleHistory(ctx, scope, position, admission, verify)
		if ctx.Err() != nil {
			return requestintent.LifecycleEvent{}, ctx.Err()
		}
		if readErr == nil {
			for _, retained := range history {
				if retained.Equal(event) {
					return event, nil
				}
			}
		}

		return requestintent.LifecycleEvent{}, err
	}

	return event, nil
}

func lifecycleTerminal(event requestintent.LifecycleEvent) bool {
	switch event.Data().State {
	case requestintent.LifecycleCompleted, requestintent.LifecycleFailed, requestintent.LifecycleCancelled:
		return true
	default:
		return false
	}
}
