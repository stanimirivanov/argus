package processprotocol

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRunExchangesBoundedDocument(t *testing.T) {
	t.Parallel()
	input := []byte(`{"request":"one"}`)
	var diagnostics bytes.Buffer
	output, err := Run(t.Context(), Options{
		Command: helperCommand("echo"), Input: input, Stderr: &diagnostics, Timeout: 10 * time.Second,
	})
	if err != nil || !bytes.Equal(output, input) || diagnostics.String() != "adapter diagnostic" {
		t.Fatalf("Run() = %q, %v, stderr %q", output, err, diagnostics.String())
	}
}

func TestRunRequiresDeadlineAndBoundedRequest(t *testing.T) {
	t.Parallel()
	if _, err := Run(t.Context(), Options{Command: helperCommand("echo")}); err == nil {
		t.Fatal("Run accepted an adapter without a deadline")
	}
	if _, err := Run(t.Context(), Options{
		Command: helperCommand("echo"), Input: make([]byte, MaxInputBytes+1), Timeout: time.Second,
	}); err == nil || !strings.Contains(err.Error(), "request exceeds") {
		t.Fatalf("Run oversized request error = %v", err)
	}
}

func TestRunStopsOnStdoutOverflow(t *testing.T) {
	t.Parallel()
	start := time.Now()
	_, err := Run(t.Context(), Options{Command: helperCommand("overflow"), Timeout: 10 * time.Second})
	if !errors.Is(err, ErrOutputLimit) || time.Since(start) >= 10*time.Second {
		t.Fatalf("Run() error = %v, elapsed %s", err, time.Since(start))
	}
}

func TestRunDistinguishesCancellationTimeoutAndExit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		mode    string
		ctx     func() (context.Context, context.CancelFunc)
		timeout time.Duration
		want    error
	}{
		{"timeout", "wait", func() (context.Context, context.CancelFunc) { return context.WithCancel(t.Context()) }, 200 * time.Millisecond, context.DeadlineExceeded},
		{"cancellation", "wait", func() (context.Context, context.CancelFunc) {
			ctx, cancel := context.WithCancel(t.Context())
			time.AfterFunc(200*time.Millisecond, cancel)
			return ctx, cancel
		}, 10 * time.Second, context.Canceled},
		{"nonzero exit", "exit", func() (context.Context, context.CancelFunc) { return context.WithCancel(t.Context()) }, 10 * time.Second, nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := test.ctx()
			defer cancel()
			_, err := Run(ctx, Options{Command: helperCommand(test.mode), Timeout: test.timeout})
			if err == nil || test.want != nil && !errors.Is(err, test.want) ||
				test.want == nil && !strings.Contains(err.Error(), "exited") {
				t.Fatalf("Run() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestDiagnosticTailIsBounded(t *testing.T) {
	t.Parallel()
	var tail diagnosticTail
	_, _ = tail.Write(bytes.Repeat([]byte("a"), MaxDiagnosticBytes))
	_, _ = tail.Write([]byte("latest"))
	if len(tail.bytes) != MaxDiagnosticBytes || !bytes.HasSuffix(tail.bytes, []byte("latest")) {
		t.Fatalf("tail length = %d, suffix = %q", len(tail.bytes), tail.bytes[len(tail.bytes)-16:])
	}
}

func TestRunDoesNotWaitIndefinitelyForDescendantPipes(t *testing.T) {
	t.Parallel()
	start := time.Now()
	var diagnostics bytes.Buffer
	_, err := Run(t.Context(), Options{
		Command: helperCommand("spawn"), Stderr: &diagnostics, Timeout: 15 * time.Second,
	})
	pid, parseErr := strconv.Atoi(strings.TrimSpace(diagnostics.String()))
	if parseErr == nil {
		child, findErr := os.FindProcess(pid)
		if findErr == nil {
			_ = child.Kill()
			_, _ = child.Wait()
		}
	}
	if parseErr != nil {
		t.Fatalf("descendant PID diagnostic %q: %v", diagnostics.String(), parseErr)
	}
	if !errors.Is(err, ErrInheritedPipes) || time.Since(start) > 10*time.Second {
		t.Fatalf("Run() error = %v, elapsed %s", err, time.Since(start))
	}
}

func helperCommand(mode string) []string {
	return []string{os.Args[0], "-test.run=^TestProcessProtocolHelper$", "--", mode}
}

func TestProcessProtocolHelper(t *testing.T) {
	if len(os.Args) < 2 || os.Args[len(os.Args)-2] != "--" {
		return
	}
	switch os.Args[len(os.Args)-1] {
	case "echo":
		_, _ = io.Copy(os.Stdout, os.Stdin)
		_, _ = io.WriteString(os.Stderr, "adapter diagnostic")
		os.Exit(0)
	case "overflow":
		_, _ = os.Stdout.Write(bytes.Repeat([]byte("x"), MaxOutputBytes+1))
		time.Sleep(10 * time.Second)
	case "wait":
		time.Sleep(10 * time.Second)
	case "exit":
		os.Exit(7)
	case "spawn":
		child := exec.Command(os.Args[0], "-test.run=^TestProcessProtocolHelper$", "--", "linger") //nolint:gosec // Test-only descendant of this binary.
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		if child.Start() != nil {
			os.Exit(8)
		}
		_, _ = fmt.Fprintln(os.Stderr, child.Process.Pid)
		os.Exit(0)
	case "linger":
		time.Sleep(12 * time.Second)
		os.Exit(0)
	}
}
