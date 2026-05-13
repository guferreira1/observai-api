package http

import (
	"encoding/json"
	"fmt"
	"log/slog"
	stdhttp "net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-playground/validator/v10"
	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/core/usecase"
	"github.com/guferreira1/observai-api/internal/platform/logger"
)

// RouterOptions configures cross-cutting HTTP behavior shared by every route.
type RouterOptions struct {
	Logger             *slog.Logger
	RequestTimeout     time.Duration
	MaxRequestBodyByte int64
	RateLimit          RateLimitConfig
	Metrics            stdhttp.Handler
	Liveness           stdhttp.Handler
	Readiness          stdhttp.Handler
}

// Router handles HTTP requests for ObservAI API.
type Router struct {
	mux      chi.Router
	analysis *usecase.Analysis
	chat     *usecase.Chat
	validate *validator.Validate
	logger   *slog.Logger
	options  RouterOptions
}

// NewRouter creates the ObservAI HTTP router.
func NewRouter(analysis *usecase.Analysis, chat *usecase.Chat, opts RouterOptions) *Router {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}

	router := &Router{
		mux:      chi.NewRouter(),
		analysis: analysis,
		chat:     chat,
		validate: validator.New(validator.WithRequiredStructEnabled()),
		logger:   opts.Logger,
		options:  opts,
	}

	router.routes()
	return router
}

// ServeHTTP routes HTTP requests to API handlers.
func (router *Router) ServeHTTP(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	router.mux.ServeHTTP(writer, request)
}

func (router *Router) routes() {
	router.mux.Use(middleware.RequestID)
	router.mux.Use(requestIDMiddleware)
	router.mux.Use(middleware.RealIP)
	router.mux.Use(loggerMiddleware(router.logger))
	router.mux.Use(recoverMiddleware(router.logger))
	router.mux.Use(rateLimitMiddleware(newRateLimiter(router.options.RateLimit)))
	router.mux.Use(bodyLimitMiddleware(router.options.MaxRequestBodyByte))
	router.mux.Use(timeoutMiddleware(router.options.RequestTimeout))

	router.mux.Get("/health", router.handleHealth)
	if router.options.Liveness != nil {
		router.mux.Method(stdhttp.MethodGet, "/healthz", router.options.Liveness)
	}
	if router.options.Readiness != nil {
		router.mux.Method(stdhttp.MethodGet, "/readyz", router.options.Readiness)
	}
	router.mux.Get("/v1/openapi.yaml", router.handleOpenAPI)
	router.mux.Get("/v1/analyses", router.handleListAnalyses)
	router.mux.Post("/v1/analyses", router.handleSubmitAnalysis)
	router.mux.Get("/v1/analyses/{analysisID}", router.handleGetAnalysis)
	router.mux.Get("/v1/jobs/{jobID}", router.handleGetAnalysisJob)
	router.mux.Post("/v1/analyses/{analysisID}/chat", router.handleChat)
	router.mux.Get("/v1/analyses/{analysisID}/chat", router.handleChatHistory)

	if router.options.Metrics != nil {
		router.mux.Handle("/metrics", router.options.Metrics)
	}
}

func (router *Router) handleHealth(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusOK, HealthResponse{Status: "ok"})
}

func (router *Router) handleSubmitAnalysis(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	var dto AnalysisRequestDto
	if err := decodeRequestBody(request, &dto); err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}

	if err := router.validate.Struct(dto); err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}

	job, err := router.analysis.SubmitAnalysis(request.Context(), toDomainAnalysisRequest(dto))
	if err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}

	ctx := logger.With(request.Context(), slog.String("jobId", job.ID))
	*request = *request.WithContext(ctx)

	accepted := toAnalysisJobAcceptedDto(job)
	writer.Header().Set("Location", accepted.StatusURL)
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusAccepted, accepted)
}

func (router *Router) handleGetAnalysisJob(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)
	jobID := strings.TrimSpace(chi.URLParam(request, "jobID"))
	if jobID == "" {
		router.writeError(writer, requestID, startedAt, stdhttp.StatusNotFound, "analysis_job_not_found", "analysis job not found")
		return
	}

	ctx := logger.With(request.Context(), slog.String("jobId", jobID))
	*request = *request.WithContext(ctx)

	job, err := router.analysis.GetJob(request.Context(), jobID)
	if err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}

	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusOK, toAnalysisJobStatusDto(job))
}

func (router *Router) handleListAnalyses(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	filter, err := parseAnalysisListFilter(request)
	if err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}

	result, err := router.analysis.List(request.Context(), filter)
	if err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}

	router.writeSuccessWithPagination(
		writer,
		requestID,
		startedAt,
		stdhttp.StatusOK,
		toAnalysisListResponseDto(result),
		toPagination(request, result),
	)
}

func (router *Router) handleGetAnalysis(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)
	analysisID := strings.TrimSpace(chi.URLParam(request, "analysisID"))
	if analysisID == "" {
		router.writeError(writer, requestID, startedAt, stdhttp.StatusNotFound, "not_found", "analysis not found")
		return
	}

	ctx := logger.With(request.Context(), slog.String("analysisId", analysisID))
	*request = *request.WithContext(ctx)

	result, err := router.analysis.Get(request.Context(), analysisID)
	if err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}

	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusOK, toAnalysisResponseDto(result))
}

func (router *Router) handleChat(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)
	analysisID := strings.TrimSpace(chi.URLParam(request, "analysisID"))
	if analysisID == "" {
		router.writeError(writer, requestID, startedAt, stdhttp.StatusNotFound, "not_found", "analysis not found")
		return
	}

	ctx := logger.With(request.Context(), slog.String("analysisId", analysisID))
	*request = *request.WithContext(ctx)

	var dto ChatRequestDto
	if err := decodeRequestBody(request, &dto); err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}

	if err := router.validate.Struct(dto); err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
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

	ctx := logger.With(request.Context(), slog.String("analysisId", analysisID))
	*request = *request.WithContext(ctx)

	messages, err := router.chat.History(request.Context(), analysisID)
	if err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}

	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusOK, toChatHistoryResponseDto(messages))
}

func (router *Router) writeDomainError(writer stdhttp.ResponseWriter, requestID string, startedAt time.Time, err error) {
	response := mapHTTPError(err)
	router.writeErrorResponse(writer, requestID, startedAt, response)
}

func (router *Router) writeSuccess(writer stdhttp.ResponseWriter, requestID string, startedAt time.Time, status int, data any) {
	router.writeSuccessWithPagination(writer, requestID, startedAt, status, data, nil)
}

func (router *Router) writeSuccessWithPagination(writer stdhttp.ResponseWriter, requestID string, startedAt time.Time, status int, data any, pagination *Pagination) {
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
			Pagination: pagination,
		},
	})
}

func (router *Router) writeError(writer stdhttp.ResponseWriter, requestID string, startedAt time.Time, status int, code string, message string) {
	router.writeErrorResponse(writer, requestID, startedAt, httpErrorResponse{
		status:  status,
		code:    code,
		message: message,
	})
}

func (router *Router) writeErrorResponse(writer stdhttp.ResponseWriter, requestID string, startedAt time.Time, response httpErrorResponse) {
	router.writeJSON(writer, response.status, WrapperDtoResponde[ErrorResponse]{
		Data: ErrorResponse{
			Code:    response.code,
			Message: response.message,
			Details: response.details,
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
	return requestIDFromContext(request.Context())
}

func parseAnalysisListFilter(request *stdhttp.Request) (domain.AnalysisListFilter, error) {
	query := request.URL.Query()

	limit, err := parseOptionalPositiveInt(query.Get("limit"), "limit")
	if err != nil {
		return domain.AnalysisListFilter{}, err
	}

	offset, err := parseOptionalPositiveInt(query.Get("offset"), "offset")
	if err != nil {
		return domain.AnalysisListFilter{}, err
	}

	severity := domain.Severity(strings.TrimSpace(query.Get("severity")))
	if severity != "" && !isValidSeverity(severity) {
		return domain.AnalysisListFilter{}, fmt.Errorf("%w: severity %q is not supported", domain.ErrInvalidAnalysisFilter, severity)
	}

	return domain.AnalysisListFilter{
		Limit:    limit,
		Offset:   offset,
		Severity: severity,
		Service:  strings.TrimSpace(query.Get("service")),
	}, nil
}

func parseOptionalPositiveInt(raw string, name string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("%w: %s must be a non-negative integer", domain.ErrInvalidAnalysisFilter, name)
	}

	return value, nil
}

func isValidSeverity(severity domain.Severity) bool {
	return domain.IsValidSeverity(severity)
}

func toPagination(request *stdhttp.Request, result domain.AnalysisList) *Pagination {
	pagination := &Pagination{
		Limit:  result.Limit,
		Offset: result.Offset,
	}

	nextOffset := result.Offset + result.Limit
	if nextOffset >= result.Total {
		return pagination
	}

	query := request.URL.Query()
	query.Set("limit", strconv.Itoa(result.Limit))
	query.Set("offset", strconv.Itoa(nextOffset))
	pagination.Next = request.URL.Path + "?" + query.Encode()

	return pagination
}
