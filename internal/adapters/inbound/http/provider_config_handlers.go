package http

import (
	"errors"
	"fmt"
	stdhttp "net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/core/policy"
	"github.com/guferreira1/observai-api/internal/core/usecase"
)

// ProviderConfigRequestDto carries POST/PATCH /v1/admin/providers payloads.
type ProviderConfigRequestDto struct {
	Type        string            `json:"type" validate:"required"`
	Name        string            `json:"name" validate:"required"`
	URL         string            `json:"url" validate:"required,url"`
	TimeoutMs   int               `json:"timeoutMs,omitempty"`
	Signals     []string          `json:"signals,omitempty"`
	Options     map[string]string `json:"options,omitempty"`
	Credentials string            `json:"credentials,omitempty"`
	IsActive    bool              `json:"isActive,omitempty"`
}

// ProviderConfigResponseDto is the public projection of domain.ProviderConfig.
type ProviderConfigResponseDto struct {
	ID                string            `json:"id"`
	Type              string            `json:"type"`
	Name              string            `json:"name"`
	URL               string            `json:"url"`
	TimeoutMs         int               `json:"timeoutMs"`
	Signals           []string          `json:"signals"`
	Options           map[string]string `json:"options,omitempty"`
	CredentialsMasked string            `json:"credentialsMasked,omitempty"`
	HasCredentials    bool              `json:"hasCredentials"`
	IsActive          bool              `json:"isActive"`
	CreatedAt         string            `json:"createdAt"`
	UpdatedAt         string            `json:"updatedAt"`
}

// LLMConfigRequestDto carries POST/PATCH /v1/admin/llm-providers payloads.
type LLMConfigRequestDto struct {
	Type      string            `json:"type" validate:"required"`
	Name      string            `json:"name" validate:"required"`
	BaseURL   string            `json:"baseUrl" validate:"required,url"`
	Model     string            `json:"model" validate:"required"`
	TimeoutMs int               `json:"timeoutMs,omitempty"`
	Options   map[string]string `json:"options,omitempty"`
	APIKey    string            `json:"apiKey,omitempty"`
	IsActive  bool              `json:"isActive,omitempty"`
}

// LLMConfigResponseDto is the public projection of domain.LLMConfig.
type LLMConfigResponseDto struct {
	ID         string            `json:"id"`
	Type       string            `json:"type"`
	Name       string            `json:"name"`
	BaseURL    string            `json:"baseUrl"`
	Model      string            `json:"model"`
	TimeoutMs  int               `json:"timeoutMs"`
	Options    map[string]string `json:"options,omitempty"`
	APIKeyMask string            `json:"apiKeyMasked,omitempty"`
	HasAPIKey  bool              `json:"hasApiKey"`
	IsActive   bool              `json:"isActive"`
	CreatedAt  string            `json:"createdAt"`
	UpdatedAt  string            `json:"updatedAt"`
}

// TestConnectionResponseDto is the payload returned by the test-connection
// endpoints.
type TestConnectionResponseDto struct {
	Reached   bool   `json:"reached"`
	LatencyMs int64  `json:"latencyMs"`
	Error     string `json:"error,omitempty"`
}

func toProviderConfigResponseDto(config domain.ProviderConfig) ProviderConfigResponseDto {
	dto := ProviderConfigResponseDto{
		ID:             config.ID,
		Type:           string(config.Type),
		Name:           config.Name,
		URL:            config.URL,
		TimeoutMs:      int(config.Timeout / time.Millisecond),
		Signals:        config.Signals,
		Options:        config.Options,
		HasCredentials: config.CredentialsCiphertext != "",
		IsActive:       config.IsActive,
		CreatedAt:      config.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      config.UpdatedAt.Format(time.RFC3339),
	}
	if config.CredentialsCiphertext != "" {
		dto.CredentialsMasked = policy.MaskSecret(config.CredentialsCiphertext)
	}
	return dto
}

func toLLMConfigResponseDto(config domain.LLMConfig) LLMConfigResponseDto {
	dto := LLMConfigResponseDto{
		ID:        config.ID,
		Type:      string(config.Type),
		Name:      config.Name,
		BaseURL:   config.BaseURL,
		Model:     config.Model,
		TimeoutMs: int(config.Timeout / time.Millisecond),
		Options:   config.Options,
		HasAPIKey: config.APIKeyCipher != "",
		IsActive:  config.IsActive,
		CreatedAt: config.CreatedAt.Format(time.RFC3339),
		UpdatedAt: config.UpdatedAt.Format(time.RFC3339),
	}
	if config.APIKeyCipher != "" {
		dto.APIKeyMask = policy.MaskSecret(config.APIKeyCipher)
	}
	return dto
}

func toProviderConfigRequest(dto ProviderConfigRequestDto) usecase.ProviderConfigRequest {
	timeout := time.Duration(dto.TimeoutMs) * time.Millisecond
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	return usecase.ProviderConfigRequest{
		Type:        domain.ObservabilityProviderType(strings.ToLower(strings.TrimSpace(dto.Type))),
		Name:        dto.Name,
		URL:         dto.URL,
		Timeout:     timeout,
		Signals:     dto.Signals,
		Options:     dto.Options,
		Credentials: dto.Credentials,
		IsActive:    dto.IsActive,
	}
}

func toLLMConfigRequest(dto LLMConfigRequestDto) usecase.LLMConfigRequest {
	timeout := time.Duration(dto.TimeoutMs) * time.Millisecond
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return usecase.LLMConfigRequest{
		Type:     domain.LLMProviderType(strings.ToLower(strings.TrimSpace(dto.Type))),
		Name:     dto.Name,
		BaseURL:  dto.BaseURL,
		Model:    dto.Model,
		Timeout:  timeout,
		Options:  dto.Options,
		APIKey:   dto.APIKey,
		IsActive: dto.IsActive,
	}
}

func (router *Router) handleCreateProviderConfig(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	var dto ProviderConfigRequestDto
	if err := decodeRequestBody(request, &dto); err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}
	if err := router.validate.Struct(dto); err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}

	config, err := router.providerConfigs.Create(request.Context(), toProviderConfigRequest(dto))
	if err != nil {
		router.writeProviderConfigError(writer, requestID, startedAt, err)
		return
	}
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusCreated, toProviderConfigResponseDto(config))
}

func (router *Router) handleListProviderConfigs(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	limit, offset := paginationFromQuery(request)
	configs, err := router.providerConfigs.List(request.Context(), limit, offset)
	if err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}
	items := make([]ProviderConfigResponseDto, 0, len(configs))
	for _, config := range configs {
		items = append(items, toProviderConfigResponseDto(config))
	}
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusOK, items)
}

func (router *Router) handleGetProviderConfig(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	config, err := router.providerConfigs.Get(request.Context(), chi.URLParam(request, "providerID"))
	if err != nil {
		router.writeProviderConfigError(writer, requestID, startedAt, err)
		return
	}
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusOK, toProviderConfigResponseDto(config))
}

func (router *Router) handleUpdateProviderConfig(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	var dto ProviderConfigRequestDto
	if err := decodeRequestBody(request, &dto); err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}
	if err := router.validate.Struct(dto); err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}

	config, err := router.providerConfigs.Update(request.Context(), chi.URLParam(request, "providerID"), toProviderConfigRequest(dto))
	if err != nil {
		router.writeProviderConfigError(writer, requestID, startedAt, err)
		return
	}
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusOK, toProviderConfigResponseDto(config))
}

func (router *Router) handleDeleteProviderConfig(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	if err := router.providerConfigs.Delete(request.Context(), chi.URLParam(request, "providerID")); err != nil {
		router.writeProviderConfigError(writer, requestID, startedAt, err)
		return
	}
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusNoContent, struct{}{})
}

func (router *Router) handleActivateProviderConfig(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	config, err := router.providerConfigs.Activate(request.Context(), chi.URLParam(request, "providerID"))
	if err != nil {
		router.writeProviderConfigError(writer, requestID, startedAt, err)
		return
	}
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusOK, toProviderConfigResponseDto(config))
}

func (router *Router) handleDeactivateProviderConfig(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	config, err := router.providerConfigs.Deactivate(request.Context(), chi.URLParam(request, "providerID"))
	if err != nil {
		router.writeProviderConfigError(writer, requestID, startedAt, err)
		return
	}
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusOK, toProviderConfigResponseDto(config))
}

func (router *Router) handleTestProviderConfig(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	result, err := router.providerConfigs.Test(request.Context(), chi.URLParam(request, "providerID"))
	if err != nil {
		router.writeProviderConfigError(writer, requestID, startedAt, err)
		return
	}
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusOK, TestConnectionResponseDto{
		Reached:   result.Reached,
		LatencyMs: result.LatencyMs,
		Error:     result.Error,
	})
}

func (router *Router) handleCreateLLMConfig(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	var dto LLMConfigRequestDto
	if err := decodeRequestBody(request, &dto); err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}
	if err := router.validate.Struct(dto); err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}

	config, err := router.llmConfigs.Create(request.Context(), toLLMConfigRequest(dto))
	if err != nil {
		router.writeProviderConfigError(writer, requestID, startedAt, err)
		return
	}
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusCreated, toLLMConfigResponseDto(config))
}

func (router *Router) handleListLLMConfigs(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	limit, offset := paginationFromQuery(request)
	configs, err := router.llmConfigs.List(request.Context(), limit, offset)
	if err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}
	items := make([]LLMConfigResponseDto, 0, len(configs))
	for _, config := range configs {
		items = append(items, toLLMConfigResponseDto(config))
	}
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusOK, items)
}

func (router *Router) handleGetLLMConfig(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	config, err := router.llmConfigs.Get(request.Context(), chi.URLParam(request, "llmID"))
	if err != nil {
		router.writeProviderConfigError(writer, requestID, startedAt, err)
		return
	}
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusOK, toLLMConfigResponseDto(config))
}

func (router *Router) handleUpdateLLMConfig(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	var dto LLMConfigRequestDto
	if err := decodeRequestBody(request, &dto); err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}
	if err := router.validate.Struct(dto); err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}

	config, err := router.llmConfigs.Update(request.Context(), chi.URLParam(request, "llmID"), toLLMConfigRequest(dto))
	if err != nil {
		router.writeProviderConfigError(writer, requestID, startedAt, err)
		return
	}
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusOK, toLLMConfigResponseDto(config))
}

func (router *Router) handleDeleteLLMConfig(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	if err := router.llmConfigs.Delete(request.Context(), chi.URLParam(request, "llmID")); err != nil {
		router.writeProviderConfigError(writer, requestID, startedAt, err)
		return
	}
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusNoContent, struct{}{})
}

func (router *Router) handleActivateLLMConfig(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	config, err := router.llmConfigs.Activate(request.Context(), chi.URLParam(request, "llmID"))
	if err != nil {
		router.writeProviderConfigError(writer, requestID, startedAt, err)
		return
	}
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusOK, toLLMConfigResponseDto(config))
}

func (router *Router) handleTestLLMConfig(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	result, err := router.llmConfigs.Test(request.Context(), chi.URLParam(request, "llmID"))
	if err != nil {
		router.writeProviderConfigError(writer, requestID, startedAt, err)
		return
	}
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusOK, TestConnectionResponseDto{
		Reached:   result.Reached,
		LatencyMs: result.LatencyMs,
		Error:     result.Error,
	})
}

func (router *Router) writeProviderConfigError(writer stdhttp.ResponseWriter, requestID string, startedAt time.Time, err error) {
	switch {
	case errors.Is(err, domain.ErrProviderConfigNotFound):
		router.writeError(writer, requestID, startedAt, stdhttp.StatusNotFound, "provider_config_not_found", err.Error())
	case errors.Is(err, domain.ErrLLMConfigNotFound):
		router.writeError(writer, requestID, startedAt, stdhttp.StatusNotFound, "llm_config_not_found", err.Error())
	case errors.Is(err, domain.ErrProviderConfigConflict):
		router.writeError(writer, requestID, startedAt, stdhttp.StatusConflict, "provider_config_conflict", err.Error())
	case errors.Is(err, domain.ErrInvalidProviderConfig):
		router.writeError(writer, requestID, startedAt, stdhttp.StatusBadRequest, "invalid_provider_config", reasonOrFallback(err, domain.ErrInvalidProviderConfig, "provider configuration is invalid"))
	case errors.Is(err, domain.ErrInvalidLLMConfig):
		router.writeError(writer, requestID, startedAt, stdhttp.StatusBadRequest, "invalid_llm_config", reasonOrFallback(err, domain.ErrInvalidLLMConfig, "llm configuration is invalid"))
	default:
		router.writeDomainError(writer, requestID, startedAt, err)
	}
}

func reasonOrFallback(err error, base error, fallback string) string {
	message := err.Error()
	prefix := base.Error() + ": "
	if strings.HasPrefix(message, prefix) {
		reason := strings.TrimPrefix(message, prefix)
		if reason != "" {
			return reason
		}
	}
	return fallback
}

var _ = fmt.Sprintf
