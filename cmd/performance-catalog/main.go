// Command performance-catalog fetches complete authorized Perfeng metadata for inspection.
package main

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"syscall"

	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/performance/adapters/cataloghttp"
	"github.com/stanimirivanov/argus/internal/performance/adapters/cli/catalogcli"
	"github.com/stanimirivanov/argus/internal/performance/adapters/perfeng"
)

var commandSpec = commandline.Spec{
	Name: "performance-catalog", Role: "read-only operator inspection",
	Synopsis: "performance-catalog -endpoint <URL> -issuer <URI> -principal-issuer <URI> -principal-id <ID> -producer-version <version> [-producer-id perfeng-control-plane]",
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
	if err := catalogcli.Run(ctx, os.Args[1:], os.Getenv("ARGUS_PERFENG_TOKEN"), os.Stdout, fetch); err != nil {
		os.Exit(commandline.Report(os.Stderr, err))
	}
}
