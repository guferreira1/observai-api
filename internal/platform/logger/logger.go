package logger

import (
	"log/slog"
	"os"
)

// New creates a structured application logger.
func New(env string) *slog.Logger {
	level := slog.LevelInfo
	if env == "local" {
		level = slog.LevelDebug
	}

	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))
}
