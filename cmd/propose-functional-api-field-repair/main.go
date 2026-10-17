// Command propose-functional-api-field-repair emits one compatible field candidate.
package main

import (
	"context"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/stanimirivanov/argus/internal/adaptation/adapters/cli/proposalcli"
	"github.com/stanimirivanov/argus/internal/adaptation/adapters/gitfieldsource"
	"github.com/stanimirivanov/argus/internal/adaptation/adapters/processadapter"
	"github.com/stanimirivanov/argus/internal/adaptation/fieldrepair"
	"github.com/stanimirivanov/argus/internal/change"
	"github.com/stanimirivanov/argus/internal/change/adapters/openapi"
	"github.com/stanimirivanov/argus/internal/commandline"
)

var commandSpec = commandline.Spec{
	Name:     "propose-functional-api-field-repair",
	Synopsis: "propose-functional-api-field-repair -impact <path> -manifest <path> -source-checkout <path> [options] -- <adapter-command>",
	Role:     "CI worker",
}

func main() {
	if code, handled := commandline.HandleMeta(os.Args[1:], os.Stdout, os.Stderr, commandSpec); handled {
		os.Exit(code)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := proposalcli.RunField(ctx, os.Args[1:], os.Stdout, os.Stderr,
		func(command []string, stderr io.Writer) (fieldrepair.Adapter, error) {
			return processadapter.New(command, stderr)
		}, func(directory string) (proposalcli.FieldSource, error) {
			return gitfieldsource.New(directory)
		}, func(base, head []byte, path, method, route string) (change.FieldRename, bool) {
			proof, err := openapi.DetectCompatibleFieldRename(base, head, path, method, route)
			return proof, err == nil
		}); err != nil {
		os.Exit(commandline.Report(os.Stderr, err))
	}
}
