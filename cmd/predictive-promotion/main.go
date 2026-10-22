// Command predictive-promotion gates staged mode on reviewed owner policy.
package main

import (
	"os"

	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/predictive/adapters/cli"
	"github.com/stanimirivanov/argus/internal/predictive/adapters/replaycli"
)

var commandSpec = commandline.Spec{Name: "predictive-promotion", Synopsis: "predictive-promotion -index <path> -constraints <path> -budget-ms <ms> -shadow-index <path> -policy <path> -as-of <timestamp>", Role: "owner-policy staged promotion gate"}

func main() {
	if code, handled := commandline.HandleMeta(os.Args[1:], os.Stdout, os.Stderr, commandSpec); handled {
		os.Exit(code)
	}
	if err := cli.RunPromotion(os.Args[1:], os.Stdout, replaycli.LoadPromotionReplay); err != nil {
		os.Exit(commandline.Report(os.Stderr, err))
	}
}
