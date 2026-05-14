package http

import (
	"context"
	"encoding/json"
	"errors"
	stdhttp "net/http"
	"strings"

	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/core/usecase"
)

// AuthConfig configures the API-key bearer middleware.
//
// StaticKeys lists keys defined in environment/YAML; they grant the
// "default" scope. AdminKeys list keys that additionally unlock admin
// endpoints (/v1/admin/*). The Keys use case, when non-nil, resolves
// runtime-issued API keys persisted in the database. Skip enumerates
// extra URL paths that bypass authentication; operational paths are
// always skipped.
type AuthConfig struct {
	StaticKeys []string
	AdminKeys  []string
	Keys       *usecase.APIKey
	Skip       []string
}

type authContextKey struct{}

// AuthPrincipal carries the resolved authentication data for the request.
type AuthPrincipal struct {
	KeyID string
	Name  string
	Scope domain.APIKeyScope
}

// PrincipalFromContext returns the authenticated principal previously
// attached by authMiddleware, when present.
func PrincipalFromContext(ctx context.Context) (AuthPrincipal, bool) {
	value, ok := ctx.Value(authContextKey{}).(AuthPrincipal)
	return value, ok
}

// authMiddleware enforces bearer authentication against the supplied
// configuration. Static keys are checked first (cheap map lookup); when
// the supplied token does not match any static key the persistent
// repository is consulted. The resolved principal is attached to the
// request context so downstream handlers can audit or scope responses.
//
// When neither static keys nor the persistent repository are configured
// the middleware is a no-op so local/dev runs do not require credentials.
func authMiddleware(config AuthConfig) func(stdhttp.Handler) stdhttp.Handler {
	staticKeys := indexKeys(config.StaticKeys)
	adminKeys := indexKeys(config.AdminKeys)
	skip := indexSkipPaths(config.Skip)

	authDisabled := len(staticKeys) == 0 && len(adminKeys) == 0 && config.Keys == nil

	return func(next stdhttp.Handler) stdhttp.Handler {
		return stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
			if authDisabled || skip[request.URL.Path] {
				next.ServeHTTP(writer, request)
				return
			}

			token, ok := bearerToken(request.Header.Get("Authorization"))
			if !ok {
				writeUnauthorized(writer)
				return
			}

			principal, ok := resolveStaticKey(token, staticKeys, adminKeys)
			if !ok && config.Keys != nil {
				persisted, err := config.Keys.Resolve(request.Context(), token)
				if err != nil {
					if !errors.Is(err, domain.ErrAPIKeyNotFound) {
						writeUnauthorized(writer)
						return
					}
				} else {
					principal = AuthPrincipal{KeyID: persisted.ID, Name: persisted.Name, Scope: persisted.Scope}
					ok = true
				}
			}
			if !ok {
				writeUnauthorized(writer)
				return
			}

			ctx := context.WithValue(request.Context(), authContextKey{}, principal)
			next.ServeHTTP(writer, request.WithContext(ctx))
		})
	}
}

// RequireAdminScope wraps a handler so only requests authenticated with an
// admin-scoped key reach it. Use for /v1/admin/* routes.
func RequireAdminScope(next stdhttp.HandlerFunc) stdhttp.HandlerFunc {
	return func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
		principal, ok := PrincipalFromContext(request.Context())
		if !ok || principal.Scope != domain.APIKeyScopeAdmin {
			writeForbidden(writer)
			return
		}
		next(writer, request)
	}
}

func resolveStaticKey(token string, staticKeys, adminKeys map[string]bool) (AuthPrincipal, bool) {
	if adminKeys[token] {
		return AuthPrincipal{Name: "static-admin", Scope: domain.APIKeyScopeAdmin}, true
	}
	if staticKeys[token] {
		return AuthPrincipal{Name: "static-default", Scope: domain.APIKeyScopeDefault}, true
	}
	return AuthPrincipal{}, false
}

func bearerToken(header string) (string, bool) {
	trimmed := strings.TrimSpace(header)
	if trimmed == "" {
		return "", false
	}
	const prefix = "Bearer "
	if len(trimmed) <= len(prefix) || !strings.EqualFold(trimmed[:len(prefix)], prefix) {
		return "", false
	}
	return strings.TrimSpace(trimmed[len(prefix):]), true
}

func indexKeys(values []string) map[string]bool {
	if len(values) == 0 {
		return nil
	}
	index := make(map[string]bool, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		index[trimmed] = true
	}
	return index
}

func indexSkipPaths(values []string) map[string]bool {
	defaults := []string{"/health", "/healthz", "/readyz", "/metrics"}
	index := make(map[string]bool, len(defaults)+len(values))
	for _, path := range defaults {
		index[path] = true
	}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			index[trimmed] = true
		}
	}
	return index
}

func writeUnauthorized(writer stdhttp.ResponseWriter) {
	payload := WrapperDtoResponde[ErrorResponse]{
		Data: ErrorResponse{
			Code:    "unauthorized",
			Message: "missing or invalid API key",
		},
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(stdhttp.StatusUnauthorized)
	_ = json.NewEncoder(writer).Encode(payload)
}

func writeForbidden(writer stdhttp.ResponseWriter) {
	payload := WrapperDtoResponde[ErrorResponse]{
		Data: ErrorResponse{
			Code:    "forbidden",
			Message: "admin scope required",
		},
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(stdhttp.StatusForbidden)
	_ = json.NewEncoder(writer).Encode(payload)
}
