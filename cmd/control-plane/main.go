// Command control-plane runs the Argus control plane.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

const componentName = "control-plane"

func main() {
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	run(ctx, logger)
}

func run(ctx context.Context, logger *slog.Logger) {
	logger.Info("control plane started", "component", componentName)

	<-ctx.Done()

	logger.Info(
		"control plane stopped",
		"component", componentName,
		"reason", "shutdown requested",
	)
}
