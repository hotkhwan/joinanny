// Command tradebot runs the combined all-in-one runtime (Telegram poller plus
// the API server in a single process). It is intended for local development;
// production deploys split these concerns into the worker and api commands.
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"bottrade/internal/app"
)

func main() {
	bootstrapLogger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	application, logger, err := app.Bootstrap()
	if err != nil {
		bootstrapLogger.Error("startup error", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := application.Run(ctx); err != nil && !isShutdown(ctx, err) {
		logger.Error("application stopped", "error", err)
		os.Exit(1)
	}
}

func isShutdown(ctx context.Context, err error) bool {
	// errors.Is, not ==, so a shutdown error wrapped anywhere up the stack is
	// still recognised as graceful rather than logged as a fatal crash.
	return ctx.Err() != nil && errors.Is(err, ctx.Err())
}
