// Command predictive-portfolio schedules an offline budgeted test portfolio.
package main

import (
	"os"

	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/predictive/adapters/replaycli"
)

var commandSpec = commandline.Spec{
	Name:     "predictive-portfolio",
	Synopsis: "predictive-portfolio -index <path> -constraints <path> -budget-ms <milliseconds>",
	Role:     "offline diverse portfolio and feasible serial scheduling",
}

func main() {
	if code, handled := commandline.HandleMeta(os.Args[1:], os.Stdout, os.Stderr, commandSpec); handled {
		os.Exit(code)
	}
	if err := replaycli.RunPortfolio(os.Args[1:], os.Stdout); err != nil {
		os.Exit(commandline.Report(os.Stderr, err))
	}
}
