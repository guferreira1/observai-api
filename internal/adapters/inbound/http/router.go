package http

import (
	"encoding/json"
	stdhttp "net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-playground/validator/v10"
	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/core/usecase"
)

// Router handles HTTP requests for ObservAI API.
type Router struct {
	mux      chi.Router
	analysis *usecase.Analysis
	chat     *usecase.Chat
	validate *validator.Validate
	nextID   atomic.Uint64
	metrics  stdhttp.Handler
}

// NewRouter creates the ObservAI HTTP router.
func NewRouter(analysis *usecase.Analysis, chat *usecase.Chat, metrics stdhttp.Handler) *Router {
	router := &Router{
		mux:      chi.NewRouter(),
		analysis: analysis,
		chat:     chat,
		validate: validator.New(validator.WithRequiredStructEnabled()),
		metrics:  metrics,
	}

	router.routes()
	return router
}

// ServeHTTP routes HTTP requests to API handlers.
func (router *Router) ServeHTTP(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	router.mux.ServeHTTP(writer, request)
}

func (router *Router) routes() {
	router.mux.Use(middleware.Recoverer)
	router.mux.Get("/health", router.handleHealth)
	router.mux.Post("/v1/analyses", router.handleCreateAnalysis)
	router.mux.Post("/v1/analyses/{analysisID}/chat", router.handleChat)
	router.mux.Get("/v1/analyses/{analysisID}/chat", router.handleChatHistory)
	router.mux.Handle("/metrics", router.metrics)
}

func (router *Router) handleHealth(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusOK, HealthResponse{Status: "ok"})
}

func (router *Router) handleCreateAnalysis(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	var dto AnalysisRequestDto
	if err := json.NewDecoder(request.Body).Decode(&dto); err != nil {
		router.writeError(writer, requestID, startedAt, stdhttp.StatusBadRequest, "invalid_json", "request body must be valid JSON")
		return
	}

	if err := router.validate.Struct(dto); err != nil {
		router.writeError(writer, requestID, startedAt, stdhttp.StatusBadRequest, "invalid_request", "request validation failed")
		return
	}

	result, err := router.analysis.Analyze(request.Context(), toDomainAnalysisRequest(dto))
	if err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}

	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusCreated, toAnalysisResponseDto(result))
}

func (router *Router) handleChat(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)
	analysisID := strings.TrimSpace(chi.URLParam(request, "analysisID"))
	if analysisID == "" {
		router.writeError(writer, requestID, startedAt, stdhttp.StatusNotFound, "not_found", "analysis not found")
		return
	}

	var dto ChatRequestDto
	if err := json.NewDecoder(request.Body).Decode(&dto); err != nil {
		router.writeError(writer, requestID, startedAt, stdhttp.StatusBadRequest, "invalid_json", "request body must be valid JSON")
		return
	}

	if err := router.validate.Struct(dto); err != nil {
		router.writeError(writer, requestID, startedAt, stdhttp.StatusBadRequest, "invalid_request", "request validation failed")
		return
	}

	answer, err := router.chat.Ask(request.Context(), domain.ChatQuestion{
		AnalysisID: analysisID,
		Question:   dto.Question,
	})
	if err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}

	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusOK, toChatResponseDto(answer))
}

func (router *Router) handleChatHistory(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)
	analysisID := strings.TrimSpace(chi.URLParam(request, "analysisID"))
	if analysisID == "" {
		router.writeError(writer, requestID, startedAt, stdhttp.StatusNotFound, "not_found", "analysis not found")
		return
	}

	messages, err := router.chat.History(request.Context(), analysisID)
	if err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}

	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusOK, toChatHistoryResponseDto(messages))
}

func (router *Router) writeDomainError(writer stdhttp.ResponseWriter, requestID string, startedAt time.Time, err error) {
	response := mapDomainError(err)
	router.writeError(writer, requestID, startedAt, response.status, response.code, response.message)
}

func (router *Router) writeSuccess(writer stdhttp.ResponseWriter, requestID string, startedAt time.Time, status int, data any) {
	router.writeJSON(writer, status, WrapperDtoResponde[any]{
		Data: data,
		Metadata: ResponseMetadata{
			RequestID:        requestID,
			ProcessingTimeMs: time.Since(startedAt).Milliseconds(),
			Provider: ProviderSummary{
				Observability: []string{"fake"},
				LLM:           "fake",
				Mode:          "local",
			},
		},
	})
}

func (router *Router) writeError(writer stdhttp.ResponseWriter, requestID string, startedAt time.Time, status int, code string, message string) {
	router.writeJSON(writer, status, WrapperDtoResponde[ErrorResponse]{
		Data: ErrorResponse{
			Code:    code,
			Message: message,
		},
		Metadata: ResponseMetadata{
			RequestID:        requestID,
			ProcessingTimeMs: time.Since(startedAt).Milliseconds(),
			Provider: ProviderSummary{
				Mode: "local",
			},
		},
	})
}

func (router *Router) writeJSON(writer stdhttp.ResponseWriter, status int, payload any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(payload)
}

func (router *Router) requestID(request *stdhttp.Request) string {
	id := strings.TrimSpace(request.Header.Get("X-Request-Id"))
	if id != "" {
		return id
	}

	next := router.nextID.Add(1)
	return "request-" + time.Now().UTC().Format("20060102150405") + "-" + strconv.FormatUint(next, 10)
}
