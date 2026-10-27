package requestintent

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWorkerWaitsAfterUncertaintyAndRecoversWithoutPost(t *testing.T) {
	t.Parallel()
	d, store, _ := dispatchFixture(t)
	sends := 0
	d.Provider = dispatchProvider{
		submit:           func(context.Context, Scope, Child) (Outcome, error) { sends++; return Outcome{}, ErrUnavailable },
		recoveryProvider: func(context.Context, Scope, Child) (Outcome, error) { return Outcome{}, ErrNotFound },
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	ticks, observed, done := make(chan time.Time), make(chan error, 1), make(chan error, 1)
	go func() { done <- d.Run(ctx, ticks, func(_ Outcome, err error) { observed <- err }); close(done) }()
	defer func() { cancel(); <-done }()
	select {
	case err := <-observed:
		if !errors.Is(err, ErrUnavailable) || store.claims != 1 {
			t.Fatal("first attempt", err)
		}
	case <-ctx.Done():
		t.Fatal("worker did not run")
	}
	store.claim.Fence++ // No further access can occur until the explicit tick.
	select {
	case ticks <- time.Now():
	case <-ctx.Done():
		t.Fatal("worker did not wait for tick")
	}
	select {
	case err := <-observed:
		if !errors.Is(err, ErrNotFound) || store.claims != 2 || sends != 1 {
			t.Fatal("worker replayed POST", err)
		}
	case <-ctx.Done():
		t.Fatal("worker did not recover")
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := d.Run(ctx, ticks, func(Outcome, error) { t.Error("canceled worker observed work") }); !errors.Is(err, context.Canceled) || store.claims != 2 {
		t.Fatal(err)
	}
}

func TestWorkerRejectsMissingOrClosedScheduler(t *testing.T) {
	d, _, _ := dispatchFixture(t)
	if err := d.Run(t.Context(), nil, func(Outcome, error) {}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	ticks := make(chan time.Time)
	if err := d.Run(t.Context(), ticks, nil); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	d.Provider = dispatchProvider{validationErr: ErrInvalid}
	if err := d.Run(t.Context(), ticks, func(Outcome, error) { close(ticks) }); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}
