// Command propose-functional-api-repair emits one constrained review candidate.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/stanimirivanov/argus/internal/adaptation/adapters/cli/proposalcli"
	"github.com/stanimirivanov/argus/internal/adaptation/adapters/processadapter"
	"github.com/stanimirivanov/argus/internal/adaptation/functionalapi"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, arguments []string) error {
	return proposalcli.Run(
		ctx, arguments, os.Stdout, os.Stderr,
		func(command []string, stderr io.Writer) (functionalapi.Adapter, error) {
			return processadapter.New(command, stderr)
		},
	)
}
