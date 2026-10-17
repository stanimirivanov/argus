// Command validate-functional-api-field-repair proves one field proposal in a disposable checkout.
package main

import (
	"context"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/stanimirivanov/argus/internal/adaptation/adapters/cli/fieldvalidationcli"
	"github.com/stanimirivanov/argus/internal/adaptation/adapters/fileworkspace"
	"github.com/stanimirivanov/argus/internal/adaptation/adapters/validationprocessadapter"
	"github.com/stanimirivanov/argus/internal/adaptation/fieldvalidation"
	"github.com/stanimirivanov/argus/internal/commandline"
)

var commandSpec = commandline.Spec{Name: "validate-functional-api-field-repair", Synopsis: "validate-functional-api-field-repair -proposal <path> -disposable-workspace <path> -- <adapter-command>", Role: "CI worker"}

func main() {
	if code, handled := commandline.HandleMeta(os.Args[1:], os.Stdout, os.Stderr, commandSpec); handled {
		os.Exit(code)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		os.Exit(commandline.Report(os.Stderr, err))
	}
}
func run(ctx context.Context, args []string) error {
	return fieldvalidationcli.Run(ctx, args, os.Stdout, os.Stderr,
		func(root string, command []string, stderr io.Writer) (fieldvalidation.Workspace, fieldvalidation.Runner, error) {
			workspace, err := fileworkspace.Open(root)
			if err != nil {
				return nil, nil, err
			}
			runner, err := validationprocessadapter.New(command, workspace.Root(), stderr)
			if err != nil {
				return nil, nil, err
			}

			return workspace, runner, nil
		})
}
