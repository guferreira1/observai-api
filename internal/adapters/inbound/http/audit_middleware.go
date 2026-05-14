package http

import (
	"context"
	"log/slog"
	stdhttp "net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/core/usecase"
)

// auditMiddleware persists one audit_log entry per authenticated request.
//
// Requests without an authenticated principal (e.g. operational probes or
// local/dev with auth disabled) are skipped. The audit append happens in
// the background so it never blocks the response; failures are logged at
// warning level and swallowed because audit failures must not surface as
// 5xx responses.
func auditMiddleware(useCase *usecase.AuditLog, logger *slog.Logger) func(stdhttp.Handler) stdhttp.Handler {
	if useCase == nil {
		return func(next stdhttp.Handler) stdhttp.Handler { return next }
	}
	if logger == nil {
		logger = slog.Default()
	}
	return func(next stdhttp.Handler) stdhttp.Handler {
		return stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
			startedAt := time.Now()
			recorder := &statusRecorder{ResponseWriter: writer, status: stdhttp.StatusOK}
			next.ServeHTTP(recorder, request)

			principal, ok := PrincipalFromContext(request.Context())
			if !ok {
				return
			}

			entry := domain.AuditEntry{
				RequestID:  middleware.GetReqID(request.Context()),
				APIKeyID:   principal.KeyID,
				Actor:      principal.Name,
				Method:     request.Method,
				Path:       routePattern(request),
				Status:     recorder.status,
				DurationMs: time.Since(startedAt).Milliseconds(),
				Remote:     remoteAddr(request),
				CreatedAt:  time.Now().UTC(),
			}
			go func() {
				if err := useCase.Append(context.Background(), entry); err != nil {
					logger.Warn("audit append failed", "error", err)
				}
			}()
		})
	}
}
