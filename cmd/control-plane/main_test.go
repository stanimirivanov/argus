package main

import (
	"context"
	"encoding/json"
	"log/slog"
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
	done := make(chan struct{})

	go func() {
		run(ctx, logger)
		close(done)
	}()

	started := waitForLogEntry(t, entries)
	assertLogValue(t, started, "msg", "control plane started")
	assertLogValue(t, started, "component", componentName)

	select {
	case <-done:
		t.Fatal("run returned before cancellation")
	default:
	}

	cancel()

	select {
	case <-done:
	case <-time.After(testTimeout):
		t.Fatal("run did not return after cancellation")
	}

	stopped := waitForLogEntry(t, entries)
	assertLogValue(t, stopped, "msg", "control plane stopped")
	assertLogValue(t, stopped, "component", componentName)
	assertLogValue(t, stopped, "reason", "shutdown requested")
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
