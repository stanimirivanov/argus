// Command plan-browser creates reviewed selected and mandatory full-suite browser jobs.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/execution/adapters/cli/planningcli"
)

var commandSpec = commandline.Spec{Name: "plan-browser", Synopsis: "plan-browser -manifest <path|-> -bindings <path|->", Role: "CI worker"}

func main() {
	if code, handled := commandline.HandleMeta(os.Args[1:], os.Stdout, os.Stderr, commandSpec); handled {
		os.Exit(code)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := planningcli.RunBrowser(ctx, os.Args[1:], os.Stdin, os.Stdout); err != nil {
		os.Exit(commandline.Report(os.Stderr, err))
	}
}
