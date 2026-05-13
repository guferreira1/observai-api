package http

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	stdhttp "net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/guferreira1/observai-api/internal/adapters/outbound/fake"
	"github.com/guferreira1/observai-api/internal/core/usecase"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRouterCreateAnalysisWrapsResponse(t *testing.T) {
	t.Parallel()

	router := newTestRouter()
	body := bytes.NewBufferString(`{
		"goal": "investigate checkout latency",
		"timeWindow": {
			"start": "2026-05-12T10:00:00Z",
			"end": "2026-05-12T11:00:00Z"
		},
		"affectedServices": ["checkout-service"],
		"signals": ["logs", "metrics", "traces"]
	}`)
	request := httptest.NewRequest(stdhttp.MethodPost, "/v1/analyses", body)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	require.Equal(t, stdhttp.StatusCreated, response.Code)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	assert.Contains(t, payload, "data")
	assert.Contains(t, payload, "metadata")

	data := payload["data"].(map[string]any)
	assert.Equal(t, "analysis-000001", data["id"])
	assert.Equal(t, "high", data["severity"])
}

func TestRouterReturnsAnalysisByID(t *testing.T) {
	t.Parallel()

	router := newTestRouter()
	createRequest := httptest.NewRequest(stdhttp.MethodPost, "/v1/analyses", bytes.NewBufferString(`{
		"goal": "investigate checkout latency",
		"timeWindow": {
			"start": "2026-05-12T10:00:00Z",
			"end": "2026-05-12T11:00:00Z"
		},
		"affectedServices": ["checkout-service"],
		"signals": ["logs", "metrics", "traces"]
	}`))
	createResponse := httptest.NewRecorder()
	router.ServeHTTP(createResponse, createRequest)
	require.Equal(t, stdhttp.StatusCreated, createResponse.Code)

	getRequest := httptest.NewRequest(stdhttp.MethodGet, "/v1/analyses/analysis-000001", nil)
	getResponse := httptest.NewRecorder()
	router.ServeHTTP(getResponse, getRequest)
	require.Equal(t, stdhttp.StatusOK, getResponse.Code)

	var payload WrapperDtoResponde[AnalysisResponseDto]
	require.NoError(t, json.Unmarshal(getResponse.Body.Bytes(), &payload))
	assert.Equal(t, "analysis-000001", payload.Data.ID)
	assert.Equal(t, "high", payload.Data.Severity)
	assert.Equal(t, "fake", payload.Metadata.Provider.LLM)
}

func TestRouterListsAnalysesWithPaginationMetadata(t *testing.T) {
	t.Parallel()

	router := newTestRouter()
	createCheckoutRequest := httptest.NewRequest(stdhttp.MethodPost, "/v1/analyses", bytes.NewBufferString(`{
		"goal": "investigate checkout latency",
		"timeWindow": {
			"start": "2026-05-12T10:00:00Z",
			"end": "2026-05-12T11:00:00Z"
		},
		"affectedServices": ["checkout-service"],
		"signals": ["logs", "metrics", "traces"]
	}`))
	createCheckoutResponse := httptest.NewRecorder()
	router.ServeHTTP(createCheckoutResponse, createCheckoutRequest)
	require.Equal(t, stdhttp.StatusCreated, createCheckoutResponse.Code)

	createBillingRequest := httptest.NewRequest(stdhttp.MethodPost, "/v1/analyses", bytes.NewBufferString(`{
		"goal": "investigate billing errors",
		"timeWindow": {
			"start": "2026-05-12T10:00:00Z",
			"end": "2026-05-12T11:00:00Z"
		},
		"affectedServices": ["billing-service"],
		"signals": ["metrics"]
	}`))
	createBillingResponse := httptest.NewRecorder()
	router.ServeHTTP(createBillingResponse, createBillingRequest)
	require.Equal(t, stdhttp.StatusCreated, createBillingResponse.Code)

	listRequest := httptest.NewRequest(stdhttp.MethodGet, "/v1/analyses?service=checkout-service&limit=1", nil)
	listResponse := httptest.NewRecorder()
	router.ServeHTTP(listResponse, listRequest)
	require.Equal(t, stdhttp.StatusOK, listResponse.Code)

	var payload WrapperDtoResponde[AnalysisListResponseDto]
	require.NoError(t, json.Unmarshal(listResponse.Body.Bytes(), &payload))
	require.Len(t, payload.Data.Items, 1)
	assert.Equal(t, "analysis-000001", payload.Data.Items[0].ID)
	require.NotNil(t, payload.Metadata.Pagination)
	assert.Equal(t, 1, payload.Metadata.Pagination.Limit)
	assert.Equal(t, 0, payload.Metadata.Pagination.Offset)
	assert.Empty(t, payload.Metadata.Pagination.Next)
}

func TestRouterRejectsInvalidAnalysisListFilter(t *testing.T) {
	t.Parallel()

	router := newTestRouter()
	request := httptest.NewRequest(stdhttp.MethodGet, "/v1/analyses?severity=urgent", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	require.Equal(t, stdhttp.StatusBadRequest, response.Code)

	var payload WrapperDtoResponde[ErrorResponse]
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	assert.Equal(t, "invalid_analysis_filter", payload.Data.Code)
	assert.Contains(t, payload.Data.Message, "severity")
}

func TestRouterReturnsValidationDetailsOnPayloadFailure(t *testing.T) {
	t.Parallel()

	router := newTestRouter()
	request := httptest.NewRequest(stdhttp.MethodPost, "/v1/analyses", bytes.NewBufferString(`{
		"affectedServices": ["checkout-service"]
	}`))
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	require.Equal(t, stdhttp.StatusBadRequest, response.Code)

	var payload WrapperDtoResponde[ErrorResponse]
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	assert.Equal(t, "invalid_request", payload.Data.Code)
	require.NotEmpty(t, payload.Data.Details)

	missingGoal := false
	for _, detail := range payload.Data.Details {
		if detail.Field == "Goal" || detail.Field == "goal" {
			missingGoal = true
			assert.Equal(t, "required", detail.Rule)
		}
	}
	assert.True(t, missingGoal, "expected goal field to be reported as required")
}

func TestRouterPropagatesRequestIDFromHeader(t *testing.T) {
	t.Parallel()

	router := newTestRouter()
	request := httptest.NewRequest(stdhttp.MethodGet, "/health", nil)
	request.Header.Set("X-Request-Id", "req-from-client")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	require.Equal(t, stdhttp.StatusOK, response.Code)
	assert.Equal(t, "req-from-client", response.Header().Get("X-Request-Id"))

	var payload WrapperDtoResponde[HealthResponse]
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	assert.Equal(t, "req-from-client", payload.Metadata.RequestID)
}

func TestRouterRejectsBodyLargerThanLimit(t *testing.T) {
	t.Parallel()

	repository := fake.NewAnalysisRepository()
	analysis := usecase.NewAnalysis(
		fake.NewSignalCollector(),
		fake.NewAnalysisGenerator(),
		repository,
		fake.NewAnalysisContextCache(),
		6*time.Hour,
		fake.NewIDGenerator("analysis"),
	)
	chat := usecase.NewChat(repository, fake.NewAnalysisContextCache(), 6*time.Hour, repository, fake.NewChatResponder())

	router := NewRouter(analysis, chat, RouterOptions{
		Logger:             slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout:     5 * time.Second,
		MaxRequestBodyByte: 16,
	})

	request := httptest.NewRequest(stdhttp.MethodPost, "/v1/analyses", bytes.NewBufferString(`{"goal":"this body is intentionally large enough to trip the limit"}`))
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, stdhttp.StatusBadRequest, response.Code)

	var payload WrapperDtoResponde[ErrorResponse]
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	assert.Equal(t, "invalid_json", payload.Data.Code)
}

func TestRouterReturnsNotFoundForMissingAnalysis(t *testing.T) {
	t.Parallel()

	router := newTestRouter()
	request := httptest.NewRequest(stdhttp.MethodGet, "/v1/analyses/analysis-missing", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	require.Equal(t, stdhttp.StatusNotFound, response.Code)

	var payload WrapperDtoResponde[ErrorResponse]
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	assert.Equal(t, "analysis_not_found", payload.Data.Code)
}

func TestRouterRejectsOutOfScopeChatQuestion(t *testing.T) {
	t.Parallel()

	router := newTestRouter()
	request := httptest.NewRequest(stdhttp.MethodPost, "/v1/analyses/analysis-000001/chat", bytes.NewBufferString(`{
		"question": "What is the capital of France?"
	}`))
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	require.Equal(t, stdhttp.StatusBadRequest, response.Code)

	var payload WrapperDtoResponde[ErrorResponse]
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	assert.Equal(t, "question_out_of_scope", payload.Data.Code)
}

func TestRouterReturnsPersistedChatHistory(t *testing.T) {
	t.Parallel()

	router := newTestRouter()
	createRequest := httptest.NewRequest(stdhttp.MethodPost, "/v1/analyses", bytes.NewBufferString(`{
		"goal": "investigate checkout latency",
		"timeWindow": {
			"start": "2026-05-12T10:00:00Z",
			"end": "2026-05-12T11:00:00Z"
		},
		"affectedServices": ["checkout-service"],
		"signals": ["logs", "metrics", "traces"]
	}`))
	createResponse := httptest.NewRecorder()
	router.ServeHTTP(createResponse, createRequest)
	require.Equal(t, stdhttp.StatusCreated, createResponse.Code)

	chatRequest := httptest.NewRequest(stdhttp.MethodPost, "/v1/analyses/analysis-000001/chat", bytes.NewBufferString(`{
		"question": "Which evidence supports this analysis?"
	}`))
	chatResponse := httptest.NewRecorder()
	router.ServeHTTP(chatResponse, chatRequest)
	require.Equal(t, stdhttp.StatusOK, chatResponse.Code)

	historyRequest := httptest.NewRequest(stdhttp.MethodGet, "/v1/analyses/analysis-000001/chat", nil)
	historyResponse := httptest.NewRecorder()
	router.ServeHTTP(historyResponse, historyRequest)
	require.Equal(t, stdhttp.StatusOK, historyResponse.Code)

	var payload WrapperDtoResponde[ChatHistoryResponseDto]
	require.NoError(t, json.Unmarshal(historyResponse.Body.Bytes(), &payload))
	require.Len(t, payload.Data.Messages, 2)
	assert.Equal(t, "user", payload.Data.Messages[0].Role)
	assert.Equal(t, "assistant", payload.Data.Messages[1].Role)
}

func TestRouterExposesMetricsEndpoint(t *testing.T) {
	t.Parallel()

	router := newTestRouter()
	request := httptest.NewRequest(stdhttp.MethodGet, "/metrics", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.NotEqual(t, stdhttp.StatusNotFound, response.Code)
}

func newTestRouter() stdhttp.Handler {
	repository := fake.NewAnalysisRepository()
	analysis := usecase.NewAnalysis(
		fake.NewSignalCollector(),
		fake.NewAnalysisGenerator(),
		repository,
		fake.NewAnalysisContextCache(),
		6*time.Hour,
		fake.NewIDGenerator("analysis"),
	)
	chat := usecase.NewChat(repository, fake.NewAnalysisContextCache(), 6*time.Hour, repository, fake.NewChatResponder())

	return NewRouter(analysis, chat, RouterOptions{
		Logger:             slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout:     5 * time.Second,
		MaxRequestBodyByte: 1 << 20,
		Metrics: stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, _ *stdhttp.Request) {
			writer.WriteHeader(stdhttp.StatusOK)
		}),
	})
}
