// Package processadapter invokes an explicitly configured adaptation adapter.
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
	"github.com/stanimirivanov/argus/internal/adaptation"
	adaptationcontract "github.com/stanimirivanov/argus/internal/adaptation/adapters/contract"
)

const maxAdapterOutputBytes = 8 << 20

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
	request adaptation.AdapterRequest,
) (adaptation.AdapterResult, error) {
	if adapter == nil || len(adapter.command) == 0 {
		return adaptation.AdapterResult{}, adaptation.ErrInvalid
	}
	document, err := adaptationcontract.ExportRequestV1(request)
	if err != nil {
		return adaptation.AdapterResult{}, err
	}
	input, err := json.Marshal(document)
	if err != nil {
		return adaptation.AdapterResult{}, fmt.Errorf("encode adaptation request: %w", err)
	}
	output := &boundedBuffer{maximum: maxAdapterOutputBytes}
	command := exec.CommandContext(ctx, adapter.command[0], adapter.command[1:]...) //nolint:gosec // Reviewed CI configuration, never evidence input.
	command.Stdin = bytes.NewReader(input)
	command.Stdout = output
	command.Stderr = adapter.stderr
	if err := command.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return adaptation.AdapterResult{}, fmt.Errorf("adaptation adapter context: %w", ctxErr)
		}
		return adaptation.AdapterResult{}, fmt.Errorf("adaptation adapter exited: %w", err)
	}
	if output.exceeded {
		return adaptation.AdapterResult{}, errors.New("adaptation adapter result exceeds 8 MiB limit")
	}
	resultDocument, err := contracts.DecodeFunctionalAPIAdaptationResultV1(output.Bytes())
	if err != nil {
		return adaptation.AdapterResult{}, fmt.Errorf("decode adaptation adapter result: %w", err)
	}

	return adaptationcontract.ImportResultV1(resultDocument)
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
