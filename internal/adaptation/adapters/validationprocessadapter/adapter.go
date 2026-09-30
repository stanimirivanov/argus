// Package validationprocessadapter invokes a validation adapter in a disposable checkout.
package validationprocessadapter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/stanimirivanov/argus/contracts"
	"github.com/stanimirivanov/argus/internal/adaptation"
	adaptationcontract "github.com/stanimirivanov/argus/internal/adaptation/adapters/contract"
	"github.com/stanimirivanov/argus/internal/processprotocol"
)

const maxAdapterRuntime = 2 * time.Hour

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
	output, err := processprotocol.Run(ctx, processprotocol.Options{
		Command: adapter.command, Directory: adapter.directory, Input: input,
		Stderr: adapter.stderr, Timeout: maxAdapterRuntime,
	})
	if err != nil {
		return adaptation.ValidationAdapterResult{}, err
	}
	resultDocument, err := contracts.DecodeFunctionalAPIRepairValidationResultV1(output)
	if err != nil {
		return adaptation.ValidationAdapterResult{}, fmt.Errorf("decode validation adapter result: %w", err)
	}

	return adaptationcontract.ImportValidationResultV1(resultDocument)
}
