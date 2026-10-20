// Command capture-browser-locator-review-outcome records terminal human review.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/stanimirivanov/argus/internal/adaptation/adapters/cli/browserreviewcli"
	"github.com/stanimirivanov/argus/internal/adaptation/adapters/githubreview"
	"github.com/stanimirivanov/argus/internal/adaptation/outcome"
	"github.com/stanimirivanov/argus/internal/adaptation/review"
	"github.com/stanimirivanov/argus/internal/commandline"
)

var commandSpec = commandline.Spec{Name: "capture-browser-locator-review-outcome", Synopsis: "capture-browser-locator-review-outcome -validation-evidence <path> -review <path> -reason-code <code>", Role: "CI review observer"}

func main() {
	if code, handled := commandline.HandleMeta(os.Args[1:], os.Stdout, os.Stderr, commandSpec); handled {
		os.Exit(code)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := browserreviewcli.Capture(ctx, os.Args[1:], os.Stdout, os.Getenv,
		func(apiURL, host, token string) (outcome.Gateway, review.SourceReader, error) {
			options := githubreview.ClientOptions{BaseURL: apiURL, Host: host, Token: token}
			observer, err := githubreview.NewOutcomeObserver(options)
			if err != nil {
				return nil, nil, err
			}
			source, err := githubreview.NewSourceReader(options)

			return observer, source, err
		}); err != nil {
		os.Exit(commandline.Report(os.Stderr, err))
	}
}
