// Command predictive-abstention evaluates offline evidence safety and fallback.
package main

import (
	"os"

	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/predictive/adapters/replaycli"
)

var commandSpec = commandline.Spec{
	Name:     "predictive-abstention",
	Synopsis: "predictive-abstention -index <path>",
	Role:     "offline predictive safety and full-control fallback evaluation",
}

func main() {
	if code, handled := commandline.HandleMeta(os.Args[1:], os.Stdout, os.Stderr, commandSpec); handled {
		os.Exit(code)
	}
	if err := replaycli.RunAbstention(os.Args[1:], os.Stdout); err != nil {
		os.Exit(commandline.Report(os.Stderr, err))
	}
}
