// Command plan-functional-api creates deterministic heterogeneous CI matrix jobs.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/execution/adapters/cli/planningcli"
)

var commandSpec = commandline.Spec{
	Name:     "plan-functional-api",
	Synopsis: "plan-functional-api -manifest <path|-> -bindings <path|->",
	Role:     "CI worker",
}

func main() {
	if code, handled := commandline.HandleMeta(os.Args[1:], os.Stdout, os.Stderr, commandSpec); handled {
		os.Exit(code)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := planningcli.Run(ctx, os.Args[1:], os.Stdin, os.Stdout); err != nil {
		os.Exit(commandline.Report(os.Stderr, err))
	}
}
