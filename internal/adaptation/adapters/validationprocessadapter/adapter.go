// Package validationprocessadapter invokes a validation adapter in a disposable checkout.
package validationprocessadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/stanimirivanov/argus/contracts"
	"github.com/stanimirivanov/argus/internal/adaptation"
	adaptationcontract "github.com/stanimirivanov/argus/internal/adaptation/adapters/contract"
)

const maxAdapterOutputBytes = 8 << 20

// Adapter exchanges phase documents with an explicit command whose working
// directory is the disposable checkout controlled by the validation service.
type Adapter struct {
	command   []string
	directory string
	stderr    io.Writer
}

// New constructs a no-shell validation process adapter.
func New(command []string, directory string, stderr io.Writer) (*Adapter, error) {
	if len(command) == 0 || command[0] == "" || directory == "" {
		return nil, fmt.Errorf("%w: validation adapter configuration", adaptation.ErrInvalid)
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return nil, fmt.Errorf("resolve validation directory: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, fmt.Errorf("inspect validation directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%w: validation directory is not a directory", adaptation.ErrInvalid)
	}
	if stderr == nil {
		stderr = io.Discard
	}

	return &Adapter{command: append([]string{}, command...), directory: absolute, stderr: stderr}, nil
}

// Execute sends one validated phase request and accepts one bounded result.
func (adapter *Adapter) Execute(
	ctx context.Context,
	request adaptation.ValidationRequest,
) (adaptation.ValidationAdapterResult, error) {
	if adapter == nil || len(adapter.command) == 0 || adapter.directory == "" {
		return adaptation.ValidationAdapterResult{}, adaptation.ErrInvalid
	}
	document, err := adaptationcontract.ExportValidationRequestV1(request)
	if err != nil {
		return adaptation.ValidationAdapterResult{}, err
	}
	input, err := json.Marshal(document)
	if err != nil {
		return adaptation.ValidationAdapterResult{}, fmt.Errorf("encode validation request: %w", err)
	}
	output := &boundedBuffer{maximum: maxAdapterOutputBytes}
	command := exec.CommandContext(ctx, adapter.command[0], adapter.command[1:]...) //nolint:gosec // Reviewed CI configuration, never proposal input.
	command.Dir = adapter.directory
	command.Stdin = bytes.NewReader(input)
	command.Stdout = output
	command.Stderr = adapter.stderr
	if err := command.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return adaptation.ValidationAdapterResult{}, fmt.Errorf("validation adapter context: %w", ctxErr)
		}

		return adaptation.ValidationAdapterResult{}, fmt.Errorf("validation adapter exited: %w", err)
	}
	if output.exceeded {
		return adaptation.ValidationAdapterResult{}, errors.New("validation adapter result exceeds 8 MiB limit")
	}
	resultDocument, err := contracts.DecodeFunctionalAPIRepairValidationResultV1(output.Bytes())
	if err != nil {
		return adaptation.ValidationAdapterResult{}, fmt.Errorf("decode validation adapter result: %w", err)
	}

	return adaptationcontract.ImportValidationResultV1(resultDocument)
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
