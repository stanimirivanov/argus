// Package processadapter invokes an explicitly configured CI-local adapter.
package processadapter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/execution"
	executioncontract "github.com/stanimirivanov/argus/internal/execution/adapters/contract"
	"github.com/stanimirivanov/argus/internal/execution/functionalapi"
	"github.com/stanimirivanov/argus/internal/processprotocol"
)

const maxAdapterRuntime = 2 * time.Hour

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
	request functionalapi.Request,
) (functionalapi.AdapterResult, error) {
	if adapter == nil || len(adapter.command) == 0 {
		return functionalapi.AdapterResult{}, execution.ErrInvalid
	}
	document, err := executioncontract.ExportRequestV1(request)
	if err != nil {
		return functionalapi.AdapterResult{}, err
	}
	input, err := json.Marshal(document)
	if err != nil {
		return functionalapi.AdapterResult{}, fmt.Errorf("encode adapter request: %w", err)
	}
	output, err := processprotocol.Run(ctx, processprotocol.Options{
		Command: adapter.command, Input: input, Stderr: adapter.stderr, Timeout: maxAdapterRuntime,
	})
	if err != nil {
		return functionalapi.AdapterResult{}, err
	}
	resultDocument, err := contracts.DecodeFunctionalAPIAdapterResultV1(output)
	if err != nil {
		return functionalapi.AdapterResult{}, fmt.Errorf("decode adapter result: %w", err)
	}

	return executioncontract.ImportResultV1(resultDocument)
}
