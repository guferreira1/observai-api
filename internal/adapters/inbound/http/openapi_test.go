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
	assert.Contains(t, string(document), "question_out_of_scope")
}
