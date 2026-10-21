// Command predictive-calibration evaluates repository and family probabilities offline.
package main

import (
	"os"

	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/predictive/adapters/replaycli"
)

var commandSpec = commandline.Spec{
	Name:     "predictive-calibration",
	Synopsis: "predictive-calibration -index <path>",
	Role:     "offline repository and family calibration evaluation",
}

func main() {
	if code, handled := commandline.HandleMeta(os.Args[1:], os.Stdout, os.Stderr, commandSpec); handled {
		os.Exit(code)
	}
	if err := replaycli.RunCalibration(os.Args[1:], os.Stdout); err != nil {
		os.Exit(commandline.Report(os.Stderr, err))
	}
}
