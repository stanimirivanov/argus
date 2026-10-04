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
	"time"

	dbosgo "github.com/dbos-inc/dbos-transact-golang/dbos"
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
	if len(os.Args) > 2 || (len(os.Args) == 2 && os.Args[1] != "--dbos-evaluation") {
		os.Exit(commandline.Report(os.Stderr, commandline.UsageText("usage: migrate [--dbos-evaluation]")))
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var err error
	if len(os.Args) == 2 {
		err = runDBOSEvaluation(ctx, os.Getenv(databaseURLEnvironment), os.Stdout)
	} else {
		err = run(ctx, os.Getenv(databaseURLEnvironment), os.Stdout)
	}
	if err != nil {
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

// runDBOSEvaluation is an explicit privileged operation. The ordinary server
// launches DBOS with SkipMigrations and cannot create or upgrade this schema.
func runDBOSEvaluation(ctx context.Context, databaseURL string, output io.Writer) error {
	if databaseURL == "" {
		return errors.New("ARGUS_DATABASE_URL is required")
	}
	runtime, err := dbosgo.NewContext(ctx, dbosgo.Config{
		AppName:        "argus-change-evaluation-migration",
		DatabaseURL:    databaseURL,
		DatabaseSchema: "argus_dbos_eval",
	})
	if err != nil {
		return errors.New("DBOS evaluation migration failed; inspect database logs")
	}
	if err := dbosgo.Shutdown(runtime, 10*time.Second); err != nil {
		return errors.New("DBOS evaluation migration cleanup failed")
	}
	if _, err := fmt.Fprintln(output, "DBOS evaluation schema prepared"); err != nil {
		return fmt.Errorf("write migration result: %w", err)
	}

	return nil
}
