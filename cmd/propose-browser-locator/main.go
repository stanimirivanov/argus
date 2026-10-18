// Command propose-browser-locator emits a constrained browser repair candidate or abstention.
package main

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"syscall"

	"github.com/stanimirivanov/argus/internal/adaptation/adapters/cli/browserlocatorcli"
	adaptationcontract "github.com/stanimirivanov/argus/internal/adaptation/adapters/contract"
	"github.com/stanimirivanov/argus/internal/adaptation/browserlocator"
	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/contracts"
	executioncontract "github.com/stanimirivanov/argus/internal/execution/adapters/contract"
	"github.com/stanimirivanov/argus/internal/execution/browser"
)

var commandSpec = commandline.Spec{Name: "propose-browser-locator", Synopsis: "propose-browser-locator -baseline <path> -failed <path> -test-checkout <dir> -artifact-root <dir> -registry <path> -source-path <path> -suite-key <key> -test-key <key>", Role: "CI worker"}

func main() {
	if code, handled := commandline.HandleMeta(os.Args[1:], os.Stdout, os.Stderr, commandSpec); handled {
		os.Exit(code)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := browserlocatorcli.Run(ctx, os.Args[1:], os.Stdout, importAttempt, exportDiagnosis); err != nil {
		os.Exit(commandline.Report(os.Stderr, err))
	}
}

func importAttempt(data []byte) (browser.Attempt, error) {
	document, err := contracts.DecodeBrowserExecutionAttemptV1(data)
	if err != nil {
		return browser.Attempt{}, err
	}

	return executioncontract.ImportBrowserAttempt(document)
}

func exportDiagnosis(value browserlocator.Diagnosis) ([]byte, error) {
	document, err := adaptationcontract.ExportBrowserLocatorDiagnosisV1(value)
	if err != nil {
		return nil, err
	}

	return json.MarshalIndent(document, "", "  ")
}
