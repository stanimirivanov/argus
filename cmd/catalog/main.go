// Command catalog imports and reads immutable Argus repository catalog snapshots.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/stanimirivanov/argus/internal/catalog/adapters/cli/catalogcli"
	"github.com/stanimirivanov/argus/internal/catalog/adapters/postgres"
)

const databaseURLEnvironment = "ARGUS_DATABASE_URL"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	openRuntime := func(ctx context.Context, databaseURL string) (catalogcli.Runtime, error) {
		return postgres.OpenStore(ctx, databaseURL)
	}
	if err := catalogcli.Run(
		ctx,
		os.Args[1:],
		os.Getenv(databaseURLEnvironment),
		os.Stdout,
		openRuntime,
	); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
