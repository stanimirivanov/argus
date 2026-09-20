// Package processadapter invokes an explicitly configured CI-local adapter.
package processadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"

	"github.com/stanimirivanov/argus/contracts"
	"github.com/stanimirivanov/argus/internal/execution"
	executioncontract "github.com/stanimirivanov/argus/internal/execution/adapters/contract"
)

const maxAdapterOutputBytes = 8 << 20

// Adapter exchanges one JSON request/result pair with a configured process.
// Arguments are passed directly to the operating system; no command shell or
// manifest-supplied executable is involved.
type Adapter struct {
	command []string
	stderr  io.Writer
}

// New constructs an adapter for an explicit executable and argument vector.
func New(command []string, stderr io.Writer) (*Adapter, error) {
	if len(command) == 0 || command[0] == "" {
		return nil, fmt.Errorf("%w: missing adapter command", execution.ErrInvalid)
	}
	if stderr == nil {
		stderr = io.Discard
	}

	return &Adapter{command: append([]string{}, command...), stderr: stderr}, nil
}

// Execute sends the validated request on stdin and accepts exactly one bounded
// adapter-result JSON document on stdout.
func (adapter *Adapter) Execute(
	ctx context.Context,
	request execution.Request,
) (execution.AdapterResult, error) {
	if adapter == nil || len(adapter.command) == 0 {
		return execution.AdapterResult{}, execution.ErrInvalid
	}
	document, err := executioncontract.ExportRequestV1(request)
	if err != nil {
		return execution.AdapterResult{}, err
	}
	input, err := json.Marshal(document)
	if err != nil {
		return execution.AdapterResult{}, fmt.Errorf("encode adapter request: %w", err)
	}
	output := &boundedBuffer{maximum: maxAdapterOutputBytes}
	command := exec.CommandContext(ctx, adapter.command[0], adapter.command[1:]...) //nolint:gosec // Explicit CI configuration; never manifest input.
	command.Stdin = bytes.NewReader(input)
	command.Stdout = output
	command.Stderr = adapter.stderr
	if err := command.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return execution.AdapterResult{}, fmt.Errorf("adapter process context: %w", ctxErr)
		}

		return execution.AdapterResult{}, fmt.Errorf("adapter process exited: %w", err)
	}
	if output.exceeded {
		return execution.AdapterResult{}, errors.New("adapter result exceeds 8 MiB limit")
	}
	resultDocument, err := contracts.DecodeFunctionalAPIAdapterResultV1(output.Bytes())
	if err != nil {
		return execution.AdapterResult{}, fmt.Errorf("decode adapter result: %w", err)
	}

	return executioncontract.ImportResultV1(resultDocument)
}

type boundedBuffer struct {
	bytes.Buffer
	maximum  int
	exceeded bool
}

func (buffer *boundedBuffer) Write(data []byte) (int, error) {
	accepted := len(data)
	remaining := buffer.maximum - buffer.Len()
	if remaining <= 0 {
		buffer.exceeded = true

		return accepted, nil
	}
	if len(data) > remaining {
		buffer.exceeded = true
		data = data[:remaining]
	}
	_, err := buffer.Buffer.Write(data)

	return accepted, err
}
