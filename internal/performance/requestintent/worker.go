package requestintent

import (
	"context"
	"time"
)

// Run serially invokes One immediately and once per scheduler tick, including
// after idle/error results. The caller owns the bounded polling policy and must
// provide a nonblocking observer. Cancellation stops acquisition and active I/O;
// the caller must join Run before closing its ports. No result authorizes replay.
func (d Dispatcher) Run(ctx context.Context, ticks <-chan time.Time, observe func(Outcome, error)) error {
	if ticks == nil || observe == nil {
		return ErrInvalid
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		outcome, err := d.One(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		observe(outcome, err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case _, open := <-ticks:
			if !open {
				return ErrInvalid
			}
		}
	}
}
