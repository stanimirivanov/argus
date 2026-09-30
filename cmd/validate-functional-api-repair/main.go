// Command validate-functional-api-repair proves one proposal in a disposable checkout.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/stanimirivanov/argus/internal/adaptation/adapters/cli/validationcli"
	"github.com/stanimirivanov/argus/internal/adaptation/adapters/fileworkspace"
	"github.com/stanimirivanov/argus/internal/adaptation/adapters/validationprocessadapter"
	"github.com/stanimirivanov/argus/internal/adaptation/validation"
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
	return validationcli.Run(
		ctx, arguments, os.Stdout, os.Stderr,
		func(
			root string,
			command []string,
			stderr io.Writer,
		) (validation.Workspace, validation.Runner, error) {
			workspace, err := fileworkspace.Open(root)
			if err != nil {
				return nil, nil, err
			}
			runner, err := validationprocessadapter.New(command, workspace.Root(), stderr)
			if err != nil {
				return nil, nil, err
			}

			return workspace, runner, nil
		},
	)
}
