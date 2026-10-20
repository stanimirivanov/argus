// Command predictive-baselines compares deterministic policies on held-out evidence.
package main

import (
	"os"

	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/predictive/adapters/replaycli"
)

var commandSpec = commandline.Spec{
	Name:     "predictive-baselines",
	Synopsis: "predictive-baselines -index <path> -budget-ms <milliseconds> -seed <uint64>",
	Role:     "offline deterministic baseline comparison",
}

func main() {
	if code, handled := commandline.HandleMeta(os.Args[1:], os.Stdout, os.Stderr, commandSpec); handled {
		os.Exit(code)
	}
	if err := replaycli.RunBaselines(os.Args[1:], os.Stdout); err != nil {
		os.Exit(commandline.Report(os.Stderr, err))
	}
}
