package http

import (
	"context"
	"encoding/json"
	"errors"
	stdhttp "net/http"
	"strings"

	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/core/ports"
	"github.com/guferreira1/observai-api/internal/core/usecase"
	"github.com/guferreira1/observai-api/internal/platform/crypto"
)

// AuthConfig configures bearer + cookie authentication for HTTP requests.
//
// StaticKeys lists API keys defined in environment/YAML; they grant the
// "default" API-key scope. AdminKeys list API keys that additionally
// unlock admin endpoints. The APIKeys use case, when non-nil, resolves
// runtime-issued API keys persisted in the database. Signer and Users,
// when both non-nil, enable JWT cookie authentication. Skip enumerates
// extra URL paths that bypass authentication; operational paths are
// always skipped.
type AuthConfig struct {
	StaticKeys []string
	AdminKeys  []string
	APIKeys    *usecase.APIKey
	Signer     *crypto.JWTSigner
	Users      ports.UserRepository
	Skip       []string
}

// AuthSource names the credential type used to authenticate the request.
type AuthSource string

const (
	// AuthSourceAPIKey indicates the request authenticated with an API key.
	AuthSourceAPIKey AuthSource = "api_key"
	// AuthSourceUser indicates the request authenticated with a JWT cookie
	// representing a registered user.
	AuthSourceUser AuthSource = "user"
)

type authContextKey struct{}

// AuthPrincipal carries the resolved authentication data for the request.
type AuthPrincipal struct {
	Source AuthSource
	KeyID  string
	UserID string
	Name   string
	Scope  domain.APIKeyScope
	Role   domain.Role
}

// EffectiveRole maps API-key scopes to user roles so authorization checks
// can be written uniformly. Admin keys map to RoleAdmin, default keys to
// RoleOperator (they may submit analyses and use the chat).
func (principal AuthPrincipal) EffectiveRole() domain.Role {
	if principal.Source == AuthSourceUser {
		return principal.Role
	}
	if principal.Scope == domain.APIKeyScopeAdmin {
		return domain.RoleAdmin
	}
	return domain.RoleOperator
}

// PrincipalFromContext returns the authenticated principal previously
// attached by authMiddleware, when present.
func PrincipalFromContext(ctx context.Context) (AuthPrincipal, bool) {
	value, ok := ctx.Value(authContextKey{}).(AuthPrincipal)
	return value, ok
}

func withPrincipal(ctx context.Context, principal AuthPrincipal) context.Context {
	return context.WithValue(ctx, authContextKey{}, principal)
}

// SessionCookieName is the name of the cookie that carries the JWT access
// token for browser-based sessions.
const SessionCookieName = "oai_session"

// RefreshCookieName is the name of the cookie that carries the refresh token.
const RefreshCookieName = "oai_refresh"

// CSRFCookieName is the name of the cookie that carries the CSRF token in
// the double-submit pattern.
const CSRFCookieName = "oai_csrf"

// authMiddleware enforces authentication against the supplied configuration.
//
// The middleware tries credentials in order: JWT cookie first (when a
// signer and user repository are configured), then static API keys, then
// persisted API keys. Operational endpoints listed in Skip bypass
// authentication entirely. When no credential source is configured the
// middleware is a no-op so local development runs without secrets.
func authMiddleware(config AuthConfig) func(stdhttp.Handler) stdhttp.Handler {
	staticKeys := indexKeys(config.StaticKeys)
	adminKeys := indexKeys(config.AdminKeys)
	skip := indexSkipPaths(config.Skip)

	cookieEnabled := config.Signer != nil && config.Users != nil
	apiKeyEnabled := len(staticKeys) > 0 || len(adminKeys) > 0 || config.APIKeys != nil
	authDisabled := !cookieEnabled && !apiKeyEnabled

	openPrincipal := AuthPrincipal{
		Source: AuthSourceAPIKey,
		Name:   "anonymous",
		Scope:  domain.APIKeyScopeAdmin,
	}

	return func(next stdhttp.Handler) stdhttp.Handler {
		return stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
			if authDisabled {
				next.ServeHTTP(writer, request.WithContext(withPrincipal(request.Context(), openPrincipal)))
				return
			}
			if skip[request.URL.Path] {
				next.ServeHTTP(writer, request)
				return
			}

			if cookieEnabled {
				if principal, ok := resolveCookie(request, config.Signer, config.Users); ok {
					next.ServeHTTP(writer, request.WithContext(withPrincipal(request.Context(), principal)))
					return
				}
			}

			if apiKeyEnabled {
				token, ok := bearerToken(request.Header.Get("Authorization"))
				if !ok {
					writeUnauthorized(writer)
					return
				}

				principal, ok := resolveStaticKey(token, staticKeys, adminKeys)
				if !ok && config.APIKeys != nil {
					persisted, err := config.APIKeys.Resolve(request.Context(), token)
					if err != nil {
						if !errors.Is(err, domain.ErrAPIKeyNotFound) {
							writeUnauthorized(writer)
							return
						}
					} else {
						principal = AuthPrincipal{
							Source: AuthSourceAPIKey,
							KeyID:  persisted.ID,
							Name:   persisted.Name,
							Scope:  persisted.Scope,
						}
						ok = true
					}
				}
				if !ok {
					writeUnauthorized(writer)
					return
				}
				next.ServeHTTP(writer, request.WithContext(withPrincipal(request.Context(), principal)))
				return
			}

			writeUnauthorized(writer)
		})
	}
}

func resolveCookie(request *stdhttp.Request, signer *crypto.JWTSigner, users ports.UserRepository) (AuthPrincipal, bool) {
	cookie, err := request.Cookie(SessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return AuthPrincipal{}, false
	}
	claims, err := signer.Parse(cookie.Value)
	if err != nil {
		return AuthPrincipal{}, false
	}
	role := domain.Role(claims.Role)
	if !domain.IsValidRole(role) {
		return AuthPrincipal{}, false
	}
	user, err := users.FindByID(request.Context(), claims.Subject)
	if err != nil || !user.IsActive {
		return AuthPrincipal{}, false
	}
	return AuthPrincipal{
		Source: AuthSourceUser,
		UserID: user.ID,
		Name:   user.Email,
		Role:   role,
	}, true
}

// RequireAdminScope wraps a handler so only requests authenticated with an
// admin-scoped key or an admin user reach it. Use for /v1/admin/* routes.
func RequireAdminScope(next stdhttp.HandlerFunc) stdhttp.HandlerFunc {
	return RequireRole(domain.RoleAdmin)(next)
}

// RequireRole returns a wrapper that allows only requests whose principal
// has one of the supplied effective roles.
func RequireRole(roles ...domain.Role) func(stdhttp.HandlerFunc) stdhttp.HandlerFunc {
	allowed := make(map[domain.Role]bool, len(roles))
	for _, role := range roles {
		allowed[role] = true
	}
	return func(next stdhttp.HandlerFunc) stdhttp.HandlerFunc {
		return func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
			principal, ok := PrincipalFromContext(request.Context())
			if !ok || !allowed[principal.EffectiveRole()] {
				writeForbidden(writer)
				return
			}
			next(writer, request)
		}
	}
}

func resolveStaticKey(token string, staticKeys, adminKeys map[string]bool) (AuthPrincipal, bool) {
	if adminKeys[token] {
		return AuthPrincipal{Source: AuthSourceAPIKey, Name: "static-admin", Scope: domain.APIKeyScopeAdmin}, true
	}
	if staticKeys[token] {
		return AuthPrincipal{Source: AuthSourceAPIKey, Name: "static-default", Scope: domain.APIKeyScopeDefault}, true
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
	defaults := []string{
		"/health", "/healthz", "/readyz", "/metrics",
		"/v1/openapi.yaml",
		"/v1/auth/login", "/v1/auth/refresh",
	}
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
	writeAuthFailure(writer, stdhttp.StatusUnauthorized, "unauthorized", "missing or invalid credentials")
}

func writeForbidden(writer stdhttp.ResponseWriter) {
	writeAuthFailure(writer, stdhttp.StatusForbidden, "forbidden", "insufficient privileges")
}

func writeAuthFailure(writer stdhttp.ResponseWriter, status int, code, message string) {
	payload := WrapperDtoResponde[ErrorResponse]{
		Data: ErrorResponse{Code: code, Message: message},
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(payload)
}
