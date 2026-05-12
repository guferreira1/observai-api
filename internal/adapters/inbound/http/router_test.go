package http

import (
	"bytes"
	"encoding/json"
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

	return NewRouter(analysis, chat, stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, _ *stdhttp.Request) {
		writer.WriteHeader(stdhttp.StatusOK)
	}))
}
