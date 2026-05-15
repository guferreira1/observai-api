package http

import (
	"encoding/json"
	"errors"
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
	Auth               AuthConfig
	Cookies            CookieConfig
	Metrics            stdhttp.Handler
	Liveness           stdhttp.Handler
	Readiness          stdhttp.Handler
	Capabilities       CapabilitiesResponse
	Provider           ProviderSummary
	Trace              *usecase.Trace
	APIKeys            *usecase.APIKey
	Webhooks           *usecase.WebhookSubscriptions
	AuditLog           *usecase.AuditLog
	Retention          *usecase.AnalysisRetention
	Sessions           *usecase.Auth
	Users              *usecase.User
	Setup              *usecase.Setup
	ProviderConfigs    *usecase.ProviderConfig
	LLMConfigs         *usecase.LLMConfig
}

// Router handles HTTP requests for ObservAI API.
type Router struct {
	mux             chi.Router
	analysis        *usecase.Analysis
	chat            *usecase.Chat
	traces          *usecase.Trace
	apiKeys         *usecase.APIKey
	webhooks        *usecase.WebhookSubscriptions
	auditLog        *usecase.AuditLog
	retention       *usecase.AnalysisRetention
	sessions        *usecase.Auth
	users           *usecase.User
	setup           *usecase.Setup
	providerConfigs *usecase.ProviderConfig
	llmConfigs      *usecase.LLMConfig
	validate        *validator.Validate
	logger          *slog.Logger
	options         RouterOptions
}

// NewRouter creates the ObservAI HTTP router.
//
// The trace use case is optional. When nil, GET /v1/analyses/{id}/traces
// returns a 404 with `not_found` so clients can discover that the running
// instance has no trace provider configured.
func NewRouter(analysis *usecase.Analysis, chat *usecase.Chat, opts RouterOptions) *Router {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}

	router := &Router{
		mux:             chi.NewRouter(),
		analysis:        analysis,
		chat:            chat,
		traces:          opts.Trace,
		apiKeys:         opts.APIKeys,
		webhooks:        opts.Webhooks,
		auditLog:        opts.AuditLog,
		retention:       opts.Retention,
		sessions:        opts.Sessions,
		users:           opts.Users,
		setup:           opts.Setup,
		providerConfigs: opts.ProviderConfigs,
		llmConfigs:      opts.LLMConfigs,
		validate:        validator.New(validator.WithRequiredStructEnabled()),
		logger:          opts.Logger,
		options:         opts,
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
	router.mux.Use(authMiddleware(router.options.Auth))
	router.mux.Use(csrfMiddleware())
	router.mux.Use(auditMiddleware(router.auditLog, router.logger))
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
	router.mux.Get("/v1/capabilities", router.handleCapabilities)

	reader := RequireRole(domain.RoleViewer, domain.RoleOperator, domain.RoleAdmin)
	writer := RequireRole(domain.RoleOperator, domain.RoleAdmin)
	admin := RequireRole(domain.RoleAdmin)

	router.mux.Method(stdhttp.MethodGet, "/v1/analyses", reader(router.handleListAnalyses))
	router.mux.Method(stdhttp.MethodGet, "/v1/analyses/stats", reader(router.handleAnalysisStats))
	router.mux.Method(stdhttp.MethodGet, "/v1/services", reader(router.handleServicesAutocomplete))
	router.mux.Method(stdhttp.MethodPost, "/v1/analyses", writer(router.handleSubmitAnalysis))
	router.mux.Method(stdhttp.MethodGet, "/v1/analyses/{analysisID}", reader(router.handleGetAnalysis))
	router.mux.Method(stdhttp.MethodGet, "/v1/analyses/{analysisID}/export", reader(router.handleExportAnalysis))
	router.mux.Method(stdhttp.MethodGet, "/v1/analyses/{analysisID}/traces", reader(router.handleGetTraces))
	router.mux.Method(stdhttp.MethodGet, "/v1/jobs/{jobID}", reader(router.handleGetAnalysisJob))
	router.mux.Method(stdhttp.MethodDelete, "/v1/jobs/{jobID}", writer(router.handleCancelAnalysisJob))
	router.mux.Method(stdhttp.MethodPost, "/v1/analyses/{analysisID}/chat", writer(router.handleChat))
	router.mux.Method(stdhttp.MethodGet, "/v1/analyses/{analysisID}/chat", reader(router.handleChatHistory))
	router.mux.Method(stdhttp.MethodPost, "/v1/analyses/{analysisID}/chat/{messageID}/feedback", writer(router.handleChatFeedback))
	router.mux.Method(stdhttp.MethodPost, "/v1/analyses/{analysisID}/chat/{messageID}/regenerate", writer(router.handleChatRegenerate))

	if router.setup != nil {
		router.mux.Method(stdhttp.MethodGet, "/v1/setup/status", stdhttp.HandlerFunc(router.handleSetupStatus))
		router.mux.Method(stdhttp.MethodPost, "/v1/setup/admin", stdhttp.HandlerFunc(router.handleBootstrapAdmin))
	}

	if router.sessions != nil {
		router.mux.Method(stdhttp.MethodPost, "/v1/auth/login", stdhttp.HandlerFunc(router.handleLogin))
		router.mux.Method(stdhttp.MethodPost, "/v1/auth/logout", stdhttp.HandlerFunc(router.handleLogout))
		router.mux.Method(stdhttp.MethodPost, "/v1/auth/refresh", stdhttp.HandlerFunc(router.handleRefresh))
		router.mux.Method(stdhttp.MethodGet, "/v1/me", reader(router.handleMe))
		router.mux.Method(stdhttp.MethodPatch, "/v1/me", reader(router.handleUpdateMe))
		router.mux.Method(stdhttp.MethodPost, "/v1/me/password", reader(router.handleChangePassword))
	}

	if router.apiKeys != nil {
		router.mux.Method(stdhttp.MethodPost, "/v1/admin/keys", admin(router.handleIssueAPIKey))
		router.mux.Method(stdhttp.MethodGet, "/v1/admin/keys", admin(router.handleListAPIKeys))
		router.mux.Method(stdhttp.MethodDelete, "/v1/admin/keys/{keyID}", admin(router.handleRevokeAPIKey))
	}
	if router.webhooks != nil {
		router.mux.Method(stdhttp.MethodPost, "/v1/admin/webhooks", admin(router.handleCreateWebhook))
		router.mux.Method(stdhttp.MethodGet, "/v1/admin/webhooks", admin(router.handleListWebhooks))
		router.mux.Method(stdhttp.MethodDelete, "/v1/admin/webhooks/{webhookID}", admin(router.handleDeleteWebhook))
		router.mux.Method(stdhttp.MethodPost, "/v1/admin/webhooks/{webhookID}/test", admin(router.handleTestWebhook))
		router.mux.Method(stdhttp.MethodGet, "/v1/admin/webhook-deliveries", admin(router.handleListWebhookDeliveries))
		router.mux.Method(stdhttp.MethodPost, "/v1/admin/webhook-deliveries/{deliveryID}/retry", admin(router.handleRetryWebhookDelivery))
	}
	if router.auditLog != nil {
		router.mux.Method(stdhttp.MethodGet, "/v1/admin/audit", admin(router.handleListAudit))
	}
	if router.retention != nil {
		router.mux.Method(stdhttp.MethodDelete, "/v1/analyses/{analysisID}", writer(router.handleDeleteAnalysis))
		router.mux.Method(stdhttp.MethodDelete, "/v1/admin/analyses", admin(router.handlePurgeAnalyses))
	}
	if router.users != nil {
		router.mux.Method(stdhttp.MethodGet, "/v1/admin/users", admin(router.handleListUsers))
		router.mux.Method(stdhttp.MethodPost, "/v1/admin/users", admin(router.handleCreateUser))
		router.mux.Method(stdhttp.MethodGet, "/v1/admin/users/{userID}", admin(router.handleGetUser))
		router.mux.Method(stdhttp.MethodPatch, "/v1/admin/users/{userID}", admin(router.handleUpdateUser))
		router.mux.Method(stdhttp.MethodDelete, "/v1/admin/users/{userID}", admin(router.handleDeleteUser))
	}
	if router.providerConfigs != nil {
		router.mux.Method(stdhttp.MethodGet, "/v1/admin/providers", admin(router.handleListProviderConfigs))
		router.mux.Method(stdhttp.MethodPost, "/v1/admin/providers", admin(router.handleCreateProviderConfig))
		router.mux.Method(stdhttp.MethodGet, "/v1/admin/providers/{providerID}", admin(router.handleGetProviderConfig))
		router.mux.Method(stdhttp.MethodPatch, "/v1/admin/providers/{providerID}", admin(router.handleUpdateProviderConfig))
		router.mux.Method(stdhttp.MethodDelete, "/v1/admin/providers/{providerID}", admin(router.handleDeleteProviderConfig))
		router.mux.Method(stdhttp.MethodPost, "/v1/admin/providers/{providerID}/test", admin(router.handleTestProviderConfig))
		router.mux.Method(stdhttp.MethodPost, "/v1/admin/providers/{providerID}/activate", admin(router.handleActivateProviderConfig))
		router.mux.Method(stdhttp.MethodPost, "/v1/admin/providers/{providerID}/deactivate", admin(router.handleDeactivateProviderConfig))
	}
	if router.llmConfigs != nil {
		router.mux.Method(stdhttp.MethodGet, "/v1/admin/llm-providers", admin(router.handleListLLMConfigs))
		router.mux.Method(stdhttp.MethodPost, "/v1/admin/llm-providers", admin(router.handleCreateLLMConfig))
		router.mux.Method(stdhttp.MethodGet, "/v1/admin/llm-providers/{llmID}", admin(router.handleGetLLMConfig))
		router.mux.Method(stdhttp.MethodPatch, "/v1/admin/llm-providers/{llmID}", admin(router.handleUpdateLLMConfig))
		router.mux.Method(stdhttp.MethodDelete, "/v1/admin/llm-providers/{llmID}", admin(router.handleDeleteLLMConfig))
		router.mux.Method(stdhttp.MethodPost, "/v1/admin/llm-providers/{llmID}/test", admin(router.handleTestLLMConfig))
		router.mux.Method(stdhttp.MethodPost, "/v1/admin/llm-providers/{llmID}/activate", admin(router.handleActivateLLMConfig))
	}

	if router.options.Metrics != nil {
		router.mux.Handle("/metrics", router.options.Metrics)
	}
}

func (router *Router) handleHealth(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusOK, HealthResponse{Status: "ok"})
}

func (router *Router) handleCapabilities(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusOK, router.options.Capabilities)
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

func (router *Router) handleCancelAnalysisJob(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)
	jobID := strings.TrimSpace(chi.URLParam(request, "jobID"))
	if jobID == "" {
		router.writeError(writer, requestID, startedAt, stdhttp.StatusNotFound, "analysis_job_not_found", "analysis job not found")
		return
	}

	ctx := logger.With(request.Context(), slog.String("jobId", jobID))
	*request = *request.WithContext(ctx)

	job, err := router.analysis.CancelJob(request.Context(), jobID)
	if err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}

	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusAccepted, toAnalysisJobStatusDto(job))
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

func (router *Router) handleServicesAutocomplete(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	limit, err := parseOptionalPositiveInt(request.URL.Query().Get("limit"), "limit")
	if err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}

	services, err := router.analysis.Services(request.Context(), request.URL.Query().Get("q"), limit)
	if err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}

	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusOK, ServicesResponseDto{Items: services})
}

func (router *Router) handleAnalysisStats(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	filter, err := parseAnalysisStatsFilter(request)
	if err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}

	stats, err := router.analysis.Stats(request.Context(), filter)
	if err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}

	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusOK, toAnalysisStatsResponseDto(stats))
}

func parseAnalysisStatsFilter(request *stdhttp.Request) (domain.AnalysisStatsFilter, error) {
	query := request.URL.Query()

	from, err := parseOptionalTime(query.Get("from"), "from")
	if err != nil {
		return domain.AnalysisStatsFilter{}, err
	}

	to, err := parseOptionalTime(query.Get("to"), "to")
	if err != nil {
		return domain.AnalysisStatsFilter{}, err
	}

	severity := domain.Severity(strings.TrimSpace(query.Get("severity")))
	if severity != "" && !isValidSeverity(severity) {
		return domain.AnalysisStatsFilter{}, invalidFilterError("severity", "enum",
			fmt.Sprintf("severity %q is not supported", severity))
	}

	return domain.AnalysisStatsFilter{
		From:     from,
		To:       to,
		Service:  strings.TrimSpace(query.Get("service")),
		Severity: severity,
	}, nil
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

func (router *Router) handleExportAnalysis(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)
	analysisID := strings.TrimSpace(chi.URLParam(request, "analysisID"))
	if analysisID == "" {
		router.writeError(writer, requestID, startedAt, stdhttp.StatusNotFound, "not_found", "analysis not found")
		return
	}

	format := usecase.AnalysisExportFormat(strings.TrimSpace(request.URL.Query().Get("format")))
	if format == "" {
		format = usecase.AnalysisExportFormatJSON
	}

	ctx := logger.With(request.Context(), slog.String("analysisId", analysisID))
	*request = *request.WithContext(ctx)

	export, err := router.analysis.Export(request.Context(), analysisID, format)
	if err != nil {
		if errors.Is(err, usecase.ErrUnsupportedExportFormat) {
			router.writeError(writer, requestID, startedAt, stdhttp.StatusBadRequest, "invalid_export_format", err.Error())
			return
		}
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}

	writer.Header().Set("Content-Type", export.ContentType)
	writer.Header().Set("X-Request-Id", requestID)
	writer.Header().Set("Content-Disposition", "attachment; filename=\""+analysisID+"."+string(export.Format)+"\"")
	writer.WriteHeader(stdhttp.StatusOK)
	_, _ = writer.Write(export.Body)
}

func (router *Router) handleGetTraces(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)
	analysisID := strings.TrimSpace(chi.URLParam(request, "analysisID"))
	if analysisID == "" {
		router.writeError(writer, requestID, startedAt, stdhttp.StatusNotFound, "not_found", "analysis not found")
		return
	}

	if router.traces == nil {
		router.writeError(writer, requestID, startedAt, stdhttp.StatusNotFound, "not_found", "trace provider is not configured on this instance")
		return
	}

	ctx := logger.With(request.Context(), slog.String("analysisId", analysisID))
	*request = *request.WithContext(ctx)

	insights, err := router.traces.Get(request.Context(), analysisID)
	if err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}

	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusOK, toTraceInsightsResponseDto(insights))
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

	if wantsEventStream(request) {
		router.streamChat(writer, request, requestID, startedAt, analysisID, dto.Question)
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

func (router *Router) streamChat(writer stdhttp.ResponseWriter, request *stdhttp.Request, requestID string, startedAt time.Time, analysisID string, questionText string) {
	flusher, ok := writer.(stdhttp.Flusher)
	if !ok {
		router.writeError(writer, requestID, startedAt, stdhttp.StatusInternalServerError, "internal_error", "streaming is not supported on this connection")
		return
	}

	writer.Header().Set("Content-Type", "text/event-stream")
	writer.Header().Set("Cache-Control", "no-cache")
	writer.Header().Set("Connection", "keep-alive")
	writer.WriteHeader(stdhttp.StatusOK)
	flusher.Flush()

	sink := newSSEChatSink(writer, flusher)
	err := router.chat.AskStream(request.Context(), domain.ChatQuestion{
		AnalysisID: analysisID,
		Question:   questionText,
	}, sink)
	if err != nil {
		response := mapHTTPError(err)
		_ = sink.SendError(response.code, response.message)
		return
	}
}

func wantsEventStream(request *stdhttp.Request) bool {
	accept := strings.ToLower(request.Header.Get("Accept"))
	return strings.Contains(accept, "text/event-stream")
}

func (router *Router) handleChatHistory(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)
	analysisID := strings.TrimSpace(chi.URLParam(request, "analysisID"))
	if analysisID == "" {
		router.writeError(writer, requestID, startedAt, stdhttp.StatusNotFound, "not_found", "analysis not found")
		return
	}

	filter, err := parseChatHistoryFilter(request)
	if err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}

	ctx := logger.With(request.Context(), slog.String("analysisId", analysisID))
	*request = *request.WithContext(ctx)

	messages, err := router.chat.History(request.Context(), analysisID, filter)
	if err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}

	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusOK, toChatHistoryResponseDto(messages))
}

func (router *Router) handleChatRegenerate(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)
	analysisID := strings.TrimSpace(chi.URLParam(request, "analysisID"))
	messageID := strings.TrimSpace(chi.URLParam(request, "messageID"))
	if analysisID == "" || messageID == "" {
		router.writeError(writer, requestID, startedAt, stdhttp.StatusNotFound, "not_found", "chat message not found")
		return
	}
	ctx := logger.With(request.Context(), slog.String("analysisId", analysisID), slog.String("messageId", messageID))
	*request = *request.WithContext(ctx)

	answer, err := router.chat.Regenerate(request.Context(), analysisID, messageID)
	if err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}
	AnnotateAudit(request, AuditAnnotation{
		Action:       "chat.regenerated",
		ResourceType: "chat_message",
		ResourceID:   messageID,
		Metadata:     map[string]string{"analysisId": analysisID},
	})
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusOK, toChatResponseDto(answer))
}

func (router *Router) handleChatFeedback(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)
	analysisID := strings.TrimSpace(chi.URLParam(request, "analysisID"))
	messageID := strings.TrimSpace(chi.URLParam(request, "messageID"))
	if analysisID == "" {
		router.writeError(writer, requestID, startedAt, stdhttp.StatusNotFound, "not_found", "analysis not found")
		return
	}
	if messageID == "" {
		router.writeError(writer, requestID, startedAt, stdhttp.StatusNotFound, "not_found", "chat message not found")
		return
	}

	var dto ChatFeedbackRequestDto
	if err := decodeRequestBody(request, &dto); err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}
	if err := router.validate.Struct(dto); err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}

	ctx := logger.With(request.Context(), slog.String("analysisId", analysisID), slog.String("messageId", messageID))
	*request = *request.WithContext(ctx)

	feedback := domain.ChatFeedback{
		AnalysisID: analysisID,
		MessageID:  messageID,
		Useful:     *dto.Useful,
		Reason:     dto.Reason,
	}
	if err := router.chat.SubmitFeedback(request.Context(), feedback); err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}

	response := ChatFeedbackResponseDto{
		AnalysisID: feedback.AnalysisID,
		MessageID:  feedback.MessageID,
		Useful:     feedback.Useful,
		Reason:     feedback.Reason,
		CreatedAt:  time.Now().UTC(),
	}
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusCreated, response)
}

func parseChatHistoryFilter(request *stdhttp.Request) (domain.ChatHistoryFilter, error) {
	query := request.URL.Query()

	limit, err := parseOptionalPositiveInt(query.Get("limit"), "limit")
	if err != nil {
		return domain.ChatHistoryFilter{}, err
	}

	before, err := parseOptionalTime(query.Get("before"), "before")
	if err != nil {
		return domain.ChatHistoryFilter{}, err
	}

	return domain.ChatHistoryFilter{
		Limit:  limit,
		Before: before,
	}, nil
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
			Provider:         router.providerSummary(),
			Pagination:       pagination,
		},
	})
}

func (router *Router) providerSummary() ProviderSummary {
	if router.options.Provider.Mode != "" {
		return router.options.Provider
	}
	return ProviderSummary{
		Observability: []string{"fake"},
		LLM:           "fake",
		Mode:          "local",
	}
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
			Provider:         router.providerSummary(),
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
		return domain.AnalysisListFilter{}, invalidFilterError("severity", "enum",
			fmt.Sprintf("severity %q is not supported", severity))
	}

	signal, err := parseOptionalSignal(query.Get("signal"))
	if err != nil {
		return domain.AnalysisListFilter{}, err
	}

	from, err := parseOptionalTime(query.Get("from"), "from")
	if err != nil {
		return domain.AnalysisListFilter{}, err
	}

	to, err := parseOptionalTime(query.Get("to"), "to")
	if err != nil {
		return domain.AnalysisListFilter{}, err
	}

	sort, err := parseOptionalSort(query.Get("sort"))
	if err != nil {
		return domain.AnalysisListFilter{}, err
	}

	order, err := parseOptionalOrder(query.Get("order"))
	if err != nil {
		return domain.AnalysisListFilter{}, err
	}

	return domain.AnalysisListFilter{
		Limit:    limit,
		Offset:   offset,
		Severity: severity,
		Service:  strings.TrimSpace(query.Get("service")),
		Signal:   signal,
		Provider: strings.TrimSpace(query.Get("provider")),
		From:     from,
		To:       to,
		Query:    strings.TrimSpace(query.Get("q")),
		Sort:     sort,
		Order:    order,
	}, nil
}

func parseOptionalSignal(raw string) (domain.SignalType, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	signal := domain.SignalType(raw)
	if !domain.IsValidSignal(signal) {
		return "", invalidFilterError("signal", "enum",
			fmt.Sprintf("signal %q is not supported", raw))
	}
	return signal, nil
}

func parseOptionalTime(raw string, field string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	value, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, invalidFilterError(field, "rfc3339",
			fmt.Sprintf("%s must be a RFC3339 timestamp", field))
	}
	return value.UTC(), nil
}

func parseOptionalSort(raw string) (domain.AnalysisListSort, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	sort := domain.AnalysisListSort(raw)
	if !domain.IsValidAnalysisListSort(sort) {
		return "", invalidFilterError("sort", "enum",
			fmt.Sprintf("sort %q is not supported", raw))
	}
	return sort, nil
}

func parseOptionalOrder(raw string) (domain.AnalysisListOrder, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	order := domain.AnalysisListOrder(strings.ToLower(raw))
	if !domain.IsValidAnalysisListOrder(order) {
		return "", invalidFilterError("order", "enum",
			fmt.Sprintf("order %q is not supported", raw))
	}
	return order, nil
}

func parseOptionalPositiveInt(raw string, name string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, invalidFilterError(name, "non_negative_integer",
			fmt.Sprintf("%s must be a non-negative integer", name))
	}

	return value, nil
}

func invalidFilterError(field string, rule string, reason string) error {
	return errInvalidAnalysisFilter{
		Field:  field,
		Rule:   rule,
		Reason: reason,
	}
}

type errInvalidAnalysisFilter struct {
	Field  string
	Rule   string
	Reason string
}

func (err errInvalidAnalysisFilter) Error() string {
	return domain.ErrInvalidAnalysisFilter.Error() + ": " + err.Reason
}

func (err errInvalidAnalysisFilter) Unwrap() error {
	return domain.ErrInvalidAnalysisFilter
}

func isValidSeverity(severity domain.Severity) bool {
	return domain.IsValidSeverity(severity)
}

func toPagination(request *stdhttp.Request, result domain.AnalysisList) *Pagination {
	pagination := &Pagination{
		Limit:  result.Limit,
		Offset: result.Offset,
		Total:  result.Total,
	}

	if result.Limit <= 0 {
		return pagination
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
