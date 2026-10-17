// Command open-functional-api-field-repair-pr publishes one proven field edit as a draft.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/stanimirivanov/argus/internal/adaptation/adapters/cli/fieldreviewcli"
	"github.com/stanimirivanov/argus/internal/adaptation/adapters/githubreview"
	"github.com/stanimirivanov/argus/internal/adaptation/review"
	"github.com/stanimirivanov/argus/internal/commandline"
)

var commandSpec = commandline.Spec{Name: "open-functional-api-field-repair-pr", Synopsis: "open-functional-api-field-repair-pr -proposal <path> -validation-evidence <path> -base-branch <branch>", Role: "CI review publisher"}

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
	return fieldreviewcli.Publish(ctx, args, os.Stdout, os.Getenv,
		func(apiURL, host, readToken, writeToken string) (review.SourceReader, review.Publisher, error) {
			source, err := githubreview.NewSourceReader(githubreview.ClientOptions{BaseURL: apiURL, Host: host, Token: readToken})
			if err != nil {
				return nil, nil, err
			}
			publisher, err := githubreview.NewPublisher(githubreview.ClientOptions{BaseURL: apiURL, Host: host, Token: writeToken})
			if err != nil {
				return nil, nil, err
			}

			return source, publisher, nil
		})
}
