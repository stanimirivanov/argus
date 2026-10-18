// Command run-browser executes one browser manifest group through a CI-local adapter.
package main

import (
	"context"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/execution/adapters/cli/browsercli"
	"github.com/stanimirivanov/argus/internal/execution/adapters/processadapter"
	"github.com/stanimirivanov/argus/internal/execution/browser"
)

var commandSpec = commandline.Spec{Name: "run-browser", Synopsis: "run-browser -manifest <path|-> -attempt-id <id> -project <id> -environment <id> [options] -- <adapter-command>", Role: "CI worker"}

func main() {
	if code, handled := commandline.HandleMeta(os.Args[1:], os.Stdout, os.Stderr, commandSpec); handled {
		os.Exit(code)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := browsercli.Run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr, func(command []string, writer io.Writer) (browser.Adapter, error) {
		return processadapter.NewBrowser(command, writer)
	}); err != nil {
		os.Exit(commandline.Report(os.Stderr, err))
	}
}
