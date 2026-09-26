// Command execution-evidence persists attempts and compares selected execution with a full-suite control.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/stanimirivanov/argus/internal/catalog/adapters/postgres"
	"github.com/stanimirivanov/argus/internal/execution/adapters/cli/evidencecli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	openRuntime := func(ctx context.Context, databaseURL string) (evidencecli.Runtime, error) {
		return postgres.OpenStore(ctx, databaseURL)
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
