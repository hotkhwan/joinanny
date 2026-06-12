package logging

import (
	"fmt"
	"io"
	"log/slog"
	"strings"
)

func New(level string, out io.Writer) (*slog.Logger, error) {
	if out == nil {
		return nil, fmt.Errorf("log output writer is required")
	}

	parsed, err := ParseLevel(level)
	if err != nil {
		return nil, err
	}

	handler := slog.NewJSONHandler(out, &slog.HandlerOptions{
		Level: parsed,
	})

	return slog.New(handler), nil
}

func ParseLevel(level string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info", "":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("unknown log level %q", level)
	}
}
