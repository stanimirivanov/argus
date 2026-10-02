// Command migrate applies Argus's embedded PostgreSQL migrations.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/postgres"
)

const databaseURLEnvironment = "ARGUS_DATABASE_URL"

var commandSpec = commandline.Spec{
	Name:     "migrate",
	Synopsis: "migrate",
	Role:     "administrator",
}

func main() {
	if code, handled := commandline.HandleMeta(os.Args[1:], os.Stdout, os.Stderr, commandSpec); handled {
		os.Exit(code)
	}
	if len(os.Args) != 1 {
		os.Exit(commandline.Report(os.Stderr, commandline.UsageText("usage: migrate")))
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Getenv(databaseURLEnvironment), os.Stdout); err != nil {
		os.Exit(commandline.Report(os.Stderr, err))
	}
}

func run(ctx context.Context, databaseURL string, output io.Writer) error {
	if databaseURL == "" {
		return errors.New("ARGUS_DATABASE_URL is required")
	}

	migrator, err := postgres.OpenMigrator(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("open catalog migrator: %w", err)
	}
	defer migrator.Close()

	if err := migrator.Migrate(ctx); err != nil {
		return fmt.Errorf("migrate catalog: %w", err)
	}
	if _, err := fmt.Fprintln(output, "catalog migrations applied"); err != nil {
		return fmt.Errorf("write migration result: %w", err)
	}

	return nil
}
