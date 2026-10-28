package perfeng

import (
	"context"
	"slices"

	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

// CancellationStore revalidates authoritative scoped intent, admission and history.
// Implementations enforce atomic run/revision binding without provider I/O.
type CancellationStore interface {
	LifecycleStore
	GetCancellation(context.Context, requestintent.Scope, int64, requestintent.CancellationVerifier) (requestintent.Cancellation, error)
}

// ConsumeCancellation reads durable intent and admission, then performs at most one
// authenticated GET and one cancellation POST, outside storage transactions. A GET
// reconciles uncertain earlier delivery; terminal history skips I/O. Missing or
// rejected admission grants no provider access. Callers independently authorize
// configuration/request issuer and keep configuration/frozen work immutable.
// Returns the retained observation, not a delivery receipt or latest-state promise.
// There are no retries, original submissions, new identities or worker activation.
func (source IntegrationHTTP) ConsumeCancellation(ctx context.Context, store CancellationStore, scope requestintent.Scope, pending requestintent.Pending) (requestintent.LifecycleEvent, error) {
	if store == nil || pending.After <= 0 || source.ValidateDispatch(scope) != nil || !source.validDispatchChild(scope, pending.Child) {
		return requestintent.LifecycleEvent{}, ErrIntegration
	}
	if err := ctx.Err(); err != nil {
		return requestintent.LifecycleEvent{}, err
	}
	intent, err := store.GetCancellation(ctx, scope, pending.After, BindCancellation)
	if err != nil {
		return requestintent.LifecycleEvent{}, err
	}
	checked, err := BindCancellation(pending.Child.Payload, intent.Data().Payload)
	if err != nil || checked.Fingerprint() != intent.Fingerprint() {
		return requestintent.LifecycleEvent{}, ErrIntegration
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
	if err := ctx.Err(); err != nil {
		return requestintent.LifecycleEvent{}, err
	}
	if lifecycleTerminal(latest) && len(history) > 0 {
		return latest, nil
	}
	if !lifecycleTerminal(latest) {
		latest, err = source.cancellationObservation(ctx, pending, admission, history, intent)
		if err != nil {
			return requestintent.LifecycleEvent{}, err
		}
	}
	if err := ctx.Err(); err != nil {
		return requestintent.LifecycleEvent{}, err
	}

	return retainLifecycle(ctx, store, scope, pending.After, admission, latest, verify)
}

func (source IntegrationHTTP) cancellationObservation(ctx context.Context, pending requestintent.Pending, admission requestintent.LifecycleAdmission, history []requestintent.LifecycleEvent, intent requestintent.Cancellation) (requestintent.LifecycleEvent, error) {
	reply, err := source.Lookup(ctx, pending.Child.Payload)
	if err != nil {
		return requestintent.LifecycleEvent{}, err
	}
	observed, err := LifecycleObservation(pending.Child.Payload, reply, source.Provider)
	if err != nil || observed.Data().Run != admission.Data().Run {
		return requestintent.LifecycleEvent{}, ErrIntegration
	}
	evidence := cancellationEvidence(history, admission.Data().Initial, observed)
	latest, err := requestintent.ProjectLifecycle(evidence)
	if err != nil {
		return requestintent.LifecycleEvent{}, err
	}
	if lifecycleTerminal(latest) || latest.Data().State == requestintent.LifecycleCancelling {
		return observed, nil
	}
	reply, err = source.Cancel(ctx, pending.Child.Payload, intent.Data().Payload)
	if err != nil {
		return requestintent.LifecycleEvent{}, err
	}
	result, err := LifecycleObservation(pending.Child.Payload, reply, source.Provider)
	if err != nil || result.Data().Run != admission.Data().Run {
		return requestintent.LifecycleEvent{}, ErrIntegration
	}
	if _, err := requestintent.ProjectLifecycle(cancellationEvidence(evidence, requestintent.LifecycleEvent{}, result)); err != nil {
		return requestintent.LifecycleEvent{}, err
	}

	return result, nil
}

func cancellationEvidence(history []requestintent.LifecycleEvent, initial, event requestintent.LifecycleEvent) []requestintent.LifecycleEvent {
	evidence := slices.Clone(history)
	if len(evidence) == 0 && initial.Data().Evidence.Fingerprint() != "" {
		evidence = append(evidence, initial)
	}
	if !slices.ContainsFunc(evidence, event.Equal) {
		evidence = append(evidence, event)
	}

	return evidence
}
