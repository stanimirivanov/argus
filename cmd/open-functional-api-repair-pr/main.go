// Command open-functional-api-repair-pr publishes one validated repair as a draft GitHub pull request.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/stanimirivanov/argus/internal/adaptation/adapters/cli/reviewcli"
	"github.com/stanimirivanov/argus/internal/adaptation/adapters/githubreview"
	"github.com/stanimirivanov/argus/internal/adaptation/review"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, arguments []string) error {
	return reviewcli.Run(
		ctx, arguments, os.Stdout, os.Getenv,
		func(apiURL, host, token string) (review.Gateway, error) {
			return githubreview.NewClient(githubreview.ClientOptions{
				BaseURL: apiURL, Host: host, Token: token,
			})
		},
	)
}
