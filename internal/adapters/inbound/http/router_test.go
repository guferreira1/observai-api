package http

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	stdhttp "net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/guferreira1/observai-api/internal/adapters/outbound/inmemory"
	"github.com/guferreira1/observai-api/internal/adapters/outbound/testfakes"
	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/core/usecase"
	"github.com/guferreira1/observai-api/internal/platform/crypto"
	"github.com/guferreira1/observai-api/internal/platform/health"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRouterSubmitAnalysisReturnsAcceptedJob(t *testing.T) {
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

	require.Equal(t, stdhttp.StatusAccepted, response.Code)
	assert.Equal(t, "/v1/jobs/analysis-000001", response.Header().Get("Location"))

	var payload WrapperDtoResponde[AnalysisJobAcceptedDto]
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	assert.Equal(t, "analysis-000001", payload.Data.JobID)
	assert.Equal(t, "/v1/jobs/analysis-000001", payload.Data.StatusURL)
}

func TestRouterGetAnalysisJobReturnsCompletedStatus(t *testing.T) {
	t.Parallel()

	router := newTestRouter()
	submit := httptest.NewRequest(stdhttp.MethodPost, "/v1/analyses", bytes.NewBufferString(`{
		"goal": "investigate checkout latency",
		"timeWindow": {
			"start": "2026-05-12T10:00:00Z",
			"end": "2026-05-12T11:00:00Z"
		},
		"affectedServices": ["checkout-service"],
		"signals": ["logs", "metrics", "traces"]
	}`))
	submitResponse := httptest.NewRecorder()
	router.ServeHTTP(submitResponse, submit)
	require.Equal(t, stdhttp.StatusAccepted, submitResponse.Code)

	statusRequest := httptest.NewRequest(stdhttp.MethodGet, "/v1/jobs/analysis-000001", nil)
	statusResponse := httptest.NewRecorder()
	router.ServeHTTP(statusResponse, statusRequest)
	require.Equal(t, stdhttp.StatusOK, statusResponse.Code)

	var payload WrapperDtoResponde[AnalysisJobStatusDto]
	require.NoError(t, json.Unmarshal(statusResponse.Body.Bytes(), &payload))
	assert.Equal(t, "analysis-000001", payload.Data.JobID)
	assert.Equal(t, "completed", payload.Data.Status)
	assert.Equal(t, "done", payload.Data.Phase)
	assert.Equal(t, 100, payload.Data.ProgressPercent)
	assert.Equal(t, "analysis-000001", payload.Data.AnalysisID)
	assert.Equal(t, "/v1/analyses/analysis-000001", payload.Data.AnalysisURL)
}

func TestRouterGetAnalysisJobReturnsNotFound(t *testing.T) {
	t.Parallel()

	router := newTestRouter()
	request := httptest.NewRequest(stdhttp.MethodGet, "/v1/jobs/job-missing", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)
	require.Equal(t, stdhttp.StatusNotFound, response.Code)

	var payload WrapperDtoResponde[ErrorResponse]
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	assert.Equal(t, "analysis_job_not_found", payload.Data.Code)
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
	require.Equal(t, stdhttp.StatusAccepted, createResponse.Code)

	getRequest := httptest.NewRequest(stdhttp.MethodGet, "/v1/analyses/analysis-000001", nil)
	getResponse := httptest.NewRecorder()
	router.ServeHTTP(getResponse, getRequest)
	require.Equal(t, stdhttp.StatusOK, getResponse.Code)

	var payload WrapperDtoResponde[AnalysisResponseDto]
	require.NoError(t, json.Unmarshal(getResponse.Body.Bytes(), &payload))
	assert.Equal(t, "analysis-000001", payload.Data.ID)
	assert.Equal(t, "analysis-000001", payload.Data.JobID)
	assert.Equal(t, "high", payload.Data.Severity)
	require.NotEmpty(t, payload.Data.Evidence)
	assert.NotEmpty(t, payload.Data.Evidence[0].Severity)
	assert.NotEmpty(t, payload.Data.Evidence[0].CorrelationID)
	assert.NotEmpty(t, payload.Data.Evidence[0].TraceID)
	require.NotEmpty(t, payload.Data.RecommendedActions)
	assert.NotEmpty(t, payload.Data.RecommendedActions[0].EvidenceIDs)
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
	require.Equal(t, stdhttp.StatusAccepted, createCheckoutResponse.Code)

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
	require.Equal(t, stdhttp.StatusAccepted, createBillingResponse.Code)

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
	assert.Equal(t, 1, payload.Metadata.Pagination.Total)
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
	require.Len(t, payload.Data.Details, 1)
	assert.Equal(t, "severity", payload.Data.Details[0].Field)
	assert.Equal(t, "enum", payload.Data.Details[0].Rule)
	assert.NotEmpty(t, payload.Metadata.RequestID)
}

func TestRouterRejectsUnsupportedSignalFilter(t *testing.T) {
	t.Parallel()

	router := newTestRouter()
	request := httptest.NewRequest(stdhttp.MethodGet, "/v1/analyses?signal=events", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, stdhttp.StatusBadRequest, response.Code)

	var payload WrapperDtoResponde[ErrorResponse]
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	assert.Equal(t, "invalid_analysis_filter", payload.Data.Code)
	require.Len(t, payload.Data.Details, 1)
	assert.Equal(t, "signal", payload.Data.Details[0].Field)
	assert.Equal(t, "enum", payload.Data.Details[0].Rule)
}

func TestRouterRejectsUnsupportedSortFilter(t *testing.T) {
	t.Parallel()

	router := newTestRouter()
	request := httptest.NewRequest(stdhttp.MethodGet, "/v1/analyses?sort=cost", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, stdhttp.StatusBadRequest, response.Code)

	var payload WrapperDtoResponde[ErrorResponse]
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	assert.Equal(t, "invalid_analysis_filter", payload.Data.Code)
}

func TestRouterRejectsMalformedTimeFilter(t *testing.T) {
	t.Parallel()

	router := newTestRouter()
	request := httptest.NewRequest(stdhttp.MethodGet, "/v1/analyses?from=2026-13-32", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, stdhttp.StatusBadRequest, response.Code)

	var payload WrapperDtoResponde[ErrorResponse]
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	assert.Equal(t, "invalid_analysis_filter", payload.Data.Code)
	require.Len(t, payload.Data.Details, 1)
	assert.Equal(t, "from", payload.Data.Details[0].Field)
	assert.Equal(t, "rfc3339", payload.Data.Details[0].Rule)
}

func TestRouterRejectsInvalidAnalysisListLimit(t *testing.T) {
	t.Parallel()

	router := newTestRouter()
	request := httptest.NewRequest(stdhttp.MethodGet, "/v1/analyses?limit=-3", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	require.Equal(t, stdhttp.StatusBadRequest, response.Code)

	var payload WrapperDtoResponde[ErrorResponse]
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	assert.Equal(t, "invalid_analysis_filter", payload.Data.Code)
	require.Len(t, payload.Data.Details, 1)
	assert.Equal(t, "limit", payload.Data.Details[0].Field)
	assert.Equal(t, "non_negative_integer", payload.Data.Details[0].Rule)
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
	assert.Equal(t, "Some submitted fields are invalid. Review the highlighted fields and try again.", payload.Data.Message)
	require.NotEmpty(t, payload.Data.Details)

	missingGoal := false
	for _, detail := range payload.Data.Details {
		if detail.Field == "goal" {
			missingGoal = true
			assert.Equal(t, "required", detail.Rule)
			assert.Equal(t, "This field is required.", detail.Message)
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

	repository := inmemory.NewAnalysisRepository()
	analysis := usecase.NewAnalysis(
		testfakes.NewSignalCollector(),
		testfakes.NewAnalysisGenerator(),
		repository,
		inmemory.NewAnalysisContextCache(),
		6*time.Hour,
		testfakes.NewIDGenerator("analysis"),
	)
	chat := usecase.NewChat(repository, inmemory.NewAnalysisContextCache(), 6*time.Hour, repository, testfakes.NewChatResponder())

	router := NewRouter(analysis, chat, RouterOptions{
		Logger:             slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout:     5 * time.Second,
		MaxRequestBodyByte: 16,
	})

	request := httptest.NewRequest(stdhttp.MethodPost, "/v1/analyses", bytes.NewBufferString(`{"goal":"this body is intentionally large enough to trip the limit"}`))
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, stdhttp.StatusRequestEntityTooLarge, response.Code)

	var payload WrapperDtoResponde[ErrorResponse]
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	assert.Equal(t, "request_body_too_large", payload.Data.Code)
}

func TestRouterRejectsBodyWithUnknownFields(t *testing.T) {
	t.Parallel()

	router := newTestRouter()
	request := httptest.NewRequest(stdhttp.MethodPost, "/v1/analyses", bytes.NewBufferString(`{
		"goal": "investigate checkout latency",
		"timeWindow": {"start": "2026-05-12T10:00:00Z", "end": "2026-05-12T11:00:00Z"},
		"affectedServices": ["checkout-service"],
		"signals": ["logs"],
		"unexpectedField": "boom"
	}`))
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	require.Equal(t, stdhttp.StatusBadRequest, response.Code)

	var payload WrapperDtoResponde[ErrorResponse]
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	assert.Equal(t, "invalid_json", payload.Data.Code)
	assert.Equal(t, `Remove unsupported field "unexpectedField" from the request and try again.`, payload.Data.Message)
	require.NotEmpty(t, payload.Data.Details)
	assert.Equal(t, "unexpectedField", payload.Data.Details[0].Field)
	assert.Equal(t, "unknown_field", payload.Data.Details[0].Rule)
	assert.Equal(t, "This field is not accepted by this endpoint.", payload.Data.Details[0].Message)
}

func TestRouterLogsSetupAdminUnknownField(t *testing.T) {
	t.Parallel()

	var logBuffer bytes.Buffer
	userRepository := inmemory.NewUserRepository()
	setup := usecase.NewSetup(
		userRepository,
		usecase.NewUser(userRepository, nil, crypto.NewBcryptPasswordHasher(4), testfakes.NewIDGenerator("user")),
		nil,
	)
	router := NewRouter(nil, nil, RouterOptions{
		Logger:         slog.New(slog.NewTextHandler(&logBuffer, nil)),
		RequestTimeout: 5 * time.Second,
		Setup:          setup,
	})
	request := httptest.NewRequest(stdhttp.MethodPost, "/v1/setup/admin", bytes.NewBufferString(`{
		"email": "admin@observai.io",
		"password": "CorrectHorse42",
		"confirmPassword": "CorrectHorse42"
	}`))
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	require.Equal(t, stdhttp.StatusBadRequest, response.Code)
	var payload WrapperDtoResponde[ErrorResponse]
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	assert.Equal(t, "invalid_json", payload.Data.Code)
	assert.Equal(t, `Remove unsupported field "confirmPassword" from the request and try again.`, payload.Data.Message)
	assert.Contains(t, logBuffer.String(), "http error")
	assert.Contains(t, logBuffer.String(), "invalid_json")
	assert.Contains(t, logBuffer.String(), "confirmPassword")
}

func TestRouterBootstrapAdminAcceptsName(t *testing.T) {
	t.Parallel()

	router := newSetupAuthTestRouter(t)
	request := httptest.NewRequest(stdhttp.MethodPost, "/v1/setup/admin", bytes.NewBufferString(`{
		"name": "Gustavo Ferreira",
		"email": "admin@observai.io",
		"password": "CorrectHorse42"
	}`))
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	require.Equal(t, stdhttp.StatusCreated, response.Code)
	assert.NotEmpty(t, response.Header().Values("Set-Cookie"))
	var payload WrapperDtoResponde[SessionResponseDto]
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	assert.Equal(t, "Gustavo Ferreira", payload.Data.User.Name)
	assert.Equal(t, "admin@observai.io", payload.Data.User.Email)
	assert.NotEmpty(t, payload.Data.CSRFToken)
	assert.NotEmpty(t, payload.Data.ExpiresAt)
}

func TestRouterBootstrapAdminAllowsSecondAdminWhileSetupOpen(t *testing.T) {
	t.Parallel()

	router := newSetupAuthTestRouter(t)
	firstRequest := httptest.NewRequest(stdhttp.MethodPost, "/v1/setup/admin", bytes.NewBufferString(`{
		"name": "Gustavo Ferreira",
		"email": "admin@observai.io",
		"password": "CorrectHorse42"
	}`))
	firstResponse := httptest.NewRecorder()
	router.ServeHTTP(firstResponse, firstRequest)
	require.Equal(t, stdhttp.StatusCreated, firstResponse.Code)

	secondRequest := httptest.NewRequest(stdhttp.MethodPost, "/v1/setup/admin", bytes.NewBufferString(`{
		"name": "Second Admin",
		"email": "second@observai.io",
		"password": "AnotherP@ss1"
	}`))
	secondResponse := httptest.NewRecorder()
	router.ServeHTTP(secondResponse, secondRequest)

	require.Equal(t, stdhttp.StatusCreated, secondResponse.Code)
	var payload WrapperDtoResponde[SessionResponseDto]
	require.NoError(t, json.Unmarshal(secondResponse.Body.Bytes(), &payload))
	assert.Equal(t, "Second Admin", payload.Data.User.Name)
	assert.Equal(t, "second@observai.io", payload.Data.User.Email)
}

func TestRouterBootstrapAdminExistingAdminCredentialsIssueSession(t *testing.T) {
	t.Parallel()

	router := newSetupAuthTestRouter(t)
	createRequest := httptest.NewRequest(stdhttp.MethodPost, "/v1/setup/admin", bytes.NewBufferString(`{
		"name": "Gustavo Ferreira",
		"email": "admin@observai.io",
		"password": "CorrectHorse42"
	}`))
	createResponse := httptest.NewRecorder()
	router.ServeHTTP(createResponse, createRequest)
	require.Equal(t, stdhttp.StatusCreated, createResponse.Code)

	retryRequest := httptest.NewRequest(stdhttp.MethodPost, "/v1/setup/admin", bytes.NewBufferString(`{
		"name": "Gustavo Ferreira",
		"email": "admin@observai.io",
		"password": "CorrectHorse42"
	}`))
	retryResponse := httptest.NewRecorder()
	router.ServeHTTP(retryResponse, retryRequest)

	require.Equal(t, stdhttp.StatusOK, retryResponse.Code)
	var payload WrapperDtoResponde[SessionResponseDto]
	require.NoError(t, json.Unmarshal(retryResponse.Body.Bytes(), &payload))
	assert.Equal(t, "admin@observai.io", payload.Data.User.Email)
	assert.NotEmpty(t, payload.Data.CSRFToken)
}

func TestRouterFormatsBootstrapAdminTimestampsInConfiguredTimezone(t *testing.T) {
	t.Parallel()

	userRepository := inmemory.NewUserRepository()
	setup := usecase.NewSetup(
		userRepository,
		usecase.NewUser(userRepository, nil, crypto.NewBcryptPasswordHasher(4), testfakes.NewIDGenerator("user")),
		nil,
	)
	router := NewRouter(nil, nil, RouterOptions{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout: 5 * time.Second,
		Setup:          setup,
		TimeLocation:   time.FixedZone("America/Sao_Paulo", -3*60*60),
	})
	request := httptest.NewRequest(stdhttp.MethodPost, "/v1/setup/admin", bytes.NewBufferString(`{
		"name": "Gustavo Ferreira",
		"email": "admin@observai.io",
		"password": "CorrectHorse42"
	}`))
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	require.Equal(t, stdhttp.StatusCreated, response.Code)
	var payload WrapperDtoResponde[UserResponseDto]
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	assert.Contains(t, payload.Data.CreatedAt, "-03:00")
	assert.Contains(t, payload.Data.UpdatedAt, "-03:00")
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

func TestRouterAnswersOutOfScopeChatQuestion(t *testing.T) {
	t.Parallel()

	router := newTestRouter()
	createRequest := httptest.NewRequest(stdhttp.MethodPost, "/v1/analyses", bytes.NewBufferString(`{
		"goal": "investigate checkout latency",
		"timeWindow": {"start": "2026-05-12T10:00:00Z", "end": "2026-05-12T11:00:00Z"},
		"affectedServices": ["checkout-service"]
	}`))
	createResponse := httptest.NewRecorder()
	router.ServeHTTP(createResponse, createRequest)
	require.Equal(t, stdhttp.StatusAccepted, createResponse.Code)

	request := httptest.NewRequest(stdhttp.MethodPost, "/v1/analyses/analysis-000001/chat", bytes.NewBufferString(`{
		"question": "What is the capital of France?"
	}`))
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	require.Equal(t, stdhttp.StatusOK, response.Code)

	var payload WrapperDtoResponde[ChatResponseDto]
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	assert.Equal(t, "analysis-000001", payload.Data.AnalysisID)
	assert.Contains(t, payload.Data.Answer, "I can only answer questions about the active ObservAI analysis")
	assert.Empty(t, payload.Data.Evidence)
}

func TestRouterAcceptsContextualChatFollowUp(t *testing.T) {
	t.Parallel()

	router := newTestRouter()
	createRequest := httptest.NewRequest(stdhttp.MethodPost, "/v1/analyses", bytes.NewBufferString(`{
		"goal": "investigate checkout latency",
		"timeWindow": {"start": "2026-05-12T10:00:00Z", "end": "2026-05-12T11:00:00Z"},
		"affectedServices": ["checkout-service"]
	}`))
	createResponse := httptest.NewRecorder()
	router.ServeHTTP(createResponse, createRequest)
	require.Equal(t, stdhttp.StatusAccepted, createResponse.Code)

	request := httptest.NewRequest(stdhttp.MethodPost, "/v1/analyses/analysis-000001/chat", bytes.NewBufferString(`{
		"question": "E agora?"
	}`))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, stdhttp.StatusOK, response.Code)

	var payload WrapperDtoResponde[ChatResponseDto]
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	assert.Equal(t, "analysis-000001", payload.Data.AnalysisID)
	assert.NotEmpty(t, payload.Data.Answer)
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
	require.Equal(t, stdhttp.StatusAccepted, createResponse.Code)

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

type flushableRecorder struct {
	*httptest.ResponseRecorder
}

func (recorder *flushableRecorder) Flush() {}

func TestRouterChatStreamsSSEEventsWhenRequested(t *testing.T) {
	t.Parallel()

	router := newTestRouter()

	createRequest := httptest.NewRequest(stdhttp.MethodPost, "/v1/analyses", bytes.NewBufferString(`{
		"goal": "investigate checkout latency",
		"timeWindow": {"start": "2026-05-12T10:00:00Z", "end": "2026-05-12T11:00:00Z"},
		"affectedServices": ["checkout-service"]
	}`))
	createResponse := httptest.NewRecorder()
	router.ServeHTTP(createResponse, createRequest)
	require.Equal(t, stdhttp.StatusAccepted, createResponse.Code)

	chatRequest := httptest.NewRequest(stdhttp.MethodPost, "/v1/analyses/analysis-000001/chat", bytes.NewBufferString(`{"question": "which evidence supports this analysis?"}`))
	chatRequest.Header.Set("Accept", "text/event-stream")
	chatResponse := &flushableRecorder{ResponseRecorder: httptest.NewRecorder()}
	router.ServeHTTP(chatResponse, chatRequest)
	require.Equal(t, stdhttp.StatusOK, chatResponse.Code)
	assert.Equal(t, "text/event-stream", chatResponse.Header().Get("Content-Type"))

	body := chatResponse.Body.String()
	assert.Contains(t, body, "event: token")
	assert.Contains(t, body, "event: done")
}

func TestRouterExportsAnalysisAsMarkdown(t *testing.T) {
	t.Parallel()

	router := newTestRouter()

	createRequest := httptest.NewRequest(stdhttp.MethodPost, "/v1/analyses", bytes.NewBufferString(`{
		"goal": "investigate checkout latency",
		"timeWindow": {"start": "2026-05-12T10:00:00Z", "end": "2026-05-12T11:00:00Z"},
		"affectedServices": ["checkout-service"]
	}`))
	createResponse := httptest.NewRecorder()
	router.ServeHTTP(createResponse, createRequest)
	require.Equal(t, stdhttp.StatusAccepted, createResponse.Code)

	exportRequest := httptest.NewRequest(stdhttp.MethodGet, "/v1/analyses/analysis-000001/export?format=md", nil)
	exportResponse := httptest.NewRecorder()
	router.ServeHTTP(exportResponse, exportRequest)
	require.Equal(t, stdhttp.StatusOK, exportResponse.Code)
	assert.Contains(t, exportResponse.Header().Get("Content-Type"), "text/markdown")
	assert.Contains(t, exportResponse.Body.String(), "# ObservAI analysis analysis-000001")
	assert.Contains(t, exportResponse.Header().Get("Content-Disposition"), "analysis-000001.md")
}

func TestRouterRejectsUnknownExportFormat(t *testing.T) {
	t.Parallel()

	router := newTestRouter()

	createRequest := httptest.NewRequest(stdhttp.MethodPost, "/v1/analyses", bytes.NewBufferString(`{
		"goal": "investigate checkout latency",
		"timeWindow": {"start": "2026-05-12T10:00:00Z", "end": "2026-05-12T11:00:00Z"},
		"affectedServices": ["checkout-service"]
	}`))
	createResponse := httptest.NewRecorder()
	router.ServeHTTP(createResponse, createRequest)
	require.Equal(t, stdhttp.StatusAccepted, createResponse.Code)

	request := httptest.NewRequest(stdhttp.MethodGet, "/v1/analyses/analysis-000001/export?format=pdf", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, stdhttp.StatusBadRequest, response.Code)

	var payload WrapperDtoResponde[ErrorResponse]
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	assert.Equal(t, "invalid_export_format", payload.Data.Code)
}

func TestRouterReturnsTraceInsights(t *testing.T) {
	t.Parallel()

	repository := inmemory.NewAnalysisRepository()
	require.NoError(t, repository.Save(context.Background(), domain.AnalysisResult{
		ID:      "analysis-000001",
		TraceID: "trace-000001",
	}))
	traces := usecase.NewTrace(repository, testfakes.NewTraceProvider())
	router := NewRouter(nil, nil, RouterOptions{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout: 5 * time.Second,
		Trace:          traces,
	})

	tracesRequest := httptest.NewRequest(stdhttp.MethodGet, "/v1/analyses/analysis-000001/traces", nil)
	tracesResponse := httptest.NewRecorder()
	router.ServeHTTP(tracesResponse, tracesRequest)
	require.Equal(t, stdhttp.StatusOK, tracesResponse.Code)

	var payload WrapperDtoResponde[TraceInsightsResponseDto]
	require.NoError(t, json.Unmarshal(tracesResponse.Body.Bytes(), &payload))
	require.NotEmpty(t, payload.Data.Spans)
	require.NotEmpty(t, payload.Data.CriticalPathSpanIDs)
	require.NotEmpty(t, payload.Data.SlowestSpanIDs)
	assert.Equal(t, "span-root", payload.Data.CriticalPathSpanIDs[0])
	assert.NotEmpty(t, payload.Data.DependencyEdges)
}

func TestRouterReturnsTraceNotFoundWhenAnalysisHasNoTraceReference(t *testing.T) {
	t.Parallel()

	repository := inmemory.NewAnalysisRepository()
	require.NoError(t, repository.Save(context.Background(), domain.AnalysisResult{ID: "analysis-000001"}))
	traces := usecase.NewTrace(repository, testfakes.NewTraceProvider())
	router := NewRouter(nil, nil, RouterOptions{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout: 5 * time.Second,
		Trace:          traces,
	})

	request := httptest.NewRequest(stdhttp.MethodGet, "/v1/analyses/analysis-000001/traces", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, stdhttp.StatusNotFound, response.Code)

	var payload WrapperDtoResponde[ErrorResponse]
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	assert.Equal(t, "trace_not_found", payload.Data.Code)
	assert.Equal(t, "analysis does not contain a trace reference", payload.Data.Message)
}

func TestRouterCancelAnalysisJobMarksCanceled(t *testing.T) {
	t.Parallel()

	repository := inmemory.NewAnalysisRepository()
	jobRepository := inmemory.NewAnalysisJobRepository()
	enqueuer := inmemory.NewJobEnqueuer(inmemory.NewAnalysisQueue(1))
	analysis := usecase.NewAnalysis(
		testfakes.NewSignalCollector(),
		testfakes.NewAnalysisGenerator(),
		repository,
		inmemory.NewAnalysisContextCache(),
		6*time.Hour,
		testfakes.NewIDGenerator("analysis"),
	).WithAsyncBackend(jobRepository, enqueuer)

	chat := usecase.NewChat(repository, inmemory.NewAnalysisContextCache(), 6*time.Hour, repository, testfakes.NewChatResponder()).
		WithLocker(inmemory.NewAnalysisLocker())

	router := NewRouter(analysis, chat, RouterOptions{
		Logger:             slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout:     5 * time.Second,
		MaxRequestBodyByte: 1 << 20,
	})

	submit := httptest.NewRequest(stdhttp.MethodPost, "/v1/analyses", bytes.NewBufferString(`{
		"goal": "investigate checkout latency",
		"timeWindow": {"start": "2026-05-12T10:00:00Z", "end": "2026-05-12T11:00:00Z"},
		"affectedServices": ["checkout-service"]
	}`))
	submitResponse := httptest.NewRecorder()
	router.ServeHTTP(submitResponse, submit)
	require.Equal(t, stdhttp.StatusAccepted, submitResponse.Code)

	cancelRequest := httptest.NewRequest(stdhttp.MethodDelete, "/v1/jobs/analysis-000001", nil)
	cancelResponse := httptest.NewRecorder()
	router.ServeHTTP(cancelResponse, cancelRequest)
	require.Equal(t, stdhttp.StatusAccepted, cancelResponse.Code)

	var payload WrapperDtoResponde[AnalysisJobStatusDto]
	require.NoError(t, json.Unmarshal(cancelResponse.Body.Bytes(), &payload))
	assert.Equal(t, "analysis-000001", payload.Data.JobID)
	assert.Equal(t, "canceled", payload.Data.Status)

	idempotentRequest := httptest.NewRequest(stdhttp.MethodDelete, "/v1/jobs/analysis-000001", nil)
	idempotentResponse := httptest.NewRecorder()
	router.ServeHTTP(idempotentResponse, idempotentRequest)
	require.Equal(t, stdhttp.StatusAccepted, idempotentResponse.Code)

	var idempotent WrapperDtoResponde[AnalysisJobStatusDto]
	require.NoError(t, json.Unmarshal(idempotentResponse.Body.Bytes(), &idempotent))
	assert.Equal(t, "canceled", idempotent.Data.Status)
}

func TestRouterCancelAnalysisJobReturnsNotFound(t *testing.T) {
	t.Parallel()

	router := newTestRouter()
	request := httptest.NewRequest(stdhttp.MethodDelete, "/v1/jobs/job-missing", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, stdhttp.StatusNotFound, response.Code)

	var payload WrapperDtoResponde[ErrorResponse]
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	assert.Equal(t, "analysis_job_not_found", payload.Data.Code)
}

func TestRouterExposesCapabilities(t *testing.T) {
	t.Parallel()

	repository := inmemory.NewAnalysisRepository()
	jobRepository := inmemory.NewAnalysisJobRepository()
	enqueuer := inmemory.NewSynchronousJobEnqueuer()
	analysis := usecase.NewAnalysis(
		testfakes.NewSignalCollector(),
		testfakes.NewAnalysisGenerator(),
		repository,
		inmemory.NewAnalysisContextCache(),
		6*time.Hour,
		testfakes.NewIDGenerator("analysis"),
	).WithAsyncBackend(jobRepository, enqueuer)
	enqueuer.SetHandler(analysis.RunAnalysisJob)

	chat := usecase.NewChat(repository, inmemory.NewAnalysisContextCache(), 6*time.Hour, repository, testfakes.NewChatResponder()).
		WithLocker(inmemory.NewAnalysisLocker())

	capabilities := CapabilitiesResponse{
		Mode:    "local",
		Version: "test",
		LLM:     CapabilityLLM{Provider: "fake"},
		Observability: []CapabilityProvider{
			{Provider: "fake", Signals: []string{"logs", "metrics"}},
		},
		Limits: CapabilityLimits{
			HTTPRequestTimeoutMs: 5000,
			HTTPMaxBodyBytes:     1 << 20,
		},
	}

	router := NewRouter(analysis, chat, RouterOptions{
		Logger:             slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout:     5 * time.Second,
		MaxRequestBodyByte: 1 << 20,
		Capabilities:       capabilities,
		Provider: ProviderSummary{
			Mode:          "local",
			LLM:           "fake",
			Observability: []string{"fake"},
		},
	})

	request := httptest.NewRequest(stdhttp.MethodGet, "/v1/capabilities", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, stdhttp.StatusOK, response.Code)

	var payload WrapperDtoResponde[CapabilitiesResponse]
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	assert.Equal(t, "local", payload.Data.Mode)
	assert.Equal(t, "test", payload.Data.Version)
	assert.Equal(t, "fake", payload.Data.LLM.Provider)
	require.Len(t, payload.Data.Observability, 1)
	assert.Equal(t, "fake", payload.Data.Observability[0].Provider)
	assert.Equal(t, int64(5000), payload.Data.Limits.HTTPRequestTimeoutMs)
	assert.Equal(t, "local", payload.Metadata.Provider.Mode)
	assert.Equal(t, "fake", payload.Metadata.Provider.LLM)
}

func TestRouterExposesDynamicCapabilities(t *testing.T) {
	t.Parallel()

	capabilities := CapabilitiesResponse{
		Mode:    "local",
		Version: "test",
		LLM:     CapabilityLLM{Provider: "ChatGPT", Model: "gpt-4o-mini"},
		Observability: []CapabilityProvider{
			{Provider: "Prometheus", Signals: []string{"metrics"}},
			{Provider: "Jaeger", Signals: []string{"traces"}},
		},
	}
	provider := ProviderSummary{
		Mode:          "local",
		LLM:           "ChatGPT",
		Observability: []string{"Prometheus", "Jaeger"},
	}

	router := NewRouter(nil, nil, RouterOptions{
		Logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout:   5 * time.Second,
		CapabilitiesFunc: func() CapabilitiesResponse { return capabilities },
		ProviderFunc:     func() ProviderSummary { return provider },
	})

	request := httptest.NewRequest(stdhttp.MethodGet, "/v1/capabilities", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, stdhttp.StatusOK, response.Code)

	var payload WrapperDtoResponde[CapabilitiesResponse]
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	assert.Equal(t, "ChatGPT", payload.Data.LLM.Provider)
	require.Len(t, payload.Data.Observability, 2)
	assert.Equal(t, "Jaeger", payload.Data.Observability[1].Provider)
	assert.Equal(t, "ChatGPT", payload.Metadata.Provider.LLM)
	assert.Equal(t, []string{"Prometheus", "Jaeger"}, payload.Metadata.Provider.Observability)
}

func TestRouterHealthzAndReadyzUseEnvelope(t *testing.T) {
	t.Parallel()

	checker := health.NewChecker(time.Second, health.ProbeFunc{
		ProbeName: "database",
		Fn:        func(context.Context) error { return nil },
	}, health.ProbeFunc{
		ProbeName: "redis",
		Fn:        func(context.Context) error { return nil },
	})
	router := NewRouter(nil, nil, RouterOptions{
		Logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		ReadinessChecker: checker,
	})

	healthRequest := httptest.NewRequest(stdhttp.MethodGet, "/healthz", nil)
	healthResponse := httptest.NewRecorder()
	router.ServeHTTP(healthResponse, healthRequest)
	require.Equal(t, stdhttp.StatusOK, healthResponse.Code)

	var healthPayload WrapperDtoResponde[HealthResponse]
	require.NoError(t, json.Unmarshal(healthResponse.Body.Bytes(), &healthPayload))
	assert.Equal(t, "ok", healthPayload.Data.Status)
	assert.NotEmpty(t, healthPayload.Metadata.RequestID)

	readyRequest := httptest.NewRequest(stdhttp.MethodGet, "/readyz", nil)
	readyResponse := httptest.NewRecorder()
	router.ServeHTTP(readyResponse, readyRequest)
	require.Equal(t, stdhttp.StatusOK, readyResponse.Code)

	var readyPayload WrapperDtoResponde[ReadinessResponseDto]
	require.NoError(t, json.Unmarshal(readyResponse.Body.Bytes(), &readyPayload))
	assert.Equal(t, "ok", readyPayload.Data.Status)
	require.Len(t, readyPayload.Data.Checks, 2)
	assert.Equal(t, "database", readyPayload.Data.Checks[0].Name)
	assert.Equal(t, "redis", readyPayload.Data.Checks[1].Name)
	assert.NotEmpty(t, readyPayload.Metadata.RequestID)
}

func TestRouterReportsDisabledOptionalRoutes(t *testing.T) {
	t.Parallel()

	router := NewRouter(nil, nil, RouterOptions{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	cases := []struct {
		method string
		path   string
	}{
		{stdhttp.MethodGet, "/v1/setup/status"},
		{stdhttp.MethodPost, "/v1/auth/login"},
		{stdhttp.MethodGet, "/v1/me"},
		{stdhttp.MethodGet, "/v1/admin/users"},
		{stdhttp.MethodGet, "/v1/admin/providers"},
		{stdhttp.MethodGet, "/v1/admin/llm-providers"},
	}

	for _, testCase := range cases {
		t.Run(testCase.method+" "+testCase.path, func(t *testing.T) {
			request := httptest.NewRequest(testCase.method, testCase.path, nil)
			response := httptest.NewRecorder()

			router.ServeHTTP(response, request)

			require.Equal(t, stdhttp.StatusServiceUnavailable, response.Code)

			var payload WrapperDtoResponde[ErrorResponse]
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
			assert.Equal(t, "feature_not_configured", payload.Data.Code)
			assert.NotEmpty(t, payload.Metadata.RequestID)
		})
	}
}

func TestRouterAcceptsTelemetry(t *testing.T) {
	t.Parallel()

	router := newTestRouter()
	request := httptest.NewRequest(stdhttp.MethodPost, "/v1/telemetry", bytes.NewBufferString(`{
		"events": [
			{"name": "page.view"},
			{"name": "analysis.opened"}
		]
	}`))
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	require.Equal(t, stdhttp.StatusAccepted, response.Code)
	var payload WrapperDtoResponde[TelemetryAcceptedResponseDto]
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	assert.True(t, payload.Data.Accepted)
	assert.Equal(t, 2, payload.Data.EventCount)
}

func TestRecoverMiddlewareUsesConfiguredProviderSummary(t *testing.T) {
	t.Parallel()

	provider := ProviderSummary{
		Mode:          "prod",
		LLM:           "ollama-prod",
		Observability: []string{"prometheus-prod"},
	}
	handler := recoverMiddleware(slog.New(slog.NewTextHandler(io.Discard, nil)), func() ProviderSummary {
		return provider
	})(stdhttp.HandlerFunc(func(stdhttp.ResponseWriter, *stdhttp.Request) {
		panic("boom")
	}))
	request := httptest.NewRequest(stdhttp.MethodGet, "/panic", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	require.Equal(t, stdhttp.StatusInternalServerError, response.Code)
	var payload WrapperDtoResponde[ErrorResponse]
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	assert.Equal(t, "internal_error", payload.Data.Code)
	assert.Equal(t, provider.Mode, payload.Metadata.Provider.Mode)
	assert.Equal(t, provider.LLM, payload.Metadata.Provider.LLM)
	assert.Equal(t, provider.Observability, payload.Metadata.Provider.Observability)
}

func TestRateLimitMiddlewareUsesConfiguredProviderSummary(t *testing.T) {
	t.Parallel()

	provider := ProviderSummary{
		Mode:          "prod",
		LLM:           "openai-prod",
		Observability: []string{"loki-prod"},
	}
	limiter := newRateLimiter(RateLimitConfig{RequestsPerSecond: 0.01, Burst: 1})
	handler := rateLimitMiddleware(limiter, func() ProviderSummary {
		return provider
	})(stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, _ *stdhttp.Request) {
		writer.WriteHeader(stdhttp.StatusNoContent)
	}))
	first := httptest.NewRequest(stdhttp.MethodGet, "/limited", nil)
	first.RemoteAddr = "203.0.113.10:1000"
	firstResponse := httptest.NewRecorder()
	second := httptest.NewRequest(stdhttp.MethodGet, "/limited", nil)
	second.RemoteAddr = "203.0.113.10:1001"
	secondResponse := httptest.NewRecorder()

	handler.ServeHTTP(firstResponse, first)
	handler.ServeHTTP(secondResponse, second)

	require.Equal(t, stdhttp.StatusNoContent, firstResponse.Code)
	require.Equal(t, stdhttp.StatusTooManyRequests, secondResponse.Code)
	var payload WrapperDtoResponde[ErrorResponse]
	require.NoError(t, json.Unmarshal(secondResponse.Body.Bytes(), &payload))
	assert.Equal(t, "rate_limited", payload.Data.Code)
	assert.Equal(t, provider.Mode, payload.Metadata.Provider.Mode)
	assert.Equal(t, provider.LLM, payload.Metadata.Provider.LLM)
	assert.Equal(t, provider.Observability, payload.Metadata.Provider.Observability)
}

func newTestRouter() stdhttp.Handler {
	router, _ := newTestRouterWithBackend()
	return router
}

func newSetupAuthTestRouter(t *testing.T) stdhttp.Handler {
	t.Helper()

	userRepository := inmemory.NewUserRepository()
	refreshRepository := inmemory.NewRefreshTokenRepository()
	signer, err := crypto.NewJWTSigner(bytes.Repeat([]byte{0xab}, crypto.MinJWTSecretLength), "observai-api")
	require.NoError(t, err)

	hasher := crypto.NewBcryptPasswordHasher(4)
	userAdmin := usecase.NewUser(userRepository, refreshRepository, hasher, testfakes.NewIDGenerator("user"))
	setup := usecase.NewSetup(userRepository, userAdmin, nil)
	sessions := usecase.NewAuth(userRepository, refreshRepository, signer, hasher, testfakes.NewIDGenerator("session"), usecase.AuthOptions{
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: time.Hour,
	})

	return NewRouter(nil, nil, RouterOptions{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout: 5 * time.Second,
		Setup:          setup,
		Sessions:       sessions,
	})
}

func newTestRouterWithBackend() (stdhttp.Handler, *inmemory.AnalysisJobRepository) {
	repository := inmemory.NewAnalysisRepository()
	jobRepository := inmemory.NewAnalysisJobRepository()
	enqueuer := inmemory.NewSynchronousJobEnqueuer()

	analysis := usecase.NewAnalysis(
		testfakes.NewSignalCollector(),
		testfakes.NewAnalysisGenerator(),
		repository,
		inmemory.NewAnalysisContextCache(),
		6*time.Hour,
		testfakes.NewIDGenerator("analysis"),
	).WithAsyncBackend(jobRepository, enqueuer)
	enqueuer.SetHandler(analysis.RunAnalysisJob)

	chat := usecase.NewChat(repository, inmemory.NewAnalysisContextCache(), 6*time.Hour, repository, testfakes.NewChatResponder()).
		WithLocker(inmemory.NewAnalysisLocker()).
		WithFeedbackRepository(inmemory.NewChatFeedbackRepository())

	traces := usecase.NewTrace(repository, testfakes.NewTraceProvider())

	router := NewRouter(analysis, chat, RouterOptions{
		Logger:             slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout:     5 * time.Second,
		MaxRequestBodyByte: 1 << 20,
		Metrics: stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, _ *stdhttp.Request) {
			writer.WriteHeader(stdhttp.StatusOK)
		}),
		Trace: traces,
	})
	return router, jobRepository
}
