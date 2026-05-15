package http

import (
	"context"
	"errors"
	stdhttp "net/http"
	"strings"
	"time"

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
	Source           AuthSource
	KeyID            string
	UserID           string
	Name             string
	Scopes           []domain.APIKeyScope
	Role             domain.Role
	SessionID        string
	SessionIssuedAt  time.Time
	SessionExpiresAt time.Time
}

// HasScope reports whether the principal carries the supplied scope value.
//
// User principals derive their scope set from the role: admin has every
// scope, operator covers analysis + chat writes, viewer covers reads only.
func (principal AuthPrincipal) HasScope(target domain.APIKeyScope) bool {
	if principal.Source == AuthSourceUser {
		for _, scope := range roleScopeSet(principal.Role) {
			if scope == target {
				return true
			}
		}
		return false
	}
	for _, scope := range principal.Scopes {
		if scope == target {
			return true
		}
	}
	return false
}

// EffectiveRole maps API-key scopes to user roles so authorization checks
// can be written uniformly. Scopes carrying admin:* map to RoleAdmin; any
// write scope (analysis:write or chat:write) maps to RoleOperator;
// anything else falls back to RoleViewer.
func (principal AuthPrincipal) EffectiveRole() domain.Role {
	if principal.Source == AuthSourceUser {
		return principal.Role
	}
	hasWrite := false
	for _, scope := range principal.Scopes {
		switch scope {
		case domain.APIKeyScopeAdminRead, domain.APIKeyScopeAdminWrite:
			return domain.RoleAdmin
		case domain.APIKeyScopeAnalysisWrite, domain.APIKeyScopeChatWrite:
			hasWrite = true
		}
	}
	if hasWrite {
		return domain.RoleOperator
	}
	return domain.RoleViewer
}

func roleScopeSet(role domain.Role) []domain.APIKeyScope {
	switch role {
	case domain.RoleAdmin:
		return domain.AllAPIKeyScopes()
	case domain.RoleOperator:
		return []domain.APIKeyScope{
			domain.APIKeyScopeAnalysisRead,
			domain.APIKeyScopeAnalysisWrite,
			domain.APIKeyScopeChatWrite,
		}
	case domain.RoleViewer:
		return []domain.APIKeyScope{domain.APIKeyScopeAnalysisRead}
	}
	return nil
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
func authMiddleware(config AuthConfig, provider providerSummaryProvider) func(stdhttp.Handler) stdhttp.Handler {
	staticKeys := indexKeys(config.StaticKeys)
	adminKeys := indexKeys(config.AdminKeys)
	skip := indexSkipPaths(config.Skip)

	cookieEnabled := config.Signer != nil && config.Users != nil
	apiKeyEnabled := len(staticKeys) > 0 || len(adminKeys) > 0 || config.APIKeys != nil
	authDisabled := !cookieEnabled && !apiKeyEnabled

	openPrincipal := AuthPrincipal{
		Source: AuthSourceAPIKey,
		Name:   "anonymous",
		Scopes: domain.AllAPIKeyScopes(),
	}

	return func(next stdhttp.Handler) stdhttp.Handler {
		return stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
			if authDisabled {
				next.ServeHTTP(writer, request.WithContext(withPrincipal(request.Context(), openPrincipal)))
				return
			}
			if shouldSkipAuthentication(request.URL.Path, skip) {
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
					writeUnauthorized(writer, request, provider)
					return
				}

				principal, ok := resolveStaticKey(token, staticKeys, adminKeys)
				if !ok && config.APIKeys != nil {
					persisted, err := config.APIKeys.Resolve(request.Context(), token)
					if err != nil {
						if !errors.Is(err, domain.ErrAPIKeyNotFound) {
							writeUnauthorized(writer, request, provider)
							return
						}
					} else {
						principal = AuthPrincipal{
							Source: AuthSourceAPIKey,
							KeyID:  persisted.ID,
							Name:   persisted.Name,
							Scopes: persisted.Scopes,
						}
						ok = true
					}
				}
				if !ok {
					writeUnauthorized(writer, request, provider)
					return
				}
				next.ServeHTTP(writer, request.WithContext(withPrincipal(request.Context(), principal)))
				return
			}

			writeUnauthorized(writer, request, provider)
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
		Source:           AuthSourceUser,
		UserID:           user.ID,
		Name:             user.Email,
		Role:             role,
		SessionID:        claims.JTI,
		SessionIssuedAt:  claims.IssuedAt,
		SessionExpiresAt: claims.ExpiresAt,
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
				writeForbidden(writer, request, nil)
				return
			}
			next(writer, request)
		}
	}
}

// RequireScope returns a wrapper that allows only requests whose principal
// carries every supplied scope.
func RequireScope(scopes ...domain.APIKeyScope) func(stdhttp.HandlerFunc) stdhttp.HandlerFunc {
	return func(next stdhttp.HandlerFunc) stdhttp.HandlerFunc {
		return func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
			principal, ok := PrincipalFromContext(request.Context())
			if !ok {
				writeForbidden(writer, request, nil)
				return
			}
			for _, scope := range scopes {
				if !principal.HasScope(scope) {
					writeForbidden(writer, request, nil)
					return
				}
			}
			next(writer, request)
		}
	}
}

func resolveStaticKey(token string, staticKeys, adminKeys map[string]bool) (AuthPrincipal, bool) {
	if adminKeys[token] {
		return AuthPrincipal{Source: AuthSourceAPIKey, Name: "static-admin", Scopes: domain.AllAPIKeyScopes()}, true
	}
	if staticKeys[token] {
		return AuthPrincipal{
			Source: AuthSourceAPIKey,
			Name:   "static-default",
			Scopes: []domain.APIKeyScope{
				domain.APIKeyScopeAnalysisRead,
				domain.APIKeyScopeAnalysisWrite,
				domain.APIKeyScopeChatWrite,
			},
		}, true
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
		openAPIYAMLRoutePath, swaggerUIRoutePath, swaggerUIAliasPath,
		"/v1/auth/login", "/v1/auth/refresh",
		"/v1/setup/status", "/v1/setup/admin",
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

func shouldSkipAuthentication(requestPath string, exactSkipPaths map[string]bool) bool {
	return exactSkipPaths[requestPath] ||
		strings.HasPrefix(requestPath, swaggerUIBasePath) ||
		strings.HasPrefix(requestPath, swaggerUIAliasBasePath)
}

func writeUnauthorized(writer stdhttp.ResponseWriter, request *stdhttp.Request, provider providerSummaryProvider) {
	writeAuthFailure(writer, request, provider, stdhttp.StatusUnauthorized, "unauthorized", "missing or invalid credentials")
}

func writeForbidden(writer stdhttp.ResponseWriter, request *stdhttp.Request, provider providerSummaryProvider) {
	writeAuthFailure(writer, request, provider, stdhttp.StatusForbidden, "forbidden", "insufficient privileges")
}

func writeAuthFailure(writer stdhttp.ResponseWriter, request *stdhttp.Request, provider providerSummaryProvider, status int, code, message string) {
	requestID := ""
	if request != nil {
		requestID = requestIDFromContext(request.Context())
	}
	writeMiddlewareErrorResponse(writer, requestID, status, "", ErrorResponse{
		Code:    code,
		Message: message,
	}, middlewareProviderSummary(provider))
}
