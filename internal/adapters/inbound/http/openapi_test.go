package http

import (
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRouterServesOpenAPIYAML(t *testing.T) {
	t.Parallel()

	router := newTestRouter()
	request := httptest.NewRequest(stdhttp.MethodGet, "/v1/openapi.yaml", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	require.Equal(t, stdhttp.StatusOK, response.Code)
	assert.Equal(t, "application/yaml", response.Header().Get("Content-Type"))

	body := response.Body.String()
	assert.True(t, strings.HasPrefix(body, "openapi: 3.1.0"), "spec must start with openapi version")
	assert.Contains(t, body, "title: ObservAI API")
	assert.Contains(t, body, "/v1/analyses")
	assert.Contains(t, body, "WrapperDtoResponde_AnalysisResponseDto")
}

func TestOpenAPIDocumentEmbeddedAtCompileTime(t *testing.T) {
	t.Parallel()

	document := OpenAPIDocument()
	require.NotEmpty(t, document, "embedded OpenAPI document must not be empty")
	assert.Contains(t, string(document), "Friendly refusal for a question unrelated to the analysis.")
}

func TestRouterServesSwaggerUI(t *testing.T) {
	t.Parallel()

	router := newTestRouter()
	request := httptest.NewRequest(stdhttp.MethodGet, "/docs", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	require.Equal(t, stdhttp.StatusOK, response.Code)
	assert.Contains(t, response.Header().Get("Content-Type"), "text/html")
	body := response.Body.String()
	assert.Contains(t, body, "ObservAI API")
	assert.Contains(t, body, openAPIYAMLRoutePath)
}

func TestRouterServesSwaggerUIAssets(t *testing.T) {
	t.Parallel()

	router := newTestRouter()
	request := httptest.NewRequest(stdhttp.MethodGet, "/docs/swagger-ui.css", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	require.Equal(t, stdhttp.StatusOK, response.Code)
	assert.Contains(t, response.Header().Get("Content-Type"), "text/css")
	assert.NotEmpty(t, response.Body.String())
}

func TestRouterRedirectsSwaggerAlias(t *testing.T) {
	t.Parallel()

	router := newTestRouter()
	request := httptest.NewRequest(stdhttp.MethodGet, "/swagger", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	require.Equal(t, stdhttp.StatusTemporaryRedirect, response.Code)
	assert.Equal(t, swaggerUIRoutePath, response.Header().Get("Location"))
}

func TestOpenAPIDocumentCoversNewAdminAndAuthSurface(t *testing.T) {
	t.Parallel()

	spec := string(OpenAPIDocument())
	for _, path := range []string{
		"/v1/setup/status",
		"/v1/setup/admin",
		"/v1/auth/login",
		"/v1/auth/logout",
		"/v1/auth/refresh",
		"/v1/me",
		"/v1/me/password",
		"/v1/me/preferences",
		"/v1/me/sessions",
		"/v1/me/keys",
		"/v1/telemetry",
		"/v1/admin/users",
		"/v1/admin/keys",
		"/v1/admin/providers",
		"/v1/admin/providers/{providerID}/test",
		"/v1/admin/llm-providers",
		"/v1/admin/llm-providers/{llmID}/activate",
		"/v1/admin/provider-types",
		"/v1/admin/webhooks/{webhookID}/test",
		"/v1/admin/webhook-deliveries",
		"/v1/admin/webhook-deliveries/{deliveryID}/retry",
		"/v1/admin/webhook-deliveries/{deliveryID}/replay",
		"/v1/admin/audit",
		"/v1/admin/analyses",
		"/v1/admin/retention/policy",
		"/v1/admin/retention/preview",
	} {
		assert.Contains(t, spec, path, "OpenAPI spec must document %s", path)
	}
	assert.Contains(t, spec, "version: 1.0.0", "version must be bumped to 1.0.0")
	assert.Contains(t, spec, "text/event-stream", "chat SSE must be documented")
	assert.Contains(t, spec, "WrapperDtoResponde_ReadinessResponseDto")
	assert.Contains(t, spec, "provider_auth_failed")
	assert.Contains(t, spec, "mustChangePassword")
	assert.Contains(t, spec, "evidenceIds")
	assert.Contains(t, spec, "correlationId")
}
