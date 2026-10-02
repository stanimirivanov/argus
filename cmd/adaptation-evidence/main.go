// Command adaptation-evidence persists and retrieves bounded adaptation evidence.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/stanimirivanov/argus/internal/adaptation/adapters/cli/evidencecli"
	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/postgres"
)

var commandSpec = commandline.Spec{
	Name:     "adaptation-evidence",
	Synopsis: "adaptation-evidence <ingest|get|ingest-validation-rejection|get-validation-rejection> [options]",
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

		return &adaptationRuntime{AdaptationStore: runtime.Adaptation(), runtime: runtime}, nil
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

type adaptationRuntime struct {
	*postgres.AdaptationStore
	runtime *postgres.Runtime
}

func (runtime *adaptationRuntime) Close() { runtime.runtime.Close() }
