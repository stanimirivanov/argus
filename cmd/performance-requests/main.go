// Command performance-requests inspects scoped immutable request and pending evidence.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/performance/adapters/cli/requestcli"
	"github.com/stanimirivanov/argus/internal/postgres"
)

var commandSpec = commandline.Spec{Name: "performance-requests", Role: "opt-in scoped performance request evidence inspection", Synopsis: "performance-requests (-review-sha256 <digest>|-pending [-after N] [-limit 1..100]) -issuer <URI> -principal-issuer <URI> -principal-id <ID>"}

func main() {
	if code, handled := commandline.HandleMeta(os.Args[1:], os.Stdout, os.Stderr, commandSpec); handled {
		os.Exit(code)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	open := func(ctx context.Context) (requestcli.Reader, func(), error) {
		runtime, err := postgres.OpenRuntime(ctx, os.Getenv("ARGUS_DATABASE_URL"))
		if err != nil {
			return requestcli.Reader{}, nil, err
		}
		store := runtime.Performance()

		return requestcli.Reader{Get: store.GetRequest, Pending: store.PendingRequests}, runtime.Close, nil
	}
	if err := requestcli.Run(ctx, os.Args[1:], os.Stdout, open); err != nil {
		os.Exit(commandline.Report(os.Stderr, err))
	}
}
