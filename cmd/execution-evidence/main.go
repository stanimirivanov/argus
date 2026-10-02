// Command execution-evidence persists attempts and compares selected execution with a full-suite control.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/execution/adapters/cli/evidencecli"
	"github.com/stanimirivanov/argus/internal/postgres"
)

var commandSpec = commandline.Spec{
	Name:     "execution-evidence",
	Synopsis: "execution-evidence <ingest|shadow-report|plan-shadow-report> [options]",
	Role:     "transitional direct-database client",
}

func main() {
	if code, handled := commandline.HandleMeta(os.Args[1:], os.Stdout, os.Stderr, commandSpec); handled {
		os.Exit(code)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	openRuntime := func(ctx context.Context, databaseURL string) (evidencecli.Runtime, error) {
		runtime, err := postgres.OpenRuntime(ctx, databaseURL)
		if err != nil {
			return nil, err
		}

		return &executionRuntime{ExecutionStore: runtime.Execution(), runtime: runtime}, nil
	}
	if err := evidencecli.Run(
		ctx,
		os.Args[1:],
		os.Getenv("ARGUS_DATABASE_URL"),
		os.Stdin,
		os.Stdout,
		openRuntime,
	); err != nil {
		os.Exit(commandline.Report(os.Stderr, err))
	}
}

type executionRuntime struct {
	*postgres.ExecutionStore
	runtime *postgres.Runtime
}

func (runtime *executionRuntime) Close() { runtime.runtime.Close() }
