// Command predictive-safety reports explicit staged execution evidence.
package main

import (
	"os"

	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/predictive/adapters/cli"
)

var commandSpec = commandline.Spec{Name: "predictive-safety", Synopsis: "predictive-safety -index <path>", Role: "observational staged safety reporting"}

func main() {
	if code, handled := commandline.HandleMeta(os.Args[1:], os.Stdout, os.Stderr, commandSpec); handled {
		os.Exit(code)
	}
	if err := cli.RunSafety(os.Args[1:], os.Stdout); err != nil {
		os.Exit(commandline.Report(os.Stderr, err))
	}
}
