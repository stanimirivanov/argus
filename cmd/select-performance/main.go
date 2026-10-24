// Command select-performance inspects immutable pins or recommends reviewed targets.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/performance"
	"github.com/stanimirivanov/argus/internal/performance/adapters/cli/selectioncli"
	"github.com/stanimirivanov/argus/internal/performance/adapters/selectionio"
	"github.com/stanimirivanov/argus/internal/performance/catalogsync"
	"github.com/stanimirivanov/argus/internal/postgres"
)

var commandSpec = commandline.Spec{Name: "select-performance", Role: "opt-in owner-reviewed performance workload selection", Synopsis: "select-performance -delivery-id <ID> (-inspect|-policy <file>) -issuer <URI> -principal-issuer <URI> -principal-id <ID> -producer-version <version> [-producer-id perfeng-control-plane]"}

func main() {
	if code, handled := commandline.HandleMeta(os.Args[1:], os.Stdout, os.Stderr, commandSpec); handled {
		os.Exit(code)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	evaluate := func(ctx context.Context, request selectioncli.Request) ([]byte, error) {
		runtime, err := postgres.OpenRuntime(ctx, os.Getenv("ARGUS_DATABASE_URL"))
		if err != nil {
			return nil, errors.New("cannot open stored performance evidence")
		}
		defer runtime.Close()

		return evaluateStored(ctx, request, runtime.Change().FindCapabilityImpact, runtime.Performance().Current)
	}
	if err := selectioncli.Run(ctx, os.Args[1:], os.Stdout, evaluate, time.Now); err != nil {
		os.Exit(commandline.Report(os.Stderr, err))
	}
}

// evaluateStored uses only the requested delivery and independently configured
// catalog scope. Immutable reads need no cross-module write transaction.
func evaluateStored(ctx context.Context, request selectioncli.Request,
	find func(context.Context, catalog.Provider, string) (change.CapabilityImpact, error),
	current func(context.Context, catalogsync.Scope) (catalogsync.Snapshot, error),
) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	impact, err := find(ctx, catalog.ProviderGitHub, request.DeliveryID)
	if err != nil {
		return nil, errors.New("cannot read requested stored impact")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if impact.Change.Trigger.Provider != catalog.ProviderGitHub || impact.Change.Trigger.DeliveryID != request.DeliveryID {
		return nil, performance.ErrInvalid
	}
	expected := request.Expected
	snapshot, err := current(ctx, catalogsync.Scope{Issuer: expected.Issuer, PrincipalIssuer: expected.PrincipalIssuer, PrincipalID: expected.PrincipalID})
	if err != nil {
		return nil, errors.New("cannot read authorized stored catalog")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	evidence, err := selectionio.Inspect(impact, snapshot, expected)
	if err != nil {
		return nil, err
	}
	var document any
	if request.Inspect {
		document, err = selectionio.Context(evidence)
	} else {
		document, err = selectionio.Evaluate(evidence, request.Policy, request.Now())
	}
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return json.Marshal(document)
}
