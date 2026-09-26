// Command plan-functional-api creates deterministic heterogeneous CI matrix jobs.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/stanimirivanov/argus/internal/execution/adapters/cli/planningcli"
)

func main() {
	if err := planningcli.Run(context.Background(), os.Args[1:], os.Stdin, os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
