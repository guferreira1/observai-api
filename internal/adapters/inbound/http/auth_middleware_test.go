package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

func TestAuthMiddlewareAllowsRequestWithValidBearerToken(t *testing.T) {
	handler := authMiddleware(AuthConfig{StaticKeys: []string{"secret"}}, nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequest(http.MethodGet, "/v1/analyses", nil)
	request.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", response.Code)
	}
}

func TestAuthMiddlewareRejectsRequestWithoutHeader(t *testing.T) {
	handler := authMiddleware(AuthConfig{StaticKeys: []string{"secret"}}, nil)(http.NotFoundHandler())

	request := httptest.NewRequest(http.MethodGet, "/v1/analyses", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", response.Code)
	}
}

func TestAuthMiddlewareRejectsBadCredentials(t *testing.T) {
	handler := authMiddleware(AuthConfig{StaticKeys: []string{"secret"}}, nil)(http.NotFoundHandler())

	request := httptest.NewRequest(http.MethodGet, "/v1/analyses", nil)
	request.Header.Set("Authorization", "Bearer wrong")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", response.Code)
	}
}

func TestAuthMiddlewareIsNoopWhenNoKeysConfigured(t *testing.T) {
	called := false
	handler := authMiddleware(AuthConfig{}, nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	request := httptest.NewRequest(http.MethodGet, "/v1/analyses", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if !called || response.Code != http.StatusOK {
		t.Fatalf("expected handler to be invoked with 200 when auth disabled")
	}
}

func TestAuthMiddlewareDoesNotGrantAdminWhenNoCredentialSourceExists(t *testing.T) {
	adminHandler := RequireRole(domain.RoleAdmin)(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := authMiddleware(AuthConfig{}, nil)(http.HandlerFunc(adminHandler))

	request := httptest.NewRequest(http.MethodGet, "/v1/admin/users", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", response.Code)
	}
}

func TestAuthMiddlewareSkipsOperationalPaths(t *testing.T) {
	handler := authMiddleware(AuthConfig{StaticKeys: []string{"secret"}}, nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, path := range []string{"/healthz", "/readyz", "/metrics", "/health", "/v1/openapi.yaml", "/docs", "/docs/swagger-ui.css", "/swagger", "/swagger/swagger-ui.css"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("expected %s to bypass auth, got %d", path, response.Code)
		}
	}
}
