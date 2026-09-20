// Command run-functional-api executes one manifest group through a CI-local adapter.
package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/stanimirivanov/argus/internal/execution/adapters/cli/executioncli"
	"github.com/stanimirivanov/argus/internal/execution/adapters/processadapter"
	"github.com/stanimirivanov/argus/internal/execution/functionalapi"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, arguments []string) error {
	return executioncli.Run(
		ctx, arguments, os.Stdin, os.Stdout, os.Stderr,
		func(command []string, stderrWriter io.Writer) (functionalapi.Adapter, error) {
			return processadapter.New(command, stderrWriter)
		},
	)
}
