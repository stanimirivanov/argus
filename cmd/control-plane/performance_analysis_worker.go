package main

import (
	"context"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/stanimirivanov/argus/internal/performance/adapters/perfeng"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

type analysisRuntimeStore interface {
	perfeng.AnalysisStore
	NextAnalysis(context.Context, requestintent.Scope, string, int64) (requestintent.Pending, error)
}

type analysisTick struct {
	next    func(context.Context, requestintent.Scope, string, int64) (requestintent.Pending, error)
	consume func(context.Context, requestintent.Pending) error
}

// A tick advances past failed work and resets only on end/error, never loops
// around in one tick. Every attempt gets its own deadline, including selection.
func (c performanceAnalysisConfig) analysisStep(ctx context.Context, work analysisTick, after int64, logger *slog.Logger) (int64, time.Duration) {
	for range c.maxPerTick {
		if ctx.Err() != nil {
			return after, c.poll
		}
		call, cancel := context.WithTimeout(ctx, c.timeout)
		pending, err := work.next(call, c.scope, c.requestIssuer, after)
		if err != nil {
			if !errors.Is(err, requestintent.ErrNotFound) && ctx.Err() == nil {
				logger.Warn("performance analysis enumeration unresolved")
			}
			cancel()

			return 0, c.poll
		}
		if pending.After <= after {
			cancel()
			return 0, c.poll
		}
		after = pending.After
		err = work.consume(call, pending)
		cancel()
		if wait, paused := analysisRetry(err, c.poll); paused {
			return after, wait
		}

		if ctx.Err() != nil {
			return after, c.poll
		}
		if err != nil {
			logger.Warn("performance analysis unresolved")
		} else {
			logger.Info("performance analysis retained")
		}
	}

	return after, c.poll
}

func (c performanceAnalysisConfig) startAnalysis(ctx context.Context, store analysisRuntimeStore, root *os.File, consumer perfeng.AnalysisConsumer, logger *slog.Logger) *performanceWorker {
	if !c.enabled {
		return nil
	}
	work := analysisTick{next: store.NextAnalysis, consume: func(ctx context.Context, pending requestintent.Pending) error {
		digest := pending.Child.SHA256
		raw, err := hex.DecodeString(digest)
		if err != nil || len(raw) != 32 || strings.ToLower(digest) != digest {
			return errAnalysisResource
		}
		file, err := analysisFile(root, digest+".json", true, perfeng.NativeReviewMaximumBytes)
		if err != nil {
			return err
		}
		defer func() { closeAnalysisFile(file) }()
		_, err = consumer.Consume(ctx, store, c.scope, pending, file)

		return err
	}}

	return c.analysisLoop(ctx, work, logger)
}

func (c performanceAnalysisConfig) analysisLoop(ctx context.Context, work analysisTick, logger *slog.Logger) *performanceWorker {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		var after int64
		for ctx.Err() == nil {
			next, wait := c.analysisStep(ctx, work, after, logger)
			after = next
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}()

	return &performanceWorker{stop: cancel, done: done}
}

func analysisRetry(err error, poll time.Duration) (time.Duration, bool) {
	var status *perfeng.IntegrationHTTPStatusError
	if !errors.As(err, &status) || (status.Code != http.StatusTooManyRequests && status.Code != http.StatusServiceUnavailable) {
		return poll, false
	}
	if status.RetryAfter > poll {
		return status.RetryAfter, true
	}

	return poll, true
}
