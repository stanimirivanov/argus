// Command adaptation-evidence persists and retrieves bounded adaptation evidence.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/stanimirivanov/argus/internal/adaptation/adapters/cli/evidencecli"
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
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type adaptationRuntime struct {
	*postgres.AdaptationStore
	runtime *postgres.Runtime
}

func (runtime *adaptationRuntime) Close() { runtime.runtime.Close() }
