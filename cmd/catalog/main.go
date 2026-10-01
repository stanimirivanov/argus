// Command catalog imports and reads immutable Argus repository catalog snapshots.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/stanimirivanov/argus/internal/catalog/adapters/cli/catalogcli"
	"github.com/stanimirivanov/argus/internal/postgres"
)

const databaseURLEnvironment = "ARGUS_DATABASE_URL"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	openRuntime := func(ctx context.Context, databaseURL string) (catalogcli.Runtime, error) {
		runtime, err := postgres.OpenRuntime(ctx, databaseURL)
		if err != nil {
			return nil, err
		}

		return &catalogRuntime{CatalogStore: runtime.Catalog(), ChangeStore: runtime.Change(), runtime: runtime}, nil
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

type catalogRuntime struct {
	*postgres.CatalogStore
	*postgres.ChangeStore
	runtime *postgres.Runtime
}

func (runtime *catalogRuntime) Close() { runtime.runtime.Close() }
