package http

import (
	"encoding/json"
	"errors"
	stdhttp "net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/guferreira1/observai-api/internal/core/domain"
)

// WebhookDeliveryDto is the public projection of domain.WebhookDelivery.
type WebhookDeliveryDto struct {
	ID             string          `json:"id"`
	SubscriptionID string          `json:"subscriptionId"`
	Event          string          `json:"event"`
	Status         string          `json:"status"`
	Attempt        int             `json:"attempt"`
	LastError      string          `json:"lastError,omitempty"`
	ResponseStatus int             `json:"responseStatus,omitempty"`
	NextAttemptAt  *string         `json:"nextAttemptAt,omitempty"`
	DeliveredAt    *string         `json:"deliveredAt,omitempty"`
	CreatedAt      string          `json:"createdAt"`
	UpdatedAt      string          `json:"updatedAt"`
	Payload        json.RawMessage `json:"payload,omitempty"`
}

func toWebhookDeliveryDto(delivery domain.WebhookDelivery) WebhookDeliveryDto {
	dto := WebhookDeliveryDto{
		ID:             delivery.ID,
		SubscriptionID: delivery.SubscriptionID,
		Event:          delivery.Event,
		Status:         string(delivery.Status),
		Attempt:        delivery.Attempt,
		LastError:      delivery.LastError,
		ResponseStatus: delivery.ResponseStatus,
		CreatedAt:      delivery.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      delivery.UpdatedAt.Format(time.RFC3339),
	}
	if delivery.NextAttemptAt != nil {
		next := delivery.NextAttemptAt.Format(time.RFC3339)
		dto.NextAttemptAt = &next
	}
	if delivery.DeliveredAt != nil {
		delivered := delivery.DeliveredAt.Format(time.RFC3339)
		dto.DeliveredAt = &delivered
	}
	if json.Valid(delivery.Payload) {
		dto.Payload = json.RawMessage(delivery.Payload)
	}
	return dto
}

func (router *Router) handleTestWebhook(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	webhookID := chi.URLParam(request, "webhookID")
	delivery, err := router.webhooks.Test(request.Context(), webhookID)
	if err != nil {
		router.writeWebhookError(writer, requestID, startedAt, err)
		return
	}
	AnnotateAudit(request, AuditAnnotation{
		Action:       "webhook.tested",
		ResourceType: "webhook",
		ResourceID:   webhookID,
		Metadata:     map[string]string{"deliveryId": delivery.ID},
	})
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusAccepted, toWebhookDeliveryDto(delivery))
}

func (router *Router) handleListWebhookDeliveries(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	limit, offset := paginationFromQuery(request)
	deliveries, err := router.webhooks.ListDeliveries(request.Context(), limit, offset)
	if err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}
	items := make([]WebhookDeliveryDto, 0, len(deliveries))
	for _, delivery := range deliveries {
		items = append(items, toWebhookDeliveryDto(delivery))
	}
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusOK, items)
}

func (router *Router) handleRetryWebhookDelivery(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	deliveryID := chi.URLParam(request, "deliveryID")
	delivery, err := router.webhooks.Retry(request.Context(), deliveryID)
	if err != nil {
		router.writeWebhookError(writer, requestID, startedAt, err)
		return
	}
	AnnotateAudit(request, AuditAnnotation{
		Action:       "webhook.retried",
		ResourceType: "webhook_delivery",
		ResourceID:   deliveryID,
		Metadata:     map[string]string{"event": delivery.Event},
	})
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusAccepted, toWebhookDeliveryDto(delivery))
}

func (router *Router) writeWebhookError(writer stdhttp.ResponseWriter, requestID string, startedAt time.Time, err error) {
	switch {
	case errors.Is(err, domain.ErrWebhookNotFound):
		router.writeError(writer, requestID, startedAt, stdhttp.StatusNotFound, "webhook_not_found", err.Error())
	case errors.Is(err, domain.ErrWebhookDeliveryNotFound):
		router.writeError(writer, requestID, startedAt, stdhttp.StatusNotFound, "webhook_delivery_not_found", err.Error())
	default:
		router.writeDomainError(writer, requestID, startedAt, err)
	}
}
