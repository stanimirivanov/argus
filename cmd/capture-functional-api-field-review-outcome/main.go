// Command capture-functional-api-field-review-outcome records terminal field repair review evidence.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/stanimirivanov/argus/internal/adaptation/adapters/cli/fieldreviewcli"
	"github.com/stanimirivanov/argus/internal/adaptation/adapters/githubreview"
	"github.com/stanimirivanov/argus/internal/adaptation/outcome"
	"github.com/stanimirivanov/argus/internal/commandline"
)

var commandSpec = commandline.Spec{Name: "capture-functional-api-field-review-outcome", Synopsis: "capture-functional-api-field-review-outcome -proposal <path> -validation-evidence <path> -review <path> -reason-code <code>", Role: "CI review observer"}

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
func run(ctx context.Context, args []string) error {
	return fieldreviewcli.Capture(ctx, args, os.Stdout, os.Getenv,
		func(apiURL, host, token string) (outcome.Gateway, error) {
			return githubreview.NewOutcomeObserver(githubreview.ClientOptions{BaseURL: apiURL, Host: host, Token: token})
		})
}
