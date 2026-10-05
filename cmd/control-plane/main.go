// Command control-plane runs the Argus control plane.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	dbosgo "github.com/dbos-inc/dbos-transact-golang/dbos"
	dbosadapter "github.com/stanimirivanov/argus/internal/change/adapters/dbos"
	githubadapter "github.com/stanimirivanov/argus/internal/change/adapters/github"
	"github.com/stanimirivanov/argus/internal/change/adapters/httpapi"
	openapiadapter "github.com/stanimirivanov/argus/internal/change/adapters/openapi"
	changeimpact "github.com/stanimirivanov/argus/internal/change/impact"
	"github.com/stanimirivanov/argus/internal/change/ingest"
	"github.com/stanimirivanov/argus/internal/change/workflow"
	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/postgres"
)

const (
	componentName  = "control-plane"
	shutdownPeriod = 10 * time.Second
)

var commandSpec = commandline.Spec{
	Name:     componentName,
	Synopsis: componentName,
	Role:     "server",
}

func main() {
	os.Exit(realMain())
}

func realMain() int {
	if code, handled := commandline.HandleMeta(os.Args[1:], os.Stdout, os.Stderr, commandSpec); handled {
		return code
	}
	if len(os.Args) != 1 {
		return commandline.Report(os.Stderr, commandline.UsageText("usage: control-plane"))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	application, err := newApplication(ctx, os.Getenv)
	if err != nil {
		logger.Error("control plane configuration failed", "component", componentName, "error", err)
		return 1
	}
	defer func() {
		if err := application.Close(); err != nil {
			logger.Error("control plane cleanup failed", "component", componentName, "error", err)
		}
	}()
	if err := run(ctx, logger, application.server); err != nil {
		logger.Error("control plane failed", "component", componentName, "error", err)
		return 1
	}

	return 0
}

type application struct {
	server  *http.Server
	runtime *postgres.Runtime
	durable dbosgo.Context
}

func newApplication(ctx context.Context, getenv func(string) string) (*application, error) {
	config, err := loadConfig(getenv)
	if err != nil {
		return nil, err
	}
	decoder, err := githubadapter.NewWebhookDecoder([]byte(config.webhookSecret), config.githubHost)
	if err != nil {
		return nil, err
	}
	githubClient, err := githubadapter.NewClient(githubadapter.ClientOptions{
		BaseURL: config.githubAPIURL,
		Token:   config.githubToken,
	})
	if err != nil {
		return nil, err
	}
	runtime, err := postgres.OpenRuntime(ctx, config.databaseURL)
	if err != nil {
		return nil, err
	}
	changeStore := runtime.Change()
	ingestionService := ingest.NewService(changeStore, githubClient)
	impactService := changeimpact.NewService(changeStore, openapiadapter.NewAnalyzer(githubClient))
	var service httpapi.IngestService = workflow.NewService(ingestionService, impactService)
	var durable dbosgo.Context
	if config.dbosEvaluation {
		durable, err = dbosgo.NewContext(ctx, dbosgo.Config{
			AppName:        "argus-change-evaluation",
			DatabaseURL:    config.databaseURL,
			DatabaseSchema: "argus_dbos_eval",
			SkipMigrations: true,
		})
		if err != nil {
			runtime.Close()
			return nil, fmt.Errorf("initialize DBOS evaluation: %w", err)
		}
		service = dbosadapter.NewService(durable, ingestionService, impactService, changeStore)
		if err := dbosgo.Launch(durable); err != nil {
			shutdownErr := dbosgo.Shutdown(durable, shutdownPeriod)
			runtime.Close()
			return nil, errors.Join(fmt.Errorf("launch DBOS evaluation: %w", err), shutdownErr)
		}
	}
	server := &http.Server{
		Addr:              config.address,
		Handler:           httpapi.NewHandler(decoder, service),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	return &application{server: server, runtime: runtime, durable: durable}, nil
}

func (application *application) Close() error {
	var shutdownErr error
	if application.durable != nil {
		shutdownErr = dbosgo.Shutdown(application.durable, shutdownPeriod)
	}
	application.runtime.Close()

	return shutdownErr
}

type server interface {
	ListenAndServe() error
	Shutdown(context.Context) error
}

func run(ctx context.Context, logger *slog.Logger, server server) error {
	serveErrors := make(chan error, 1)
	go func() {
		serveErrors <- server.ListenAndServe()
	}()
	logger.Info("control plane started", "component", componentName)

	select {
	case err := <-serveErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve control plane: %w", err)
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), shutdownPeriod)
		defer cancel()
		if err := server.Shutdown(shutdownContext); err != nil {
			return fmt.Errorf("shut down control plane: %w", err)
		}
		if err := <-serveErrors; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve control plane: %w", err)
		}
		logger.Info(
			"control plane stopped",
			"component", componentName,
			"reason", "shutdown requested",
		)

		return nil
	}
}

type config struct {
	address        string
	databaseURL    string
	githubAPIURL   string
	githubHost     string
	githubToken    string
	webhookSecret  string
	dbosEvaluation bool
}

func loadConfig(getenv func(string) string) (config, error) {
	result := config{
		address:       strings.TrimSpace(getenv("ARGUS_HTTP_ADDRESS")),
		databaseURL:   strings.TrimSpace(getenv("ARGUS_DATABASE_URL")),
		githubAPIURL:  strings.TrimSpace(getenv("ARGUS_GITHUB_API_URL")),
		githubHost:    strings.ToLower(strings.TrimSpace(getenv("ARGUS_GITHUB_HOST"))),
		githubToken:   strings.TrimSpace(getenv("ARGUS_GITHUB_TOKEN")),
		webhookSecret: getenv("ARGUS_GITHUB_WEBHOOK_SECRET"),
	}
	if result.address == "" {
		result.address = "127.0.0.1:8080"
	}
	if result.githubAPIURL == "" {
		result.githubAPIURL = "https://api.github.com/"
	}
	if result.githubHost == "" {
		result.githubHost = "github.com"
	}
	missing := make([]string, 0, 3)
	for name, value := range map[string]string{
		"ARGUS_DATABASE_URL":          result.databaseURL,
		"ARGUS_GITHUB_TOKEN":          result.githubToken,
		"ARGUS_GITHUB_WEBHOOK_SECRET": result.webhookSecret,
	} {
		if value == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) != 0 {
		sort.Strings(missing)
		return config{}, fmt.Errorf("required environment is missing: %s", strings.Join(missing, ", "))
	}
	switch strings.TrimSpace(getenv("ARGUS_DBOS_EVALUATION")) {
	case "", "false":
	case "true":
		result.dbosEvaluation = true
	default:
		return config{}, errors.New("ARGUS_DBOS_EVALUATION must be true or false")
	}

	return result, nil
}
