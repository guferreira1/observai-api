package http

import (
	"context"
	"encoding/json"
	"log/slog"
	stdhttp "net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/guferreira1/observai-api/internal/platform/logger"
)

const (
	requestIDHeader = "X-Request-Id"
	requestIDLogKey = "requestId"
)

// requestIDMiddleware honors a client-supplied X-Request-Id when present and
// falls back to the chi-generated identifier propagated by middleware.RequestID.
func requestIDMiddleware(next stdhttp.Handler) stdhttp.Handler {
	return stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
		requestID := strings.TrimSpace(request.Header.Get(requestIDHeader))
		if requestID == "" {
			requestID = middleware.GetReqID(request.Context())
		}
		if requestID != "" {
			ctx := context.WithValue(request.Context(), middleware.RequestIDKey, requestID)
			request = request.WithContext(ctx)
			writer.Header().Set(requestIDHeader, requestID)
		}
		next.ServeHTTP(writer, request)
	})
}

// loggerMiddleware enriches the request context with a request-scoped slog.Logger
// and logs request completion with status, latency and route pattern.
func loggerMiddleware(base *slog.Logger) func(stdhttp.Handler) stdhttp.Handler {
	return func(next stdhttp.Handler) stdhttp.Handler {
		return stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
			startedAt := time.Now()
			requestID := middleware.GetReqID(request.Context())
			scoped := base.With(slog.String(requestIDLogKey, requestID))
			ctx := logger.Into(request.Context(), scoped)
			request = request.WithContext(ctx)

			recorder := &statusRecorder{ResponseWriter: writer, status: stdhttp.StatusOK}
			next.ServeHTTP(recorder, request)

			logger.FromContext(request.Context()).LogAttrs(request.Context(), slog.LevelInfo, "http request",
				slog.String("method", request.Method),
				slog.String("path", routePattern(request)),
				slog.Int("status", recorder.status),
				slog.Int64("durationMs", time.Since(startedAt).Milliseconds()),
				slog.String("remote", remoteAddr(request)),
			)
		})
	}
}

// timeoutMiddleware enforces a per-request processing budget through context cancellation.
// Handlers that respect ctx.Err propagate context.DeadlineExceeded back to the error mapper.
func timeoutMiddleware(timeout time.Duration) func(stdhttp.Handler) stdhttp.Handler {
	if timeout <= 0 {
		return func(next stdhttp.Handler) stdhttp.Handler { return next }
	}
	return func(next stdhttp.Handler) stdhttp.Handler {
		return stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
			ctx, cancel := context.WithTimeout(request.Context(), timeout)
			defer cancel()
			next.ServeHTTP(writer, request.WithContext(ctx))
		})
	}
}

// bodyLimitMiddleware caps request body size to mitigate abusive payloads.
func bodyLimitMiddleware(maxBytes int64) func(stdhttp.Handler) stdhttp.Handler {
	if maxBytes <= 0 {
		return func(next stdhttp.Handler) stdhttp.Handler { return next }
	}
	return func(next stdhttp.Handler) stdhttp.Handler {
		return stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
			if request.Body != nil {
				request.Body = stdhttp.MaxBytesReader(writer, request.Body, maxBytes)
			}
			next.ServeHTTP(writer, request)
		})
	}
}

// recoverMiddleware converts panics into a sanitized JSON 500 response and logs the cause.
func recoverMiddleware(base *slog.Logger, provider providerSummaryProvider) func(stdhttp.Handler) stdhttp.Handler {
	return func(next stdhttp.Handler) stdhttp.Handler {
		return stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
			defer func() {
				recovered := recover()
				if recovered == nil {
					return
				}
				if recovered == stdhttp.ErrAbortHandler {
					panic(recovered)
				}

				panicLogger := loggerFromContext(request.Context(), base)
				panicLogger.LogAttrs(request.Context(), slog.LevelError, "http handler panic",
					slog.Any("panic", recovered),
					slog.String("method", request.Method),
					slog.String("path", routePattern(request)),
				)

				writePanicResponse(writer, requestIDFromContext(request.Context()), middlewareProviderSummary(provider))
			}()

			next.ServeHTTP(writer, request)
		})
	}
}

type providerSummaryProvider func() ProviderSummary

func writePanicResponse(writer stdhttp.ResponseWriter, requestID string, provider ProviderSummary) {
	writeMiddlewareErrorResponse(writer, requestID, stdhttp.StatusInternalServerError, "", ErrorResponse{
		Code:    "internal_error",
		Message: "internal server error",
	}, provider)
}

func writeMiddlewareErrorResponse(writer stdhttp.ResponseWriter, requestID string, status int, retryAfter string, response ErrorResponse, provider ProviderSummary) {
	writer.Header().Set("Content-Type", "application/json")
	if requestID != "" {
		writer.Header().Set(requestIDHeader, requestID)
	}
	if retryAfter != "" {
		writer.Header().Set("Retry-After", retryAfter)
	}
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(WrapperDtoResponde[ErrorResponse]{
		Data: response,
		Metadata: ResponseMetadata{
			RequestID:        requestID,
			ProcessingTimeMs: 0,
			Provider:         provider,
		},
	})
}

func middlewareProviderSummary(provider providerSummaryProvider) ProviderSummary {
	if provider == nil {
		return ProviderSummary{Mode: "local"}
	}
	summary := provider()
	if summary.Mode == "" {
		summary.Mode = "local"
	}
	return summary
}

func requestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(middleware.RequestIDKey).(string); ok {
		return id
	}
	return ""
}

func loggerFromContext(ctx context.Context, fallback *slog.Logger) *slog.Logger {
	if log := logger.FromContext(ctx); log != nil {
		return log
	}
	return fallback
}

func remoteAddr(request *stdhttp.Request) string {
	if forwarded := request.Header.Get("X-Forwarded-For"); forwarded != "" {
		if comma := strings.IndexByte(forwarded, ','); comma >= 0 {
			return strings.TrimSpace(forwarded[:comma])
		}
		return strings.TrimSpace(forwarded)
	}
	return request.RemoteAddr
}

type statusRecorder struct {
	stdhttp.ResponseWriter
	status int
}

func (recorder *statusRecorder) WriteHeader(status int) {
	recorder.status = status
	recorder.ResponseWriter.WriteHeader(status)
}

// Flush forwards SSE flushes to the underlying ResponseWriter when it supports
// http.Flusher. Without this, downstream handlers that perform a Flusher type
// assertion on the wrapped writer would lose access to the real flusher.
func (recorder *statusRecorder) Flush() {
	if flusher, ok := recorder.ResponseWriter.(stdhttp.Flusher); ok {
		flusher.Flush()
	}
}

func routePattern(request *stdhttp.Request) string {
	routeContext := chi.RouteContext(request.Context())
	if routeContext == nil {
		return request.URL.Path
	}

	pattern := routeContext.RoutePattern()
	if pattern == "" {
		return request.URL.Path
	}

	return pattern
}
