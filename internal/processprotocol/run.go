// Package processprotocol enforces the resource and lifecycle limits shared by
// CI-local adapter processes. It does not interpret their versioned documents.
package processprotocol

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"
)

const (
	// MaxOutputBytes is the largest accepted adapter result document.
	MaxOutputBytes = 8 << 20
	// MaxDiagnosticBytes is the retained tail of adapter stderr.
	MaxDiagnosticBytes = 64 << 10
	// MaxInputBytes bounds the request passed to an adapter.
	MaxInputBytes = 16 << 20
	pipeWait      = 5 * time.Second
)

var (
	// ErrOutputLimit means the adapter exceeded the stdout protocol bound.
	ErrOutputLimit = errors.New("adapter stdout exceeds 8 MiB limit")
	// ErrInheritedPipes means a descendant kept a protocol pipe open after the
	// direct child exited; the worker remains responsible for tree cleanup.
	ErrInheritedPipes = errors.New("adapter process left protocol pipes open")
)

// Options describes one explicitly authorized, shell-free adapter invocation.
// Timeout must be positive even if the caller has an earlier context deadline.
type Options struct {
	Command   []string
	Directory string
	Input     []byte
	Stderr    io.Writer
	Timeout   time.Duration
}

// Run exchanges one request/result pair. The child is canceled when stdout
// overflows or a deadline expires; WaitDelay bounds waiting for inherited pipe
// handles after child termination. Only a bounded stderr tail reaches Stderr.
func Run(ctx context.Context, options Options) ([]byte, error) {
	if len(options.Command) == 0 || options.Command[0] == "" || options.Timeout <= 0 {
		return nil, errors.New("invalid adapter process configuration")
	}
	if len(options.Input) > MaxInputBytes {
		return nil, errors.New("adapter request exceeds 16 MiB limit")
	}
	deadlineCtx, stopDeadline := context.WithTimeout(ctx, options.Timeout)
	defer stopDeadline()
	processCtx, stopProcess := context.WithCancelCause(deadlineCtx)
	defer stopProcess(nil)

	command := exec.CommandContext(processCtx, options.Command[0], options.Command[1:]...) //nolint:gosec // Command comes from reviewed CI configuration, never protocol data.
	command.Dir = options.Directory
	command.Stdin = bytes.NewReader(options.Input)
	command.WaitDelay = pipeWait
	stdout := &limitedOutput{cancel: stopProcess}
	stderr := &diagnosticTail{}
	command.Stdout = stdout
	command.Stderr = stderr
	runErr := command.Run()
	var diagnosticErr error
	if options.Stderr != nil {
		_, diagnosticErr = options.Stderr.Write(stderr.bytes)
	}
	if errors.Is(context.Cause(processCtx), ErrOutputLimit) {
		return nil, ErrOutputLimit
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("adapter process canceled: %w", err)
	}
	if err := deadlineCtx.Err(); err != nil {
		return nil, fmt.Errorf("adapter process timed out: %w", err)
	}
	if errors.Is(runErr, exec.ErrWaitDelay) {
		return nil, fmt.Errorf("%w: %w", ErrInheritedPipes, runErr)
	}
	if runErr != nil {
		return nil, fmt.Errorf("adapter process exited: %w", runErr)
	}
	if diagnosticErr != nil {
		return nil, fmt.Errorf("write adapter diagnostics: %w", diagnosticErr)
	}

	return stdout.bytes, nil
}

// limitedOutput cancels the command on the first byte beyond the protocol
// limit, instead of silently discarding an unbounded child stream.
type limitedOutput struct {
	bytes  []byte
	cancel context.CancelCauseFunc
}

func (output *limitedOutput) Write(data []byte) (int, error) {
	remaining := MaxOutputBytes - len(output.bytes)
	if len(data) > remaining {
		output.bytes = append(output.bytes, data[:remaining]...)
		output.cancel(ErrOutputLimit)
		return 0, ErrOutputLimit
	}
	output.bytes = append(output.bytes, data...)

	return len(data), nil
}

// diagnosticTail retains recent context without allowing stderr to consume
// unbounded memory or logs. It is written by one os/exec copy goroutine.
type diagnosticTail struct{ bytes []byte }

func (tail *diagnosticTail) Write(data []byte) (int, error) {
	accepted := len(data)
	if len(data) >= MaxDiagnosticBytes {
		tail.bytes = append(tail.bytes[:0], data[len(data)-MaxDiagnosticBytes:]...)
		return accepted, nil
	}
	if overflow := len(tail.bytes) + len(data) - MaxDiagnosticBytes; overflow > 0 {
		copy(tail.bytes, tail.bytes[overflow:])
		tail.bytes = tail.bytes[:len(tail.bytes)-overflow]
	}
	tail.bytes = append(tail.bytes, data...)

	return accepted, nil
}
