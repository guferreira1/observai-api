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

type auditAnnotationKey struct{}

// AuditAnnotation carries handler-supplied granular event data that the
// audit middleware merges into the entry persisted at the end of the
// request lifecycle.
type AuditAnnotation struct {
	Action       string
	ResourceType string
	ResourceID   string
	Metadata     map[string]string
}

// AnnotateAudit stamps the supplied granular event data on the request
// context so the audit middleware can persist it alongside the HTTP
// transaction. Subsequent calls merge metadata maps (later values win)
// while overriding non-empty action/resource fields.
func AnnotateAudit(request *stdhttp.Request, annotation AuditAnnotation) *stdhttp.Request {
	if request == nil {
		return request
	}
	merged := mergeAuditAnnotation(annotationFromContext(request.Context()), annotation)
	ctx := context.WithValue(request.Context(), auditAnnotationKey{}, merged)
	*request = *request.WithContext(ctx)
	return request
}

func annotationFromContext(ctx context.Context) AuditAnnotation {
	value, _ := ctx.Value(auditAnnotationKey{}).(AuditAnnotation)
	return value
}

func mergeAuditAnnotation(current, addition AuditAnnotation) AuditAnnotation {
	if addition.Action != "" {
		current.Action = addition.Action
	}
	if addition.ResourceType != "" {
		current.ResourceType = addition.ResourceType
	}
	if addition.ResourceID != "" {
		current.ResourceID = addition.ResourceID
	}
	if len(addition.Metadata) > 0 {
		if current.Metadata == nil {
			current.Metadata = make(map[string]string, len(addition.Metadata))
		}
		for key, value := range addition.Metadata {
			current.Metadata[key] = value
		}
	}
	return current
}

// auditMiddleware persists one audit_log entry per authenticated request.
//
// Requests without an authenticated principal (operational probes,
// local/dev with auth disabled) are skipped. Handlers may attach
// granular event data via AnnotateAudit; the middleware reads the
// annotation after the handler returns. The audit append runs in a
// background goroutine so it never blocks the HTTP response; failures
// are logged at warning level.
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
			annotation := annotationFromContext(request.Context())

			entry := domain.AuditEntry{
				RequestID:    middleware.GetReqID(request.Context()),
				APIKeyID:     principalActorID(principal),
				Actor:        principal.Name,
				Method:       request.Method,
				Path:         routePattern(request),
				Status:       recorder.status,
				DurationMs:   time.Since(startedAt).Milliseconds(),
				Remote:       remoteAddr(request),
				Action:       annotation.Action,
				ResourceType: annotation.ResourceType,
				ResourceID:   annotation.ResourceID,
				Metadata:     annotation.Metadata,
				CreatedAt:    time.Now().UTC(),
			}
			go func() {
				if err := useCase.Append(context.Background(), entry); err != nil {
					logger.Warn("audit append failed", "error", err)
				}
			}()
		})
	}
}

func principalActorID(principal AuthPrincipal) string {
	if principal.Source == AuthSourceUser {
		return principal.UserID
	}
	return principal.KeyID
}
