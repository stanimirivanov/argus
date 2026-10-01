// Command execution-evidence persists attempts and compares selected execution with a full-suite control.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/stanimirivanov/argus/internal/execution/adapters/cli/evidencecli"
	"github.com/stanimirivanov/argus/internal/postgres"
)

func main() {
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
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type executionRuntime struct {
	*postgres.ExecutionStore
	runtime *postgres.Runtime
}

func (runtime *executionRuntime) Close() { runtime.runtime.Close() }
