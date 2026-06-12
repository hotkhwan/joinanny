package main

import (
	"context"
	"log/slog"
	"os"

	"bottrade/internal/app"
	"bottrade/internal/config"
	"bottrade/internal/logging"
)

func main() {
	bootstrapLogger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	cfg, err := config.Load()
	if err != nil {
		bootstrapLogger.Error("configuration error", "error", err)
		os.Exit(1)
	}

	logger, err := logging.New(cfg.App.LogLevel, os.Stdout)
	if err != nil {
		bootstrapLogger.Error("logger configuration error", "error", err)
		os.Exit(1)
	}

	application := app.New(cfg, logger)
	if err := application.Run(context.Background()); err != nil {
		logger.Error("application stopped", "error", err)
		os.Exit(1)
	}
}
