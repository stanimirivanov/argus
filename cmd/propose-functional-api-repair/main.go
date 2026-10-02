// Command propose-functional-api-repair emits one constrained review candidate.
package main

import (
	"context"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/stanimirivanov/argus/internal/adaptation/adapters/cli/proposalcli"
	"github.com/stanimirivanov/argus/internal/adaptation/adapters/processadapter"
	"github.com/stanimirivanov/argus/internal/adaptation/functionalapi"
	"github.com/stanimirivanov/argus/internal/commandline"
)

var commandSpec = commandline.Spec{
	Name:     "propose-functional-api-repair",
	Synopsis: "propose-functional-api-repair -impact <path> -manifest <path> [options] -- <adapter-command>",
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
	return proposalcli.Run(
		ctx, arguments, os.Stdout, os.Stderr,
		func(command []string, stderr io.Writer) (functionalapi.Adapter, error) {
			return processadapter.New(command, stderr)
		},
	)
}
