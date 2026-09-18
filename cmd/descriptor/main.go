// Command descriptor validates and normalizes an Argus repository descriptor.
package main

import (
	"fmt"
	"os"

	"github.com/stanimirivanov/argus/internal/catalog/adapters/cli/descriptorcli"
)

func main() {
	if err := descriptorcli.Run(os.Args[1:], os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
