package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/performance/adapters/perfeng"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

func TestAnalysisTicksBoundWorkAdvanceFailuresAndReset(t *testing.T) {
	c := performanceAnalysisConfig{maxPerTick: 2, timeout: time.Second, poll: time.Second, requestIssuer: "issuer", scope: requestintent.Scope{Issuer: "provider"}}
	var attempts []int64
	work := analysisTick{next: func(ctx context.Context, scope requestintent.Scope, issuer string, after int64) (requestintent.Pending, error) {
		if _, ok := ctx.Deadline(); !ok || scope != c.scope || issuer != c.requestIssuer {
			t.Error("lost scope/deadline")
		}
		if after >= 3 {
			return requestintent.Pending{}, requestintent.ErrNotFound
		}

		return requestintent.Pending{After: after + 1}, nil
	}, consume: func(_ context.Context, p requestintent.Pending) error {
		attempts = append(attempts, p.After)
		return errors.New("private failure")
	}}
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	after, _ := c.analysisStep(t.Context(), work, 0, logger)
	if after != 2 || !reflect.DeepEqual(attempts, []int64{1, 2}) {
		t.Fatal("tick budget/fairness lost", after, attempts)
	}
	after, _ = c.analysisStep(t.Context(), work, after, logger)
	if after != 0 || !reflect.DeepEqual(attempts, []int64{1, 2, 3}) {
		t.Fatal("end did not reset", after, attempts)
	}
	after, _ = c.analysisStep(t.Context(), work, after, logger)
	if after != 2 || len(attempts) != 5 {
		t.Fatal("reset skipped work")
	}
	work.consume = func(context.Context, requestintent.Pending) error {
		return &perfeng.IntegrationHTTPStatusError{Code: 429, RetryAfter: time.Hour}
	}
	after, wait := c.analysisStep(t.Context(), work, 0, logger)
	if strings.Contains(logs.String(), "private") {
		t.Fatal("private error leaked")
	}
	if after != 1 || wait != time.Hour {
		t.Fatal("provider pacing ignored", after, wait)
	}
}

func TestAnalysisShutdownCancelsAndJoins(t *testing.T) {
	c := performanceAnalysisConfig{maxPerTick: 1, timeout: time.Minute, poll: time.Hour}
	started := make(chan struct{})
	finished := make(chan struct{})
	work := analysisTick{next: func(context.Context, requestintent.Scope, string, int64) (requestintent.Pending, error) {
		return requestintent.Pending{After: 1}, nil
	}, consume: func(ctx context.Context, _ requestintent.Pending) error {
		close(started)
		<-ctx.Done()
		close(finished)
		return ctx.Err()
	}}
	worker := c.analysisLoop(t.Context(), work, slog.New(slog.NewTextHandler(io.Discard, nil)))
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("worker not started")
	}
	done := make(chan struct{})
	go func() { worker.close(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not join")
	}
	select {
	case <-finished:
	default:
		t.Fatal("child outlived shutdown")
	}
	disabled := performanceAnalysisConfig{}
	if disabled.startAnalysis(t.Context(), nil, nil, perfeng.AnalysisConsumer{}, nil) != nil {
		t.Fatal("disabled worker started")
	}
}

func TestAnalysisStartupChecksResourcesBeforeDatabase(t *testing.T) {
	values := analysisEnvironment(t)
	app, err := newApplication(t.Context(), func(name string) string {
		switch name {
		case "ARGUS_DATABASE_URL":
			return "postgres://invalid.invalid/argus"
		case "ARGUS_GITHUB_TOKEN":
			return "token"
		case "ARGUS_GITHUB_WEBHOOK_SECRET":
			return "secret"
		}

		return values[strings.TrimPrefix(name, "ARGUS_PERFORMANCE_ANALYSIS_")]
	}, false)
	if app != nil || !errors.Is(err, errAnalysisResource) {
		t.Fatal("startup did not check resources before database", err)
	}
}
