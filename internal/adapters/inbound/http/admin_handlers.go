package http

import (
	"errors"
	"fmt"
	stdhttp "net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/core/policy"
	"github.com/guferreira1/observai-api/internal/core/usecase"
)

// IssueAPIKeyRequestDto carries the payload accepted by POST /v1/admin/keys.
type IssueAPIKeyRequestDto struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Scopes      []string `json:"scopes"`
	ExpiresAt   *string  `json:"expiresAt,omitempty"`
}

// IssuedAPIKeyResponseDto is the response payload for POST /v1/admin/keys.
//
// Secret is shown once at issue time and must not be retrievable later.
type IssuedAPIKeyResponseDto struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Scopes      []string `json:"scopes"`
	Secret      string   `json:"secret"`
	CreatedAt   string   `json:"createdAt"`
	ExpiresAt   *string  `json:"expiresAt,omitempty"`
}

// APIKeyDto describes a persisted API key without the secret.
type APIKeyDto struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Scopes      []string `json:"scopes"`
	Masked      string   `json:"masked"`
	CreatedAt   string   `json:"createdAt"`
	ExpiresAt   *string  `json:"expiresAt,omitempty"`
	LastUsedAt  *string  `json:"lastUsedAt,omitempty"`
	RevokedAt   *string  `json:"revokedAt,omitempty"`
}

// CreateWebhookRequestDto carries POST /v1/admin/webhooks input.
type CreateWebhookRequestDto struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Event  string `json:"event"`
	Secret string `json:"secret"`
}

// WebhookResponseDto describes a persisted webhook subscription.
type WebhookResponseDto struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	URL        string  `json:"url"`
	Event      string  `json:"event"`
	Secret     string  `json:"secret,omitempty"`
	CreatedAt  string  `json:"createdAt"`
	DisabledAt *string `json:"disabledAt,omitempty"`
}

// AuditEntryDto describes a single audit_log row.
type AuditEntryDto struct {
	ID         int64  `json:"id"`
	RequestID  string `json:"requestId"`
	APIKeyID   string `json:"apiKeyId"`
	Actor      string `json:"actor"`
	Method     string `json:"method"`
	Path       string `json:"path"`
	Status     int    `json:"status"`
	DurationMs int64  `json:"durationMs"`
	Remote     string `json:"remote"`
	CreatedAt  string `json:"createdAt"`
}

// AuditListResponseDto is the response payload for GET /v1/admin/audit.
type AuditListResponseDto struct {
	Items []AuditEntryDto `json:"items"`
}

func (router *Router) handleIssueAPIKey(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	var dto IssueAPIKeyRequestDto
	if err := decodeRequestBody(request, &dto); err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}

	issueRequest, err := toIssueAPIKeyRequest(dto)
	if err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}

	issued, err := router.apiKeys.Issue(request.Context(), issueRequest)
	if err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}

	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusCreated, IssuedAPIKeyResponseDto{
		ID:          issued.APIKey.ID,
		Name:        issued.APIKey.Name,
		Description: issued.APIKey.Description,
		Scopes:      scopesToStrings(issued.APIKey.Scopes),
		Secret:      issued.Secret,
		CreatedAt:   issued.APIKey.CreatedAt.Format(time.RFC3339),
		ExpiresAt:   formatOptionalTime(issued.APIKey.ExpiresAt),
	})
}

func (router *Router) handleListAPIKeys(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	limit, offset := paginationFromQuery(request)
	keys, err := router.apiKeys.List(request.Context(), limit, offset)
	if err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}

	items := make([]APIKeyDto, 0, len(keys))
	for _, key := range keys {
		items = append(items, toAPIKeyDto(key))
	}
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusOK, items)
}

func (router *Router) handleRevokeAPIKey(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	keyID := chi.URLParam(request, "keyID")
	if err := router.apiKeys.Revoke(request.Context(), keyID); err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusNoContent, struct{}{})
}

func (router *Router) handleCreateWebhook(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	var dto CreateWebhookRequestDto
	if err := decodeRequestBody(request, &dto); err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}

	webhook, err := router.webhooks.Create(request.Context(), dto.Name, dto.URL, dto.Event, dto.Secret)
	if err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}

	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusCreated, WebhookResponseDto{
		ID:        webhook.ID,
		Name:      webhook.Name,
		URL:       webhook.URL,
		Event:     webhook.Event,
		Secret:    webhook.Secret,
		CreatedAt: webhook.CreatedAt.Format(time.RFC3339),
	})
}

func (router *Router) handleListWebhooks(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	limit, offset := paginationFromQuery(request)
	webhooks, err := router.webhooks.List(request.Context(), limit, offset)
	if err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}

	items := make([]WebhookResponseDto, 0, len(webhooks))
	for _, webhook := range webhooks {
		items = append(items, WebhookResponseDto{
			ID:         webhook.ID,
			Name:       webhook.Name,
			URL:        webhook.URL,
			Event:      webhook.Event,
			CreatedAt:  webhook.CreatedAt.Format(time.RFC3339),
			DisabledAt: formatOptionalTime(webhook.DisabledAt),
		})
	}
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusOK, items)
}

func (router *Router) handleDeleteWebhook(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	if err := router.webhooks.Disable(request.Context(), chi.URLParam(request, "webhookID")); err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusNoContent, struct{}{})
}

func (router *Router) handleListAudit(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	limit, offset := paginationFromQuery(request)
	filter := domain.AuditFilter{
		APIKeyID: request.URL.Query().Get("apiKeyId"),
		Limit:    limit,
		Offset:   offset,
	}
	if value := strings.TrimSpace(request.URL.Query().Get("from")); value != "" {
		if parsed, err := time.Parse(time.RFC3339, value); err == nil {
			filter.From = parsed
		}
	}
	if value := strings.TrimSpace(request.URL.Query().Get("to")); value != "" {
		if parsed, err := time.Parse(time.RFC3339, value); err == nil {
			filter.To = parsed
		}
	}

	entries, err := router.auditLog.List(request.Context(), filter)
	if err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}

	items := make([]AuditEntryDto, 0, len(entries))
	for _, entry := range entries {
		items = append(items, AuditEntryDto{
			ID:         entry.ID,
			RequestID:  entry.RequestID,
			APIKeyID:   entry.APIKeyID,
			Actor:      entry.Actor,
			Method:     entry.Method,
			Path:       entry.Path,
			Status:     entry.Status,
			DurationMs: entry.DurationMs,
			Remote:     entry.Remote,
			CreatedAt:  entry.CreatedAt.Format(time.RFC3339),
		})
	}
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusOK, AuditListResponseDto{Items: items})
}

func (router *Router) handleDeleteAnalysis(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	if err := router.retention.Delete(request.Context(), chi.URLParam(request, "analysisID")); err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusNoContent, struct{}{})
}

// PurgeAnalysesResponseDto describes the result of a bulk retention purge.
type PurgeAnalysesResponseDto struct {
	Deleted int `json:"deleted"`
}

func (router *Router) handlePurgeAnalyses(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	raw := strings.TrimSpace(request.URL.Query().Get("olderThan"))
	if raw == "" {
		router.writeDomainError(writer, requestID, startedAt, errors.New("olderThan query parameter is required (e.g. 720h)"))
		return
	}
	age, err := time.ParseDuration(raw)
	if err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}

	deleted, err := router.retention.Purge(request.Context(), age)
	if err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusOK, PurgeAnalysesResponseDto{Deleted: deleted})
}

func toIssueAPIKeyRequest(dto IssueAPIKeyRequestDto) (usecase.IssueAPIKeyRequest, error) {
	scopes := make([]domain.APIKeyScope, 0, len(dto.Scopes))
	for _, raw := range dto.Scopes {
		scopes = append(scopes, domain.APIKeyScope(raw))
	}
	request := usecase.IssueAPIKeyRequest{
		Name:        strings.TrimSpace(dto.Name),
		Description: strings.TrimSpace(dto.Description),
		Scopes:      scopes,
	}
	if dto.ExpiresAt != nil {
		cleaned := strings.TrimSpace(*dto.ExpiresAt)
		if cleaned != "" {
			expiresAt, err := time.Parse(time.RFC3339, cleaned)
			if err != nil {
				return usecase.IssueAPIKeyRequest{}, fmt.Errorf("%w: expiresAt must be RFC3339", domain.ErrInvalidAPIKey)
			}
			expiresAt = expiresAt.UTC()
			request.ExpiresAt = &expiresAt
		}
	}
	return request, nil
}

func toAPIKeyDto(key domain.APIKey) APIKeyDto {
	dto := APIKeyDto{
		ID:          key.ID,
		Name:        key.Name,
		Description: key.Description,
		Scopes:      scopesToStrings(key.Scopes),
		Masked:      policy.MaskSecret("oai_" + key.ID),
		CreatedAt:   key.CreatedAt.Format(time.RFC3339),
		ExpiresAt:   formatOptionalTime(key.ExpiresAt),
		LastUsedAt:  formatOptionalTime(key.LastUsedAt),
		RevokedAt:   formatOptionalTime(key.RevokedAt),
	}
	return dto
}

func scopesToStrings(scopes []domain.APIKeyScope) []string {
	out := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		out = append(out, string(scope))
	}
	return out
}

func paginationFromQuery(request *stdhttp.Request) (int, int) {
	limit := 50
	offset := 0
	if raw := strings.TrimSpace(request.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if raw := strings.TrimSpace(request.URL.Query().Get("offset")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed >= 0 {
			offset = parsed
		}
	}
	return limit, offset
}

func formatOptionalTime(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := value.Format(time.RFC3339)
	return &formatted
}
