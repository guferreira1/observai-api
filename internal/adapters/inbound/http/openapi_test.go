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
	assert.Contains(t, spec, "deleteAnalysis")
	assert.Contains(t, spec, "/v1/analyses/{analysisID}/chat/{messageID}/regenerate")
	assert.Contains(t, spec, "version: 1.0.0", "version must be bumped to 1.0.0")
	assert.Contains(t, spec, "text/event-stream", "chat SSE must be documented")
	assert.Contains(t, spec, "WrapperDtoResponde_ReadinessResponseDto")
	assert.Contains(t, spec, "provider_auth_failed")
	assert.Contains(t, spec, "mustChangePassword")
	assert.Contains(t, spec, "evidenceIds")
	assert.Contains(t, spec, "correlationId")
}

func TestOpenAPIDocumentMarksProtectedRoutes(t *testing.T) {
	t.Parallel()

	spec := string(OpenAPIDocument())
	assert.Contains(t, spec, "\nsecurity:\n  - bearerAuth: []\n  - sessionCookie: []\n")
	for _, scheme := range []string{"bearerAuth:", "sessionCookie:", "csrfToken:", "refreshCookie:"} {
		assert.Contains(t, spec, scheme)
	}

	publicOperations := []openAPIOperation{
		{path: "/health", method: "get"},
		{path: "/healthz", method: "get"},
		{path: "/readyz", method: "get"},
		{path: "/metrics", method: "get"},
		{path: "/v1/openapi.yaml", method: "get"},
		{path: "/v1/setup/status", method: "get"},
		{path: "/v1/setup/admin", method: "post"},
		{path: "/v1/auth/login", method: "post"},
	}
	for _, operation := range publicOperations {
		operationBlock := openAPIOperationBlock(t, spec, operation)
		assert.Contains(t, operationBlock, "security: []", "%s %s must stay public", operation.method, operation.path)
	}

	protectedWriteOperations := []openAPIOperation{
		{path: "/v1/telemetry", method: "post"},
		{path: "/v1/analyses", method: "post"},
		{path: "/v1/jobs/{jobID}", method: "delete"},
		{path: "/v1/analyses/{analysisID}", method: "delete"},
		{path: "/v1/analyses/{analysisID}/chat/{messageID}/feedback", method: "post"},
		{path: "/v1/analyses/{analysisID}/chat/{messageID}/regenerate", method: "post"},
		{path: "/v1/analyses/{analysisID}/chat", method: "post"},
		{path: "/v1/auth/logout", method: "post"},
		{path: "/v1/admin/users", method: "post"},
		{path: "/v1/admin/users/{userID}", method: "patch"},
		{path: "/v1/admin/users/{userID}", method: "delete"},
		{path: "/v1/admin/keys", method: "post"},
		{path: "/v1/admin/keys/{keyID}", method: "delete"},
		{path: "/v1/admin/providers", method: "post"},
		{path: "/v1/admin/providers/{providerID}", method: "patch"},
		{path: "/v1/admin/providers/{providerID}", method: "delete"},
		{path: "/v1/admin/providers/{providerID}/test", method: "post"},
		{path: "/v1/admin/providers/{providerID}/activate", method: "post"},
		{path: "/v1/admin/providers/{providerID}/deactivate", method: "post"},
		{path: "/v1/admin/llm-providers", method: "post"},
		{path: "/v1/admin/llm-providers/{llmID}", method: "patch"},
		{path: "/v1/admin/llm-providers/{llmID}", method: "delete"},
		{path: "/v1/admin/llm-providers/{llmID}/test", method: "post"},
		{path: "/v1/admin/llm-providers/{llmID}/activate", method: "post"},
		{path: "/v1/admin/webhooks", method: "post"},
		{path: "/v1/admin/webhooks/{webhookID}", method: "patch"},
		{path: "/v1/admin/webhooks/{webhookID}", method: "delete"},
		{path: "/v1/admin/webhooks/{webhookID}/test", method: "post"},
		{path: "/v1/admin/webhook-deliveries/{deliveryID}/retry", method: "post"},
		{path: "/v1/admin/webhook-deliveries/{deliveryID}/replay", method: "post"},
		{path: "/v1/admin/analyses", method: "delete"},
	}
	for _, operation := range protectedWriteOperations {
		operationBlock := openAPIOperationBlock(t, spec, operation)
		assert.Contains(t, operationBlock, "- bearerAuth: []", "%s %s must allow bearer auth", operation.method, operation.path)
		assert.Contains(t, operationBlock, "- sessionCookie: []", "%s %s must allow session auth", operation.method, operation.path)
		assert.Contains(t, operationBlock, "csrfToken: []", "%s %s must document CSRF for session auth", operation.method, operation.path)
	}

	sessionReadOperations := []openAPIOperation{
		{path: "/v1/me", method: "get"},
		{path: "/v1/me/preferences", method: "get"},
		{path: "/v1/me/sessions", method: "get"},
	}
	for _, operation := range sessionReadOperations {
		operationBlock := openAPIOperationBlock(t, spec, operation)
		assert.Contains(t, operationBlock, "- sessionCookie: []", "%s %s must require user session auth", operation.method, operation.path)
		assert.NotContains(t, operationBlock, "bearerAuth", "%s %s does not accept API-key bearer auth", operation.method, operation.path)
	}

	sessionWriteOperations := []openAPIOperation{
		{path: "/v1/me", method: "patch"},
		{path: "/v1/me/password", method: "post"},
		{path: "/v1/me/preferences", method: "patch"},
	}
	for _, operation := range sessionWriteOperations {
		operationBlock := openAPIOperationBlock(t, spec, operation)
		assert.Contains(t, operationBlock, "- sessionCookie: []", "%s %s must require user session auth", operation.method, operation.path)
		assert.Contains(t, operationBlock, "csrfToken: []", "%s %s must document CSRF for session auth", operation.method, operation.path)
		assert.NotContains(t, operationBlock, "bearerAuth", "%s %s does not accept API-key bearer auth", operation.method, operation.path)
	}

	refreshBlock := openAPIOperationBlock(t, spec, openAPIOperation{path: "/v1/auth/refresh", method: "post"})
	assert.Contains(t, refreshBlock, "- refreshCookie: []")
	assert.NotContains(t, refreshBlock, "bearerAuth")

	capabilitiesBlock := openAPIOperationBlock(t, spec, openAPIOperation{path: "/v1/capabilities", method: "get"})
	assert.NotContains(t, capabilitiesBlock, "security: []", "capabilities inherits the authenticated default from the router")
}

type openAPIOperation struct {
	path   string
	method string
}

func openAPIOperationBlock(t *testing.T, spec string, operation openAPIOperation) string {
	t.Helper()

	pathMarker := "\n  " + operation.path + ":\n"
	pathStart := strings.Index(spec, pathMarker)
	require.NotEqual(t, -1, pathStart, "OpenAPI path %s must exist", operation.path)

	pathBlock := spec[pathStart+len(pathMarker):]
	if nextPathOffset := strings.Index(pathBlock, "\n  /"); nextPathOffset >= 0 {
		pathBlock = pathBlock[:nextPathOffset]
	}

	methodMarker := "    " + operation.method + ":\n"
	methodStart := strings.Index(pathBlock, methodMarker)
	require.NotEqual(t, -1, methodStart, "OpenAPI operation %s %s must exist", operation.method, operation.path)

	operationBlock := pathBlock[methodStart+len(methodMarker):]
	operationEnd := len(operationBlock)
	for _, nextMethod := range []string{"    get:\n", "    post:\n", "    patch:\n", "    delete:\n"} {
		if nextMethodOffset := strings.Index(operationBlock, nextMethod); nextMethodOffset >= 0 && nextMethodOffset < operationEnd {
			operationEnd = nextMethodOffset
		}
	}
	return operationBlock[:operationEnd]
}
