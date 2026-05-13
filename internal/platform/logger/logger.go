// Package logger creates the application logger and exposes context helpers
// so request-scoped attributes (request id, analysis id, etc.) can be added
// once and inherited by every downstream call.
package logger

import (
	"context"
	"log/slog"
	"os"
)

type contextKey struct{}

var loggerKey = contextKey{}

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

// Into stores the logger in the context. Subsequent FromContext calls return it.
func Into(ctx context.Context, log *slog.Logger) context.Context {
	if log == nil {
		return ctx
	}
	return context.WithValue(ctx, loggerKey, log)
}

// FromContext returns the request-scoped logger, falling back to slog.Default
// when none is attached. Returns a non-nil logger so callers never need a guard.
func FromContext(ctx context.Context) *slog.Logger {
	if ctx == nil {
		return slog.Default()
	}
	if log, ok := ctx.Value(loggerKey).(*slog.Logger); ok && log != nil {
		return log
	}
	return slog.Default()
}

// With returns a derived context whose logger has the supplied attributes appended.
func With(ctx context.Context, attrs ...slog.Attr) context.Context {
	if len(attrs) == 0 {
		return ctx
	}
	args := make([]any, 0, len(attrs))
	for _, attr := range attrs {
		args = append(args, attr)
	}
	enriched := FromContext(ctx).With(args...)
	return Into(ctx, enriched)
}
