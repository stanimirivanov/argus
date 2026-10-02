// Package processadapter invokes an explicitly configured adaptation adapter.
package processadapter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/stanimirivanov/argus/internal/adaptation"
	adaptationcontract "github.com/stanimirivanov/argus/internal/adaptation/adapters/contract"
	"github.com/stanimirivanov/argus/internal/adaptation/endpointrepair"
	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/processprotocol"
)

const maxAdapterRuntime = 10 * time.Minute

// Adapter exchanges one request/result pair with reviewed CI-local code.
type Adapter struct {
	command []string
	stderr  io.Writer
}

// New constructs an adapter from an explicit executable and argument vector.
func New(command []string, stderr io.Writer) (*Adapter, error) {
	if len(command) == 0 || command[0] == "" {
		return nil, fmt.Errorf("%w: missing adapter command", adaptation.ErrInvalid)
	}
	if stderr == nil {
		stderr = io.Discard
	}

	return &Adapter{command: append([]string{}, command...), stderr: stderr}, nil
}

// Propose sends one bounded request and accepts one bounded JSON result. It
// never invokes a shell and never grants the adapter mutation authority.
func (adapter *Adapter) Propose(
	ctx context.Context,
	request endpointrepair.AdapterRequest,
) (endpointrepair.AdapterResult, error) {
	if adapter == nil || len(adapter.command) == 0 {
		return endpointrepair.AdapterResult{}, adaptation.ErrInvalid
	}
	document, err := adaptationcontract.ExportRequestV1(request)
	if err != nil {
		return endpointrepair.AdapterResult{}, err
	}
	input, err := json.Marshal(document)
	if err != nil {
		return endpointrepair.AdapterResult{}, fmt.Errorf("encode adaptation request: %w", err)
	}
	output, err := processprotocol.Run(ctx, processprotocol.Options{
		Command: adapter.command, Input: input, Stderr: adapter.stderr, Timeout: maxAdapterRuntime,
	})
	if err != nil {
		return endpointrepair.AdapterResult{}, err
	}
	resultDocument, err := contracts.DecodeFunctionalAPIAdaptationResultV1(output)
	if err != nil {
		return endpointrepair.AdapterResult{}, fmt.Errorf("decode adaptation adapter result: %w", err)
	}

	return adaptationcontract.ImportResultV1(resultDocument)
}
