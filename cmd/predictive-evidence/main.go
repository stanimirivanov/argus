// Command predictive-evidence captures immutable offline evaluation observations.
package main

import (
	"os"

	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/predictive/adapters/cli"
)

var commandSpec = commandline.Spec{
	Name:     "predictive-evidence",
	Synopsis: "predictive-evidence -change <path> -manifest <path> -control <path>",
	Role:     "offline observation capture",
}

func main() {
	if code, handled := commandline.HandleMeta(os.Args[1:], os.Stdout, os.Stderr, commandSpec); handled {
		os.Exit(code)
	}
	if err := cli.Run(os.Args[1:], os.Stdout); err != nil {
		os.Exit(commandline.Report(os.Stderr, err))
	}
}
