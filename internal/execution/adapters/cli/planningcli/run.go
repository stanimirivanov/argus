// Package planningcli owns manifest/bindings input and execution-plan output.
package planningcli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/stanimirivanov/argus/internal/commandline"
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

	schedulePath := flags.String("schedule", "", "opt-in predictive portfolio path")
	scheduleSHA := flags.String("schedule-sha256", "", "trusted exact portfolio file SHA-256")
	episode := flags.String("episode", "", "exact portfolio episode")
	mode := flags.String("mode", "", "shadow (default when opted in) or staged")
	promotion := addPromotionFlags(flags)
	if err := commandline.Parse(flags, arguments); err != nil {
		return fmt.Errorf("plan functional API execution: %w", err)
	}
	if *manifestPath == "" || *bindingsPath == "" || flags.NArg() != 0 ||
		(*manifestPath == "-" && *bindingsPath == "-") {
		return commandline.UsageText("usage: plan-functional-api -manifest <path|-> -bindings <path|->")
	}

	manifestData, err := readDocument(*manifestPath, stdin, "execution manifest")
	if err != nil {
		return err
	}
	manifest, err := selectioncontract.ImportExecutableManifest(manifestData)
	if err != nil {
		return fmt.Errorf("import execution manifest: %w", err)
	}
	manifestSHA256, err := selectioncontract.ExecutableManifestSHA256(manifest)
	if err != nil {
		return err
	}

	bindingsData, err := readDocument(*bindingsPath, stdin, "execution bindings")
	if err != nil {
		return err
	}

	if *schedulePath != "" {
		if *mode == "" {
			*mode = "shadow"
		}
		return runStagePlan(manifest, manifestSHA256, bindingsData, *schedulePath, *scheduleSHA, *episode, *mode, promotion, stdout)
	}
	if *scheduleSHA != "" || *episode != "" || *mode != "" || promotion.provided() {
		return commandline.UsageText("staged flags require -schedule")
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
