// Command capture-functional-api-review-outcome emits terminal human-review evidence.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/stanimirivanov/argus/internal/adaptation/adapters/cli/outcomecli"
	"github.com/stanimirivanov/argus/internal/adaptation/adapters/githubreview"
	"github.com/stanimirivanov/argus/internal/adaptation/outcome"
	"github.com/stanimirivanov/argus/internal/commandline"
)

var commandSpec = commandline.Spec{
	Name:     "capture-functional-api-review-outcome",
	Synopsis: "capture-functional-api-review-outcome -proposal <path> -validation <path> -review <path> [options]",
	Role:     "CI review observer",
}

func main() {
	if code, handled := commandline.HandleMeta(os.Args[1:], os.Stdout, os.Stderr, commandSpec); handled {
		os.Exit(code)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		os.Exit(commandline.Report(os.Stderr, err))
	}
}

func run(ctx context.Context, arguments []string) error {
	return outcomecli.Run(
		ctx, arguments, os.Stdout, os.Getenv,
		func(apiURL, host, token string) (outcome.Gateway, error) {
			return githubreview.NewOutcomeObserver(githubreview.ClientOptions{
				BaseURL: apiURL, Host: host, Token: token,
			})
		},
	)
}
