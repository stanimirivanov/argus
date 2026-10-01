// Package planningcli owns manifest/bindings input and execution-plan output.
package planningcli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/stanimirivanov/argus/internal/contracts"
	executioncontract "github.com/stanimirivanov/argus/internal/execution/adapters/contract"
	"github.com/stanimirivanov/argus/internal/execution/planning"
	selectioncontract "github.com/stanimirivanov/argus/internal/selection/adapters/contract"
)

const maxPlanningDocumentBytes = 16 << 20

// Run creates a deterministic heterogeneous functional API CI job plan.
func Run(_ context.Context, arguments []string, stdin io.Reader, stdout io.Writer) error {
	flags := flag.NewFlagSet("plan-functional-api", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	manifestPath := flags.String("manifest", "", "execution manifest path, or - for stdin")
	bindingsPath := flags.String("bindings", "", "reviewed execution bindings path, or - for stdin")
	if err := flags.Parse(arguments); err != nil {
		return fmt.Errorf("plan functional API execution: %w", err)
	}
	if *manifestPath == "" || *bindingsPath == "" || flags.NArg() != 0 ||
		(*manifestPath == "-" && *bindingsPath == "-") {
		return errors.New("usage: plan-functional-api -manifest <path|-> -bindings <path|->")
	}

	manifestData, err := readDocument(*manifestPath, stdin, "execution manifest")
	if err != nil {
		return err
	}
	manifestDocument, err := contracts.DecodeExecutionManifestV1(manifestData)
	if err != nil {
		return fmt.Errorf("read execution manifest: %w", err)
	}
	manifest, err := selectioncontract.ImportV1(manifestDocument)
	if err != nil {
		return fmt.Errorf("import execution manifest: %w", err)
	}
	manifestSHA256, err := selectioncontract.ManifestSHA256V1(manifest)
	if err != nil {
		return err
	}

	bindingsData, err := readDocument(*bindingsPath, stdin, "execution bindings")
	if err != nil {
		return err
	}
	bindingsDocument, err := contracts.DecodeFunctionalAPIExecutionBindingsV1(bindingsData)
	if err != nil {
		return fmt.Errorf("read execution bindings: %w", err)
	}
	bindings, err := executioncontract.ImportFunctionalAPIExecutionBindingsV1(bindingsDocument)
	if err != nil {
		return fmt.Errorf("import execution bindings: %w", err)
	}
	plan, err := planning.Build(manifest, manifestSHA256, bindings)
	if err != nil {
		return fmt.Errorf("plan functional API execution: %w", err)
	}
	planDocument, err := executioncontract.ExportFunctionalAPIExecutionPlanV1(plan)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(planDocument); err != nil {
		return fmt.Errorf("encode functional API execution plan: %w", err)
	}

	return nil
}

func readDocument(path string, stdin io.Reader, name string) ([]byte, error) {
	if path == "-" {
		if stdin == nil {
			return nil, fmt.Errorf("%s stdin is required", name)
		}

		return readBoundedDocument(stdin, name)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", name, err)
	}
	data, readErr := readBoundedDocument(file, name)
	closeErr := file.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close %s: %w", name, closeErr)
	}

	return data, nil
}

func readBoundedDocument(reader io.Reader, name string) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxPlanningDocumentBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	if len(data) > maxPlanningDocumentBytes {
		return nil, fmt.Errorf("%s exceeds 16 MiB limit", name)
	}

	return data, nil
}
