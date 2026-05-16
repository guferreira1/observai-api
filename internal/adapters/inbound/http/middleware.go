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

			duration := time.Since(startedAt)
			requestLogger := logger.FromContext(request.Context())
			requestLogger.LogAttrs(request.Context(), slog.LevelInfo, "http request",
				httpRequestLogAttrs(request, recorder.status, duration, recorder.principal)...,
			)
			if recorder.error != nil {
				requestLogger.LogAttrs(request.Context(), errorLogLevel(recorder.status), "http error",
					httpErrorLogAttrs(request, recorder.status, duration, *recorder.error, recorder.principal)...,
				)
			}
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

				writePanicResponse(writer, request, middlewareProviderSummary(provider))
			}()

			next.ServeHTTP(writer, request)
		})
	}
}

type providerSummaryProvider func() ProviderSummary

func writePanicResponse(writer stdhttp.ResponseWriter, request *stdhttp.Request, provider ProviderSummary) {
	writeMiddlewareErrorResponse(writer, request, stdhttp.StatusInternalServerError, "", "http.recover", ErrorResponse{
		Code:    "internal_error",
		Message: "internal server error",
	}, provider)
}

func writeMiddlewareErrorResponse(writer stdhttp.ResponseWriter, request *stdhttp.Request, status int, retryAfter string, source string, response ErrorResponse, provider ProviderSummary) {
	requestID := ""
	if request != nil {
		requestID = requestIDFromContext(request.Context())
	}
	recordError(writer, errorLogDetail{
		Code:    response.Code,
		Message: response.Message,
		Source:  source,
		Details: response.Details,
	})
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

type errorLogDetail struct {
	Code    string
	Message string
	Cause   string
	Source  string
	Details []ErrorFieldDetail
}

type statusRecorder struct {
	stdhttp.ResponseWriter
	status    int
	error     *errorLogDetail
	principal *AuthPrincipal
}

func (recorder *statusRecorder) WriteHeader(status int) {
	recorder.status = status
	recorder.ResponseWriter.WriteHeader(status)
}

func (recorder *statusRecorder) RecordError(detail errorLogDetail) {
	recorder.error = &detail
	if nested, ok := recorder.ResponseWriter.(interface{ RecordError(errorLogDetail) }); ok {
		nested.RecordError(detail)
	}
}

func (recorder *statusRecorder) RecordPrincipal(principal AuthPrincipal) {
	recorder.principal = &principal
	if nested, ok := recorder.ResponseWriter.(interface{ RecordPrincipal(AuthPrincipal) }); ok {
		nested.RecordPrincipal(principal)
	}
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

func recordError(writer stdhttp.ResponseWriter, detail errorLogDetail) {
	if recorder, ok := writer.(interface{ RecordError(errorLogDetail) }); ok {
		recorder.RecordError(detail)
	}
}

func recordPrincipal(writer stdhttp.ResponseWriter, principal AuthPrincipal) {
	if recorder, ok := writer.(interface{ RecordPrincipal(AuthPrincipal) }); ok {
		recorder.RecordPrincipal(principal)
	}
}

func httpRequestLogAttrs(request *stdhttp.Request, status int, duration time.Duration, principal *AuthPrincipal) []slog.Attr {
	route := routePattern(request)
	return append([]slog.Attr{
		slog.String("component", "http"),
		slog.String("operation", request.Method+" "+route),
		slog.String("method", request.Method),
		slog.String("path", request.URL.Path),
		slog.String("route", route),
		slog.Int("status", status),
		slog.String("statusText", stdhttp.StatusText(status)),
		slog.Int64("durationMs", duration.Milliseconds()),
		slog.String("remote", remoteAddr(request)),
	}, principalLogAttrs(principal)...)
}

func httpErrorLogAttrs(request *stdhttp.Request, status int, duration time.Duration, detail errorLogDetail, principal *AuthPrincipal) []slog.Attr {
	route := routePattern(request)
	attrs := append([]slog.Attr{
		slog.String("component", "http"),
		slog.String("source", detail.Source),
		slog.String("operation", request.Method+" "+route),
		slog.String("method", request.Method),
		slog.String("path", request.URL.Path),
		slog.String("route", route),
		slog.Int("status", status),
		slog.String("statusText", stdhttp.StatusText(status)),
		slog.String("code", detail.Code),
		slog.String("message", SanitizeExternalMessage(detail.Message)),
		slog.Int64("durationMs", duration.Milliseconds()),
		slog.String("remote", remoteAddr(request)),
	}, principalLogAttrs(principal)...)
	if detail.Cause != "" && detail.Cause != detail.Message {
		attrs = append(attrs, slog.String("cause", SanitizeExternalMessage(detail.Cause)))
	}
	if len(detail.Details) > 0 {
		attrs = append(attrs, slog.Any("details", detail.Details))
	}
	return attrs
}

func principalLogAttrs(principal *AuthPrincipal) []slog.Attr {
	if principal == nil {
		return nil
	}
	attrs := []slog.Attr{slog.String("authSource", string(principal.Source))}
	if principal.UserID != "" {
		attrs = append(attrs, slog.String("userId", principal.UserID))
	}
	if principal.KeyID != "" {
		attrs = append(attrs, slog.String("keyId", principal.KeyID))
	}
	if principal.Role != "" {
		attrs = append(attrs, slog.String("role", string(principal.Role)))
	}
	return attrs
}

func errorLogLevel(status int) slog.Level {
	if status >= stdhttp.StatusInternalServerError {
		return slog.LevelError
	}
	return slog.LevelWarn
}
