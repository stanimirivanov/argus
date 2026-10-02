// Command run-functional-api executes one manifest group through a CI-local adapter.
package main

import (
	"context"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/execution/adapters/cli/executioncli"
	"github.com/stanimirivanov/argus/internal/execution/adapters/processadapter"
	"github.com/stanimirivanov/argus/internal/execution/functionalapi"
)

var commandSpec = commandline.Spec{
	Name:     "run-functional-api",
	Synopsis: "run-functional-api -manifest <path|-> -attempt-id <id> [options] -- <adapter-command>",
	Role:     "CI worker",
}

func main() {
	if code, handled := commandline.HandleMeta(os.Args[1:], os.Stdout, os.Stderr, commandSpec); handled {
		os.Exit(code)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		os.Exit(commandline.Report(os.Stderr, err))
	}
}

func run(ctx context.Context, arguments []string) error {
	return executioncli.Run(
		ctx, arguments, os.Stdin, os.Stdout, os.Stderr,
		func(command []string, stderrWriter io.Writer) (functionalapi.Adapter, error) {
			return processadapter.New(command, stderrWriter)
		},
	)
}
