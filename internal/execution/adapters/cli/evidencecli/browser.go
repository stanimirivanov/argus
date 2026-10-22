package evidencecli

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/contracts"
	executioncontract "github.com/stanimirivanov/argus/internal/execution/adapters/contract"
	"github.com/stanimirivanov/argus/internal/execution/browser"
	"github.com/stanimirivanov/argus/internal/execution/browsershadow"
)

type browserAttemptStore interface {
	SaveBrowserAttempt(context.Context, browser.Attempt) (bool, error)
}

func runBrowserPlanShadowReport(ctx context.Context, args []string, databaseURL string, stdin io.Reader, stdout io.Writer, open OpenRuntime) error {
	flags := flag.NewFlagSet("execution-evidence browser-plan-shadow-report", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	planPath := flags.String("plan", "", "browser plan path or -")
	bindingsPath := flags.String("attempt-bindings", "", "explicit attempt bindings path or -")
	if err := commandline.Parse(flags, args); err != nil {
		return err
	}
	if *planPath == "" || *bindingsPath == "" || (*planPath == "-" && *bindingsPath == "-") || flags.NArg() != 0 {
		return commandline.UsageText("usage: execution-evidence browser-plan-shadow-report -plan <path|-> -attempt-bindings <path|->")
	}
	data, err := readEvidenceDocument(*planPath, stdin, "browser execution plan")
	if err != nil {
		return err
	}
	planDocument, err := contracts.DecodeBrowserExecutionPlanV1(data)
	if err != nil {
		return err
	}
	plan, err := executioncontract.ImportBrowserExecutionPlan(planDocument)
	if err != nil {
		return err
	}
	digest, err := executioncontract.BrowserPlanSHA256(plan)
	if err != nil {
		return err
	}
	data, err = readEvidenceDocument(*bindingsPath, stdin, "browser attempt bindings")
	if err != nil {
		return err
	}
	bindingDocument, err := contracts.DecodeBrowserPlanAttemptBindingsV1(data)
	if err != nil {
		return err
	}
	bindings := executioncontract.ImportBrowserPlanAttemptBindings(bindingDocument)
	runtime, err := openStore(ctx, databaseURL, open)
	if err != nil {
		return err
	}
	defer runtime.Close()
	reader, ok := runtime.(browsershadow.AttemptReader)
	if !ok {
		return browser.ErrInvalid
	}
	report, err := browsershadow.Compare(ctx, reader, plan, digest, bindings)
	if err != nil {
		return fmt.Errorf("compare browser plan attempts: %w", err)
	}
	output, err := executioncontract.ExportBrowserPlanShadowReport(report)
	if err != nil {
		return err
	}

	return encodeJSON(stdout, output)
}

func runBrowserIngest(ctx context.Context, args []string, databaseURL string, stdin io.Reader, stdout io.Writer, open OpenRuntime) error {
	flags := flag.NewFlagSet("execution-evidence browser-ingest", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	path := flags.String("file", "", "browser attempt path or -")
	if err := commandline.Parse(flags, args); err != nil {
		return err
	}
	if *path == "" || flags.NArg() != 0 {
		return commandline.UsageText("usage: execution-evidence browser-ingest -file <path|->")
	}
	data, err := readEvidenceDocument(*path, stdin, "browser attempt")
	if err != nil {
		return err
	}
	attempt, err := executioncontract.DecodeRecordedBrowserAttempt(data)
	if err != nil {
		return err
	}
	runtime, err := openStore(ctx, databaseURL, open)
	if err != nil {
		return err
	}
	defer runtime.Close()
	store, ok := runtime.(browserAttemptStore)
	if !ok {
		return browser.ErrInvalid
	}
	created, err := store.SaveBrowserAttempt(ctx, attempt)
	if err != nil {
		return fmt.Errorf("ingest browser attempt: %w", err)
	}

	return encodeJSON(stdout, struct {
		AttemptID string `json:"attemptId"`
		Created   bool   `json:"created"`
	}{attempt.Request.AttemptID, created})
}
