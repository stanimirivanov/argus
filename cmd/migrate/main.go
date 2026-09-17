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

	"github.com/stanimirivanov/argus/internal/catalog/postgres"
)

const databaseURLEnvironment = "ARGUS_DATABASE_URL"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Getenv(databaseURLEnvironment), os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, databaseURL string, output io.Writer) error {
	if databaseURL == "" {
		return errors.New("ARGUS_DATABASE_URL is required")
	}

	store, err := postgres.Open(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("open catalog store: %w", err)
	}
	defer store.Close()

	if err := store.Migrate(ctx); err != nil {
		return fmt.Errorf("migrate catalog: %w", err)
	}
	if _, err := fmt.Fprintln(output, "catalog migrations applied"); err != nil {
		return fmt.Errorf("write migration result: %w", err)
	}

	return nil
}
