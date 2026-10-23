// Command performance-catalog fetches complete authorized Perfeng metadata for inspection.
package main

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/performance/adapters/cataloghttp"
	"github.com/stanimirivanov/argus/internal/performance/adapters/cli/catalogcli"
	"github.com/stanimirivanov/argus/internal/performance/adapters/perfeng"
	"github.com/stanimirivanov/argus/internal/performance/catalogsync"
	"github.com/stanimirivanov/argus/internal/postgres"
)

var commandSpec = commandline.Spec{
	Name: "performance-catalog", Role: "operator inspection and opt-in atomic publication",
	Synopsis: "performance-catalog [-publish|-stored] -issuer <URI> -principal-issuer <URI> -principal-id <ID> [-endpoint <URL> -producer-version <version>] [-producer-id perfeng-control-plane]",
}

func main() {
	if code, handled := commandline.HandleMeta(os.Args[1:], os.Stdout, os.Stderr, commandSpec); handled {
		os.Exit(code)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	fetch := func(ctx context.Context, endpoint, token string, expected perfeng.Expectations) ([]json.RawMessage, error) {
		return (cataloghttp.Source{Endpoint: endpoint, Token: token, Expected: expected}).Fetch(ctx)
	}
	storage := catalogcli.Storage{
		Publish: publish,
		Current: func(ctx context.Context, scope catalogsync.Scope) ([]json.RawMessage, error) {
			runtime, err := postgres.OpenRuntime(ctx, os.Getenv("ARGUS_DATABASE_URL"))
			if err != nil {
				return nil, err
			}
			defer runtime.Close()
			snapshot, err := runtime.Performance().Current(ctx, scope)
			if err != nil {
				return nil, err
			}

			return pages(snapshot)
		},
	}
	if err := catalogcli.RunWithStorage(ctx, os.Args[1:], os.Getenv("ARGUS_PERFENG_TOKEN"), os.Stdout, fetch, storage); err != nil {
		os.Exit(commandline.Report(os.Stderr, err))
	}
}

func publish(ctx context.Context, source []json.RawMessage, expected perfeng.Expectations) (catalogcli.Publication, error) {
	snapshot, err := perfeng.Import(source, expected, time.Now())
	if err != nil {
		return catalogcli.Publication{}, err
	}
	runtime, err := postgres.OpenRuntime(ctx, os.Getenv("ARGUS_DATABASE_URL"))
	if err != nil {
		return catalogcli.Publication{}, err
	}
	defer runtime.Close()
	result, err := catalogsync.Publish(ctx, runtime.Performance(), snapshot)
	if err != nil {
		return catalogcli.Publication{}, err
	}
	documents, err := pages(result.Snapshot)

	return catalogcli.Publication{Created: result.Created, Pages: documents}, err
}

func pages(snapshot catalogsync.Snapshot) ([]json.RawMessage, error) {
	var documents []json.RawMessage
	err := json.Unmarshal(snapshot.Evidence().Pages, &documents)
	return documents, err
}
