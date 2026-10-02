// Command select generates an explainable functional API execution manifest.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/postgres"
	"github.com/stanimirivanov/argus/internal/selection/adapters/cli/selectioncli"
)

var commandSpec = commandline.Spec{
	Name:     "select",
	Synopsis: "select [-provider github] -delivery-id <id>",
	Role:     "transitional direct-database client",
}

func main() {
	if code, handled := commandline.HandleMeta(os.Args[1:], os.Stdout, os.Stderr, commandSpec); handled {
		os.Exit(code)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	openRuntime := func(ctx context.Context, databaseURL string) (selectioncli.Runtime, error) {
		runtime, err := postgres.OpenRuntime(ctx, databaseURL)
		if err != nil {
			return nil, err
		}

		return &selectionRuntime{CatalogStore: runtime.Catalog(), ChangeStore: runtime.Change(), runtime: runtime}, nil
	}
	if err := selectioncli.Run(
		ctx,
		os.Args[1:],
		os.Getenv("ARGUS_DATABASE_URL"),
		os.Stdout,
		openRuntime,
	); err != nil {
		os.Exit(commandline.Report(os.Stderr, err))
	}
}

type selectionRuntime struct {
	*postgres.CatalogStore
	*postgres.ChangeStore
	runtime *postgres.Runtime
}

func (runtime *selectionRuntime) Close() { runtime.runtime.Close() }
