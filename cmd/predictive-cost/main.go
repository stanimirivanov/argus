// Command predictive-cost estimates offline stage costs from reviewed constraints.
package main

import (
	"os"

	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/predictive/adapters/replaycli"
)

var commandSpec = commandline.Spec{
	Name:     "predictive-cost",
	Synopsis: "predictive-cost -index <path> -constraints <path>",
	Role:     "offline stage cost and reviewed constraint validation",
}

func main() {
	if code, handled := commandline.HandleMeta(os.Args[1:], os.Stdout, os.Stderr, commandSpec); handled {
		os.Exit(code)
	}
	if err := replaycli.RunCost(os.Args[1:], os.Stdout); err != nil {
		os.Exit(commandline.Report(os.Stderr, err))
	}
}
