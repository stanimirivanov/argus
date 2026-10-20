// Command open-browser-locator-repair-pr publishes one validated locator edit as a draft.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/stanimirivanov/argus/internal/adaptation/adapters/cli/browserreviewcli"
	"github.com/stanimirivanov/argus/internal/adaptation/adapters/githubreview"
	"github.com/stanimirivanov/argus/internal/adaptation/review"
	"github.com/stanimirivanov/argus/internal/commandline"
)

var commandSpec = commandline.Spec{Name: "open-browser-locator-repair-pr", Synopsis: "open-browser-locator-repair-pr -validation-evidence <path> -base-branch <branch>", Role: "CI review publisher"}

func main() {
	if code, handled := commandline.HandleMeta(os.Args[1:], os.Stdout, os.Stderr, commandSpec); handled {
		os.Exit(code)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := browserreviewcli.Publish(ctx, os.Args[1:], os.Stdout, os.Getenv,
		func(apiURL, host, readToken, writeToken string) (review.SourceReader, review.Publisher, error) {
			source, err := githubreview.NewSourceReader(githubreview.ClientOptions{BaseURL: apiURL, Host: host, Token: readToken})
			if err != nil {
				return nil, nil, err
			}
			publisher, err := githubreview.NewPublisher(githubreview.ClientOptions{BaseURL: apiURL, Host: host, Token: writeToken})

			return source, publisher, err
		}); err != nil {
		os.Exit(commandline.Report(os.Stderr, err))
	}
}
