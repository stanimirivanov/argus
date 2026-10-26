// Command performance-requests creates reviewed requests or inspects scoped evidence.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/performance/adapters/cli/requestcli"
	"github.com/stanimirivanov/argus/internal/postgres"
)

var commandSpec = commandline.Spec{Name: "performance-requests", Role: "opt-in reviewed performance request creation and scoped inspection", Synopsis: "performance-requests (-review-sha256 <digest>|-pending [-after N] [-limit 1..100]) -issuer <URI> -principal-issuer <URI> -principal-id <ID>; performance-requests -create -delivery-id <ID> -policy <file> -review <file>"}

func main() {
	if code, handled := commandline.HandleMeta(os.Args[1:], os.Stdout, os.Stderr, commandSpec); handled {
		os.Exit(code)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if len(os.Args) > 1 && os.Args[1] == "-create" {
		if err := requestcli.RunCreate(ctx, os.Args[2:], os.Stdout, createOperation, time.Now); err != nil {
			os.Exit(commandline.Report(os.Stderr, err))
		}
		return
	}
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
