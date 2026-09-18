package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"testing"
	"time"
)

const testTimeout = 5 * time.Second

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

func TestLoadConfigRequiresSecretsAndUsesSafeDefaults(t *testing.T) {
	t.Parallel()

	values := map[string]string{
		"ARGUS_DATABASE_URL":          "postgres://example.invalid/argus",
		"ARGUS_GITHUB_TOKEN":          "token",
		"ARGUS_GITHUB_WEBHOOK_SECRET": "secret",
	}
	config, err := loadConfig(func(name string) string { return values[name] })
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if config.address != "127.0.0.1:8080" || config.githubAPIURL != "https://api.github.com/" ||
		config.githubHost != "github.com" {
		t.Fatalf("unexpected defaults: %#v", config)
	}
	delete(values, "ARGUS_GITHUB_WEBHOOK_SECRET")
	if _, err := loadConfig(func(name string) string { return values[name] }); err == nil {
		t.Fatal("expected missing webhook secret to fail")
	}
}

type fakeServer struct {
	stopped chan struct{}
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
