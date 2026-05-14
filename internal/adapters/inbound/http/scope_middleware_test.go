package http

import (
	stdhttp "net/http"
	"net/http/httptest"
	"testing"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

func TestRequireScopeAllowsPrincipalWithAllScopes(t *testing.T) {
	guarded := RequireScope(domain.APIKeyScopeAdminWrite)(func(writer stdhttp.ResponseWriter, _ *stdhttp.Request) {
		writer.WriteHeader(stdhttp.StatusOK)
	})
	request := httptest.NewRequest(stdhttp.MethodPost, "/v1/admin/keys", nil)
	ctx := withPrincipal(request.Context(), AuthPrincipal{
		Source: AuthSourceAPIKey,
		Scopes: []domain.APIKeyScope{domain.APIKeyScopeAdminWrite},
	})
	response := httptest.NewRecorder()
	guarded(response, request.WithContext(ctx))
	if response.Code != stdhttp.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
}

func TestRequireScopeRejectsPrincipalMissingScope(t *testing.T) {
	guarded := RequireScope(domain.APIKeyScopeAdminWrite)(func(writer stdhttp.ResponseWriter, _ *stdhttp.Request) {
		writer.WriteHeader(stdhttp.StatusOK)
	})
	request := httptest.NewRequest(stdhttp.MethodPost, "/v1/admin/keys", nil)
	ctx := withPrincipal(request.Context(), AuthPrincipal{
		Source: AuthSourceAPIKey,
		Scopes: []domain.APIKeyScope{domain.APIKeyScopeAnalysisRead},
	})
	response := httptest.NewRecorder()
	guarded(response, request.WithContext(ctx))
	if response.Code != stdhttp.StatusForbidden {
		t.Fatalf("expected 403, got %d", response.Code)
	}
}

func TestEffectiveRoleDerivesAdminFromAdminScope(t *testing.T) {
	principal := AuthPrincipal{Source: AuthSourceAPIKey, Scopes: []domain.APIKeyScope{domain.APIKeyScopeAdminRead}}
	if role := principal.EffectiveRole(); role != domain.RoleAdmin {
		t.Fatalf("expected admin role, got %s", role)
	}
}

func TestEffectiveRoleDerivesOperatorFromWriteScope(t *testing.T) {
	principal := AuthPrincipal{Source: AuthSourceAPIKey, Scopes: []domain.APIKeyScope{domain.APIKeyScopeChatWrite}}
	if role := principal.EffectiveRole(); role != domain.RoleOperator {
		t.Fatalf("expected operator role, got %s", role)
	}
}

func TestEffectiveRoleDerivesViewerFromReadScopeOnly(t *testing.T) {
	principal := AuthPrincipal{Source: AuthSourceAPIKey, Scopes: []domain.APIKeyScope{domain.APIKeyScopeAnalysisRead}}
	if role := principal.EffectiveRole(); role != domain.RoleViewer {
		t.Fatalf("expected viewer role, got %s", role)
	}
}

func TestPrincipalHasScopeForUserRoleSet(t *testing.T) {
	principal := AuthPrincipal{Source: AuthSourceUser, Role: domain.RoleOperator}
	if !principal.HasScope(domain.APIKeyScopeChatWrite) {
		t.Fatalf("operator user should have chat:write")
	}
	if principal.HasScope(domain.APIKeyScopeAdminWrite) {
		t.Fatalf("operator user must not have admin:write")
	}
}
