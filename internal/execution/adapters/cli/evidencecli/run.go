// Package evidencecli owns ingestion and shadow-report command boundaries.
package evidencecli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/contracts"
	executioncontract "github.com/stanimirivanov/argus/internal/execution/adapters/contract"
	"github.com/stanimirivanov/argus/internal/execution/attempts"
	"github.com/stanimirivanov/argus/internal/execution/shadow"
)

const maxEvidenceDocumentBytes = 16 << 20

// Runtime owns the durable execution-evidence ports used by this command.
type Runtime interface {
	attempts.Store
	shadow.AttemptReader
	Close()
}

// OpenRuntime creates infrastructure only after command input is validated.
type OpenRuntime func(context.Context, string) (Runtime, error)

// Run dispatches execution-evidence subcommands.
func Run(
	ctx context.Context,
	arguments []string,
	databaseURL string,
	stdin io.Reader,
	stdout io.Writer,
	open OpenRuntime,
) error {
	if len(arguments) == 0 {
		return usageError()
	}
	switch arguments[0] {
	case "ingest":
		return runIngest(ctx, arguments[1:], databaseURL, stdin, stdout, open)
	case "shadow-report":
		return runShadowReport(ctx, arguments[1:], databaseURL, stdout, open)
	case "plan-shadow-report":
		return runPlanShadowReport(ctx, arguments[1:], databaseURL, stdin, stdout, open)
	default:
		return usageError()
	}
}

func runIngest(
	ctx context.Context,
	arguments []string,
	databaseURL string,
	stdin io.Reader,
	stdout io.Writer,
	open OpenRuntime,
) error {
	flags := flag.NewFlagSet("execution-evidence ingest", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	filePath := flags.String("file", "", "execution attempt path, or - for stdin")
	if err := commandline.Parse(flags, arguments); err != nil {
		return fmt.Errorf("execution-evidence ingest: %w", err)
	}
	if *filePath == "" || flags.NArg() != 0 {
		return commandline.UsageText("usage: execution-evidence ingest -file <path|->")
	}
	data, err := readEvidenceDocument(*filePath, stdin, "execution attempt")
	if err != nil {
		return err
	}
	document, err := contracts.DecodeExecutionAttemptV1(data)
	if err != nil {
		return fmt.Errorf("read execution attempt: %w", err)
	}
	attempt, err := executioncontract.ImportAttemptV1(document)
	if err != nil {
		return fmt.Errorf("import execution attempt: %w", err)
	}
	runtime, err := openStore(ctx, databaseURL, open)
	if err != nil {
		return err
	}
	defer runtime.Close()

	created, err := attempts.NewService(runtime).Ingest(ctx, attempt)
	if err != nil {
		return fmt.Errorf("ingest execution attempt: %w", err)
	}

	return encodeJSON(stdout, struct {
		AttemptID string `json:"attemptId"`
		Created   bool   `json:"created"`
	}{AttemptID: attempt.AttemptID, Created: created})
}

func runPlanShadowReport(
	ctx context.Context,
	arguments []string,
	databaseURL string,
	stdin io.Reader,
	stdout io.Writer,
	open OpenRuntime,
) error {
	flags := flag.NewFlagSet("execution-evidence plan-shadow-report", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	planPath := flags.String("plan", "", "execution plan path, or - for stdin")
	bindingsPath := flags.String("attempt-bindings", "", "attempt bindings path, or - for stdin")
	if err := commandline.Parse(flags, arguments); err != nil {
		return fmt.Errorf("execution-evidence plan-shadow-report: %w", err)
	}
	if *planPath == "" || *bindingsPath == "" || flags.NArg() != 0 ||
		(*planPath == "-" && *bindingsPath == "-") {
		return commandline.UsageText("usage: execution-evidence plan-shadow-report " +
			"-plan <path|-> -attempt-bindings <path|->")
	}
	planData, err := readEvidenceDocument(*planPath, stdin, "execution plan")
	if err != nil {
		return err
	}
	planDocument, err := contracts.DecodeFunctionalAPIExecutionPlanV1(planData)
	if err != nil {
		return fmt.Errorf("read functional API execution plan: %w", err)
	}
	plan, err := executioncontract.ImportFunctionalAPIExecutionPlanV1(planDocument)
	if err != nil {
		return fmt.Errorf("import functional API execution plan: %w", err)
	}
	planSHA256, err := executioncontract.PlanSHA256V1(plan)
	if err != nil {
		return err
	}
	bindingsData, err := readEvidenceDocument(*bindingsPath, stdin, "execution plan attempt bindings")
	if err != nil {
		return err
	}
	bindingsDocument, err := contracts.DecodeExecutionPlanAttemptBindingsV1(bindingsData)
	if err != nil {
		return fmt.Errorf("read execution plan attempt bindings: %w", err)
	}
	bindings, err := executioncontract.ImportExecutionPlanAttemptBindingsV1(bindingsDocument)
	if err != nil {
		return fmt.Errorf("import execution plan attempt bindings: %w", err)
	}
	runtime, err := openStore(ctx, databaseURL, open)
	if err != nil {
		return err
	}
	defer runtime.Close()

	report, err := shadow.NewService(runtime).ComparePlan(ctx, plan, planSHA256, bindings)
	if err != nil {
		return fmt.Errorf("compare execution plan attempts: %w", err)
	}
	document, err := executioncontract.ExportSelectionPlanShadowReportV1(report)
	if err != nil {
		return err
	}

	return encodeJSON(stdout, document)
}

func runShadowReport(
	ctx context.Context,
	arguments []string,
	databaseURL string,
	stdout io.Writer,
	open OpenRuntime,
) error {
	flags := flag.NewFlagSet("execution-evidence shadow-report", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	selectedID := flags.String("selected-attempt", "", "selected-stage attempt ID")
	fullSuiteID := flags.String("full-suite-attempt", "", "full-suite-stage attempt ID")
	if err := commandline.Parse(flags, arguments); err != nil {
		return fmt.Errorf("execution-evidence shadow-report: %w", err)
	}
	if *selectedID == "" || *fullSuiteID == "" || flags.NArg() != 0 {
		return commandline.UsageText("usage: execution-evidence shadow-report " +
			"-selected-attempt <id> -full-suite-attempt <id>")
	}
	runtime, err := openStore(ctx, databaseURL, open)
	if err != nil {
		return err
	}
	defer runtime.Close()

	report, err := shadow.NewService(runtime).Compare(ctx, *selectedID, *fullSuiteID)
	if err != nil {
		return fmt.Errorf("compare execution attempts: %w", err)
	}
	document, err := executioncontract.ExportSelectionShadowReportV1(report)
	if err != nil {
		return err
	}

	return encodeJSON(stdout, document)
}

func openStore(ctx context.Context, databaseURL string, open OpenRuntime) (Runtime, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return nil, errors.New("ARGUS_DATABASE_URL is required")
	}
	if open == nil {
		return nil, errors.New("execution evidence runtime factory is required")
	}
	runtime, err := open(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open execution evidence store: %w", err)
	}

	return runtime, nil
}

func readEvidenceDocument(path string, stdin io.Reader, name string) ([]byte, error) {
	if path == "-" {
		if stdin == nil {
			return nil, fmt.Errorf("%s stdin is required", name)
		}
		return readBoundedEvidenceDocument(stdin, name)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", name, err)
	}
	data, readErr := readBoundedEvidenceDocument(file, name)
	closeErr := file.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close %s: %w", name, closeErr)
	}

	return data, nil
}

func readBoundedEvidenceDocument(reader io.Reader, name string) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxEvidenceDocumentBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	if len(data) > maxEvidenceDocumentBytes {
		return nil, fmt.Errorf("%s exceeds 16 MiB limit", name)
	}

	return data, nil
}

func encodeJSON(output io.Writer, value any) error {
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("encode execution evidence: %w", err)
	}

	return nil
}

func usageError() error {
	return commandline.UsageText("usage: execution-evidence <ingest|shadow-report|plan-shadow-report> [options]")
}
