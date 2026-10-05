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
	Synopsis: "migrate [--local] [--dbos-evaluation]",
	Role:     "administrator",
}

func main() {
	if code, handled := commandline.HandleMeta(os.Args[1:], os.Stdout, os.Stderr, commandSpec); handled {
		os.Exit(code)
	}
	local, dbosEvaluation, err := parseMode(os.Args[1:])
	if err != nil {
		os.Exit(commandline.Report(os.Stderr, err))
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	databaseURL := os.Getenv(databaseURLEnvironment)
	if local {
		if databaseURL != "" {
			os.Exit(commandline.Report(os.Stderr, commandline.UsageText("--local cannot be combined with ARGUS_DATABASE_URL")))
		}
		databaseURL = commandline.LocalDatabaseURL
	}
	if dbosEvaluation {
		err = runDBOSEvaluation(ctx, databaseURL, os.Stdout)
	} else {
		err = run(ctx, databaseURL, os.Stdout)
	}
	if err != nil {
		os.Exit(commandline.Report(os.Stderr, err))
	}
}

func parseMode(args []string) (bool, bool, error) {
	var local, dbosEvaluation bool
	for _, arg := range args {
		switch arg {
		case "--local":
			if local {
				return false, false, commandline.UsageText("--local may be specified only once")
			}
			local = true
		case "--dbos-evaluation":
			if dbosEvaluation {
				return false, false, commandline.UsageText("--dbos-evaluation may be specified only once")
			}
			dbosEvaluation = true
		default:
			return false, false, commandline.UsageText("usage: migrate [--local] [--dbos-evaluation]")
		}
	}

	return local, dbosEvaluation, nil
}

func run(ctx context.Context, databaseURL string, output io.Writer) error {
	if databaseURL == "" {
		return errors.New("ARGUS_DATABASE_URL is required (or use --local with compose.local.yaml)")
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
		return errors.New("ARGUS_DATABASE_URL is required (or use --local with compose.local.yaml)")
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
