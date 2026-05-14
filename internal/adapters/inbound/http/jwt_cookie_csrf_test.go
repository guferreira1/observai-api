package http

import (
	"bytes"
	"context"
	stdhttp "net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/guferreira1/observai-api/internal/adapters/outbound/inmemory"
	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/platform/crypto"
)

func newJWTFixture(t *testing.T) (*crypto.JWTSigner, *inmemory.UserRepository, domain.User) {
	t.Helper()
	signer, err := crypto.NewJWTSigner(bytes.Repeat([]byte{0xab}, crypto.MinJWTSecretLength), "observai-api")
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	users := inmemory.NewUserRepository()
	user := domain.User{
		ID:           "user-1",
		Email:        "admin@observai.io",
		PasswordHash: "hash",
		Role:         domain.RoleAdmin,
		IsActive:     true,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	if err := users.Create(context.Background(), user); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return signer, users, user
}

func issueAccessToken(t *testing.T, signer *crypto.JWTSigner, user domain.User) string {
	t.Helper()
	now := time.Now().UTC()
	token, err := signer.Sign(crypto.JWTClaims{
		Subject:   user.ID,
		Role:      string(user.Role),
		JTI:       "jti-1",
		IssuedAt:  now,
		ExpiresAt: now.Add(15 * time.Minute),
	})
	if err != nil {
		t.Fatalf("sign access token: %v", err)
	}
	return token
}

func TestAuthMiddlewareAcceptsValidJWTCookie(t *testing.T) {
	signer, users, user := newJWTFixture(t)
	captured := AuthPrincipal{}
	handler := authMiddleware(AuthConfig{Signer: signer, Users: users})(stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
		principal, _ := PrincipalFromContext(request.Context())
		captured = principal
		writer.WriteHeader(stdhttp.StatusOK)
	}))

	request := httptest.NewRequest(stdhttp.MethodGet, "/v1/me", nil)
	request.AddCookie(&stdhttp.Cookie{Name: SessionCookieName, Value: issueAccessToken(t, signer, user)})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != stdhttp.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	if captured.Source != AuthSourceUser || captured.UserID != "user-1" || captured.Role != domain.RoleAdmin {
		t.Fatalf("unexpected principal: %+v", captured)
	}
}

func TestAuthMiddlewareRejectsExpiredJWTCookie(t *testing.T) {
	signer, users, user := newJWTFixture(t)
	expired, err := signer.Sign(crypto.JWTClaims{
		Subject:   user.ID,
		Role:      string(user.Role),
		IssuedAt:  time.Now().Add(-time.Hour),
		ExpiresAt: time.Now().Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("sign expired token: %v", err)
	}
	handler := authMiddleware(AuthConfig{Signer: signer, Users: users})(stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, _ *stdhttp.Request) {
		writer.WriteHeader(stdhttp.StatusOK)
	}))

	request := httptest.NewRequest(stdhttp.MethodGet, "/v1/me", nil)
	request.AddCookie(&stdhttp.Cookie{Name: SessionCookieName, Value: expired})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != stdhttp.StatusUnauthorized {
		t.Fatalf("expected 401 for expired token, got %d", response.Code)
	}
}

func TestAuthMiddlewareRejectsInactiveUserCookie(t *testing.T) {
	signer, users, user := newJWTFixture(t)
	if err := users.SetActive(context.Background(), user.ID, false, time.Now().UTC()); err != nil {
		t.Fatalf("deactivate user: %v", err)
	}
	handler := authMiddleware(AuthConfig{Signer: signer, Users: users})(stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, _ *stdhttp.Request) {
		writer.WriteHeader(stdhttp.StatusOK)
	}))

	request := httptest.NewRequest(stdhttp.MethodGet, "/v1/me", nil)
	request.AddCookie(&stdhttp.Cookie{Name: SessionCookieName, Value: issueAccessToken(t, signer, user)})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != stdhttp.StatusUnauthorized {
		t.Fatalf("expected 401 for inactive user, got %d", response.Code)
	}
}

func TestRequireRoleAllowsMatchingRole(t *testing.T) {
	guarded := RequireRole(domain.RoleAdmin)(func(writer stdhttp.ResponseWriter, _ *stdhttp.Request) {
		writer.WriteHeader(stdhttp.StatusOK)
	})
	request := httptest.NewRequest(stdhttp.MethodGet, "/v1/admin/users", nil)
	ctx := withPrincipal(request.Context(), AuthPrincipal{Source: AuthSourceUser, Role: domain.RoleAdmin})
	response := httptest.NewRecorder()
	guarded(response, request.WithContext(ctx))
	if response.Code != stdhttp.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
}

func TestRequireRoleRejectsMismatchedRole(t *testing.T) {
	guarded := RequireRole(domain.RoleAdmin)(func(writer stdhttp.ResponseWriter, _ *stdhttp.Request) {
		writer.WriteHeader(stdhttp.StatusOK)
	})
	request := httptest.NewRequest(stdhttp.MethodGet, "/v1/admin/users", nil)
	ctx := withPrincipal(request.Context(), AuthPrincipal{Source: AuthSourceUser, Role: domain.RoleViewer})
	response := httptest.NewRecorder()
	guarded(response, request.WithContext(ctx))
	if response.Code != stdhttp.StatusForbidden {
		t.Fatalf("expected 403, got %d", response.Code)
	}
}

func TestCSRFMiddlewareEnforcesDoubleSubmitOnMutations(t *testing.T) {
	called := false
	handler := csrfMiddleware()(stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, _ *stdhttp.Request) {
		called = true
		writer.WriteHeader(stdhttp.StatusOK)
	}))

	request := httptest.NewRequest(stdhttp.MethodPost, "/v1/analyses", nil)
	request = request.WithContext(withPrincipal(request.Context(), AuthPrincipal{Source: AuthSourceUser, Role: domain.RoleAdmin}))
	request.AddCookie(&stdhttp.Cookie{Name: CSRFCookieName, Value: "csrf-token"})
	request.Header.Set(CSRFHeaderName, "csrf-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if !called || response.Code != stdhttp.StatusOK {
		t.Fatalf("expected 200 with matching csrf, got %d called=%v", response.Code, called)
	}
}

func TestCSRFMiddlewareRejectsMissingHeader(t *testing.T) {
	handler := csrfMiddleware()(stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, _ *stdhttp.Request) {
		writer.WriteHeader(stdhttp.StatusOK)
	}))
	request := httptest.NewRequest(stdhttp.MethodPost, "/v1/analyses", nil)
	request = request.WithContext(withPrincipal(request.Context(), AuthPrincipal{Source: AuthSourceUser, Role: domain.RoleAdmin}))
	request.AddCookie(&stdhttp.Cookie{Name: CSRFCookieName, Value: "csrf-token"})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != stdhttp.StatusForbidden {
		t.Fatalf("expected 403 without CSRF header, got %d", response.Code)
	}
}

func TestCSRFMiddlewareRejectsMismatchedTokens(t *testing.T) {
	handler := csrfMiddleware()(stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, _ *stdhttp.Request) {
		writer.WriteHeader(stdhttp.StatusOK)
	}))
	request := httptest.NewRequest(stdhttp.MethodPost, "/v1/analyses", nil)
	request = request.WithContext(withPrincipal(request.Context(), AuthPrincipal{Source: AuthSourceUser, Role: domain.RoleAdmin}))
	request.AddCookie(&stdhttp.Cookie{Name: CSRFCookieName, Value: "cookie-value"})
	request.Header.Set(CSRFHeaderName, "header-value")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != stdhttp.StatusForbidden {
		t.Fatalf("expected 403 on mismatched tokens, got %d", response.Code)
	}
}

func TestCSRFMiddlewareSkipsAPIKeyAuth(t *testing.T) {
	handler := csrfMiddleware()(stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, _ *stdhttp.Request) {
		writer.WriteHeader(stdhttp.StatusOK)
	}))
	request := httptest.NewRequest(stdhttp.MethodPost, "/v1/analyses", nil)
	request = request.WithContext(withPrincipal(request.Context(), AuthPrincipal{Source: AuthSourceAPIKey, Scopes: domain.AllAPIKeyScopes()}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != stdhttp.StatusOK {
		t.Fatalf("expected api-key flow to bypass CSRF, got %d", response.Code)
	}
}

func TestCSRFMiddlewareBypassesSafeMethods(t *testing.T) {
	handler := csrfMiddleware()(stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, _ *stdhttp.Request) {
		writer.WriteHeader(stdhttp.StatusOK)
	}))
	for _, method := range []string{stdhttp.MethodGet, stdhttp.MethodHead, stdhttp.MethodOptions} {
		request := httptest.NewRequest(method, "/v1/analyses", nil)
		request = request.WithContext(withPrincipal(request.Context(), AuthPrincipal{Source: AuthSourceUser, Role: domain.RoleAdmin}))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != stdhttp.StatusOK {
			t.Fatalf("%s should bypass CSRF, got %d", method, response.Code)
		}
	}
}
