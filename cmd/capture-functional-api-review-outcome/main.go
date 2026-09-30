// Command capture-functional-api-review-outcome emits terminal human-review evidence.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/stanimirivanov/argus/internal/adaptation/adapters/cli/outcomecli"
	"github.com/stanimirivanov/argus/internal/adaptation/adapters/githubreview"
	"github.com/stanimirivanov/argus/internal/adaptation/outcome"
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
	return outcomecli.Run(
		ctx, arguments, os.Stdout, os.Getenv,
		func(apiURL, host, token string) (outcome.Gateway, error) {
			return githubreview.NewClient(githubreview.ClientOptions{
				BaseURL: apiURL, Host: host, Token: token,
			})
		},
	)
}
