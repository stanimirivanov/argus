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
)

type browserAttemptStore interface {
	SaveBrowserAttempt(context.Context, browser.Attempt) (bool, error)
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
	document, err := contracts.DecodeBrowserExecutionAttemptV1(data)
	if err != nil {
		return err
	}
	attempt, err := executioncontract.ImportBrowserAttempt(document)
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
