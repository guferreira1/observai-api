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
