package domain

import (
	"errors"
	"time"
)

// ErrInvalidWebhook is returned when a webhook payload fails validation.
var ErrInvalidWebhook = errors.New("invalid webhook")

// ErrWebhookNotFound is returned when a referenced webhook does not exist.
var ErrWebhookNotFound = errors.New("webhook not found")

// WebhookEventAnalysisCompleted is dispatched when an analysis job succeeds.
const WebhookEventAnalysisCompleted = "analysis.completed"

// WebhookEventAnalysisFailed is dispatched when an analysis job fails.
const WebhookEventAnalysisFailed = "analysis.failed"

// WebhookEventAnalysisCanceled is dispatched when an analysis job is canceled
// by a client.
const WebhookEventAnalysisCanceled = "analysis.canceled"

// WebhookEventTest is dispatched by the admin "test webhook" endpoint so
// operators can verify a subscription without waiting for a real event.
const WebhookEventTest = "webhook.test"

// WebhookDeliveryStatus identifies the lifecycle state of a delivery
// attempt.
type WebhookDeliveryStatus string

const (
	// WebhookDeliveryPending indicates the delivery is enqueued for the next
	// scheduled attempt.
	WebhookDeliveryPending WebhookDeliveryStatus = "pending"
	// WebhookDeliveryDelivered indicates the receiver responded with a 2xx.
	WebhookDeliveryDelivered WebhookDeliveryStatus = "delivered"
	// WebhookDeliveryFailed indicates the delivery exhausted its retries.
	WebhookDeliveryFailed WebhookDeliveryStatus = "failed"
)

// ErrWebhookDeliveryNotFound is returned when a webhook delivery id cannot
// be resolved.
var ErrWebhookDeliveryNotFound = errors.New("webhook delivery not found")

// WebhookDelivery is the persisted record of an attempted webhook delivery.
//
// Payload carries the encoded JSON body that was (or will be) sent so a
// manual retry reproduces the original message bit-for-bit. The retry
// scheduler consults Status + NextAttemptAt to decide which deliveries
// are due.
type WebhookDelivery struct {
	ID             string
	SubscriptionID string
	Event          string
	Payload        []byte
	Status         WebhookDeliveryStatus
	Attempt        int
	LastError      string
	ResponseStatus int
	NextAttemptAt  *time.Time
	DeliveredAt    *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Webhook describes an external endpoint that receives event notifications.
//
// Secret is mandatory; outbound calls sign the payload with HMAC-SHA256
// using this secret so receivers can verify authenticity.
type Webhook struct {
	ID         string
	Name       string
	URL        string
	Secret     string
	Event      string
	CreatedAt  time.Time
	DisabledAt *time.Time
}
