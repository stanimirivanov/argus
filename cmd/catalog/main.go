// Command catalog imports and reads immutable Argus repository catalog snapshots.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/stanimirivanov/argus/internal/catalog/adapters/cli/catalogcli"
	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/postgres"
)

const databaseURLEnvironment = "ARGUS_DATABASE_URL"

var commandSpec = commandline.Spec{
	Name:     "catalog",
	Synopsis: "catalog <import|get|list-tests|import-impact|list-impact|get-change-impact> [options]",
	Role:     "transitional direct-database client",
}

func main() {
	if code, handled := commandline.HandleMeta(os.Args[1:], os.Stdout, os.Stderr, commandSpec); handled {
		os.Exit(code)
	}
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
		os.Exit(commandline.Report(os.Stderr, err))
	}
}

type catalogRuntime struct {
	*postgres.CatalogStore
	*postgres.ChangeStore
	runtime *postgres.Runtime
}

func (runtime *catalogRuntime) Close() { runtime.runtime.Close() }
