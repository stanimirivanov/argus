// Command playwright-catalog checks declared UI tests against Playwright discovery.
package main

import (
	"os"

	"github.com/stanimirivanov/argus/internal/catalog/adapters/cli/playwrightcli"
	"github.com/stanimirivanov/argus/internal/commandline"
)

var commandSpec = commandline.Spec{
	Name:     "playwright-catalog",
	Synopsis: "playwright-catalog -source-revision <sha> -suite <key> <descriptor.json> <playwright-list.json>",
	Role:     "CI/local validator",
}

func main() {
	if code, handled := commandline.HandleMeta(os.Args[1:], os.Stdout, os.Stderr, commandSpec); handled {
		os.Exit(code)
	}
	if err := playwrightcli.Run(os.Args[1:], os.Stdout); err != nil {
		os.Exit(commandline.Report(os.Stderr, err))
	}
}
