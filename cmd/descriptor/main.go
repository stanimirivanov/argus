// Command descriptor validates and normalizes an Argus repository descriptor.
package main

import (
	"os"

	"github.com/stanimirivanov/argus/internal/catalog/adapters/cli/descriptorcli"
	"github.com/stanimirivanov/argus/internal/commandline"
)

var commandSpec = commandline.Spec{
	Name:     "descriptor",
	Synopsis: "descriptor -revision <digest> [-algorithm git-sha1] <descriptor.json>",
	Role:     "CI/local validator",
}

func main() {
	if code, handled := commandline.HandleMeta(os.Args[1:], os.Stdout, os.Stderr, commandSpec); handled {
		os.Exit(code)
	}
	if err := descriptorcli.Run(os.Args[1:], os.Stdout); err != nil {
		os.Exit(commandline.Report(os.Stderr, err))
	}
}
