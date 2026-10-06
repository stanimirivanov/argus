package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/commandline"
)

const testTimeout = 5 * time.Second

func TestRunControlPlaneDrainsHTTPBeforeCancelingApplication(t *testing.T) {
	t.Parallel()

	signalCtx, stop := context.WithCancel(t.Context())
	defer stop()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := newDrainingServer()
	defer server.allowShutdown()
	applicationContext := make(chan context.Context, 1)
	lifetimeAtClose := make(chan error, 1)
	applicationClosed := make(chan struct{})
	done := make(chan int, 1)
	go func() {
		done <- runControlPlane(signalCtx, logger, func(ctx context.Context) (managedApplication, error) {
			applicationContext <- ctx

			return managedApplication{server: server, close: func() error {
				lifetimeAtClose <- ctx.Err()
				close(applicationClosed)
				return nil
			}}, nil
		})
	}()

	var lifetimeCtx context.Context
	select {
	case lifetimeCtx = <-applicationContext:
	case <-time.After(testTimeout):
		t.Fatal("control plane did not open application resources")
	}
	select {
	case <-server.started:
	case <-time.After(testTimeout):
		t.Fatal("control plane did not start serving")
	}
	stop()
	select {
	case <-server.shutdownStarted:
	case <-time.After(testTimeout):
		t.Fatal("control plane did not start HTTP shutdown")
	}
	if err := lifetimeCtx.Err(); err != nil {
		t.Fatalf("shutdown signal canceled application work before HTTP drain: %v", err)
	}
	select {
	case <-applicationClosed:
		t.Fatal("application resources closed before HTTP drain")
	default:
	}

	server.allowShutdown()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("control plane exit code = %d, want success", code)
		}
	case <-time.After(testTimeout):
		t.Fatal("control plane did not finish after HTTP drain")
	}
	select {
	case <-applicationClosed:
	default:
		t.Fatal("application resources were not closed after HTTP drain")
	}
	if err := <-lifetimeAtClose; err != nil {
		t.Fatalf("application context canceled before resource cleanup: %v", err)
	}
	if !errors.Is(lifetimeCtx.Err(), context.Canceled) {
		t.Fatalf("application context after cleanup = %v, want cancellation", lifetimeCtx.Err())
	}
}

func TestRunWaitsForCancellationAndLogsLifecycle(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	entries := make(chan []byte, 2)
	logger := slog.New(slog.NewJSONHandler(channelWriter{entries: entries}, nil))
	fake := newFakeServer()
	done := make(chan error, 1)
	go func() {
		done <- run(ctx, logger, fake)
	}()

	started := waitForLogEntry(t, entries)
	assertLogValue(t, started, "msg", "control plane started")
	assertLogValue(t, started, "component", componentName)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	case <-time.After(testTimeout):
		t.Fatal("run did not return after cancellation")
	}

	stopped := waitForLogEntry(t, entries)
	assertLogValue(t, stopped, "msg", "control plane stopped")
	assertLogValue(t, stopped, "reason", "shutdown requested")
}

func TestLoadConfigLocalDefaultsAndBoundaries(t *testing.T) {
	t.Parallel()
	values := map[string]string{
		"ARGUS_GITHUB_TOKEN":          "token",
		"ARGUS_GITHUB_WEBHOOK_SECRET": "secret",
	}
	read := func(name string) string { return values[name] }
	got, err := loadConfig(read, true)
	if err != nil || got.databaseURL != commandline.LocalDatabaseURL || got.address != "127.0.0.1:8080" {
		t.Fatalf("local config = %#v, error = %v", got, err)
	}
	values["ARGUS_HTTP_ADDRESS"] = "0.0.0.0:8080"
	if _, err := loadConfig(read, true); err == nil {
		t.Fatal("local mode accepted a non-loopback listener")
	}
	values["ARGUS_HTTP_ADDRESS"] = "127.0.0.1:8080"
	values["ARGUS_DATABASE_URL"] = "postgres://example.invalid/argus"
	if _, err := loadConfig(read, true); err == nil {
		t.Fatal("local mode accepted an explicit database URL")
	}
	delete(values, "ARGUS_DATABASE_URL")
	delete(values, "ARGUS_GITHUB_TOKEN")
	if _, err := loadConfig(read, true); err == nil {
		t.Fatal("local mode accepted a missing GitHub token")
	}
}

func TestLoadConfigRequiresSecretsAndUsesSafeDefaults(t *testing.T) {
	t.Parallel()

	values := map[string]string{
		"ARGUS_DATABASE_URL":          "postgres://example.invalid/argus",
		"ARGUS_GITHUB_TOKEN":          "token",
		"ARGUS_GITHUB_WEBHOOK_SECRET": "secret",
	}
	config, err := loadConfig(func(name string) string { return values[name] }, false)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if config.address != "127.0.0.1:8080" || config.githubAPIURL != "https://api.github.com/" ||
		config.githubHost != "github.com" {
		t.Fatalf("unexpected defaults: %#v", config)
	}
	delete(values, "ARGUS_GITHUB_WEBHOOK_SECRET")
	if _, err := loadConfig(func(name string) string { return values[name] }, false); err == nil {
		t.Fatal("expected missing webhook secret to fail")
	}
}

func TestLoadConfigDBOSEvaluationRequiresExplicitOptIn(t *testing.T) {
	t.Parallel()
	values := map[string]string{
		"ARGUS_DATABASE_URL":          "postgres://example.invalid/argus",
		"ARGUS_GITHUB_TOKEN":          "token",
		"ARGUS_GITHUB_WEBHOOK_SECRET": "secret",
	}
	read := func(name string) string { return values[name] }
	defaultConfig, err := loadConfig(read, false)
	if err != nil || defaultConfig.dbosEvaluation {
		t.Fatalf("default evaluation = %t, error = %v", defaultConfig.dbosEvaluation, err)
	}
	values["ARGUS_DBOS_EVALUATION"] = "true"
	enabled, err := loadConfig(read, false)
	if err != nil || !enabled.dbosEvaluation {
		t.Fatalf("enabled evaluation = %t, error = %v", enabled.dbosEvaluation, err)
	}
	values["ARGUS_DBOS_EVALUATION"] = "yes"
	if _, err := loadConfig(read, false); err == nil {
		t.Fatal("ambiguous opt-in must be rejected")
	}
}

type fakeServer struct {
	stopped chan struct{}
}

type drainingServer struct {
	started         chan struct{}
	shutdownStarted chan struct{}
	release         chan struct{}
	stopped         chan struct{}
	once            sync.Once
}

func newDrainingServer() *drainingServer {
	return &drainingServer{
		started: make(chan struct{}), shutdownStarted: make(chan struct{}),
		release: make(chan struct{}), stopped: make(chan struct{}),
	}
}

func (server *drainingServer) allowShutdown() {
	server.once.Do(func() { close(server.release) })
}

func (server *drainingServer) ListenAndServe() error {
	close(server.started)
	<-server.stopped
	return http.ErrServerClosed
}

func (server *drainingServer) Shutdown(ctx context.Context) error {
	close(server.shutdownStarted)
	select {
	case <-server.release:
		close(server.stopped)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func newFakeServer() *fakeServer {
	return &fakeServer{stopped: make(chan struct{})}
}

func (server *fakeServer) ListenAndServe() error {
	<-server.stopped
	return http.ErrServerClosed
}

func (server *fakeServer) Shutdown(context.Context) error {
	select {
	case <-server.stopped:
		return errors.New("server already stopped")
	default:
		close(server.stopped)
		return nil
	}
}

type channelWriter struct {
	entries chan<- []byte
}

func (w channelWriter) Write(data []byte) (int, error) {
	entry := append([]byte(nil), data...)
	w.entries <- entry
	return len(data), nil
}

func waitForLogEntry(t *testing.T, entries <-chan []byte) map[string]any {
	t.Helper()

	select {
	case entry := <-entries:
		var decoded map[string]any
		if err := json.Unmarshal(entry, &decoded); err != nil {
			t.Fatalf("decode structured log: %v", err)
		}

		return decoded
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for lifecycle log")
		return nil
	}
}

func assertLogValue(t *testing.T, entry map[string]any, key string, want any) {
	t.Helper()
	if got := entry[key]; got != want {
		t.Errorf("log field %q = %v, want %v", key, got, want)
	}
}
