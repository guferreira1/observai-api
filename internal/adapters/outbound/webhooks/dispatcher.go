// Package webhooks implements an HTTP webhook dispatcher with HMAC-SHA256
// payload signing and persistent retry tracking.
//
// Delivery happens on a background goroutine so callers (analysis worker)
// are not blocked by slow receivers. After each attempt the dispatcher
// updates the corresponding delivery row through the configured
// WebhookDeliveryRepository so operators can inspect the outcome and the
// retry scheduler can pick up pending entries on the next sweep.
package webhooks

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/core/ports"
	"github.com/guferreira1/observai-api/internal/platform/observability"
)

// DispatcherOptions configures the HTTP dispatcher.
type DispatcherOptions struct {
	Timeout        time.Duration
	Logger         *slog.Logger
	Observer       observability.ProviderObserver
	Deliveries     ports.WebhookDeliveryRepository
	MaxAttempts    int
	RetryBaseDelay time.Duration
	RetryMaxDelay  time.Duration
	Clock          func() time.Time
}

// Dispatcher implements ports.WebhookDispatcher over net/http.
type Dispatcher struct {
	client         *http.Client
	logger         *slog.Logger
	observer       observability.ProviderObserver
	deliveries     ports.WebhookDeliveryRepository
	maxAttempts    int
	retryBaseDelay time.Duration
	retryMaxDelay  time.Duration
	clock          func() time.Time
}

// NewDispatcher builds an HTTP webhook dispatcher with the supplied
// options. Sensible defaults are applied when fields are zero.
func NewDispatcher(opts DispatcherOptions) *Dispatcher {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	observer := opts.Observer
	if observer == nil {
		observer = observability.NoopProviderObserver{}
	}
	maxAttempts := opts.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	baseDelay := opts.RetryBaseDelay
	if baseDelay <= 0 {
		baseDelay = 30 * time.Second
	}
	maxDelay := opts.RetryMaxDelay
	if maxDelay <= 0 {
		maxDelay = 30 * time.Minute
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	return &Dispatcher{
		client:         &http.Client{Timeout: timeout},
		logger:         logger,
		observer:       observer,
		deliveries:     opts.Deliveries,
		maxAttempts:    maxAttempts,
		retryBaseDelay: baseDelay,
		retryMaxDelay:  maxDelay,
		clock:          clock,
	}
}

// Dispatch enqueues a single webhook delivery. The call returns
// immediately; delivery happens on a goroutine so the producer is not
// blocked by the receiver.
func (dispatcher *Dispatcher) Dispatch(ctx context.Context, webhook domain.Webhook, delivery domain.WebhookDelivery) error {
	go dispatcher.deliver(context.WithoutCancel(ctx), webhook, delivery)
	return nil
}

func (dispatcher *Dispatcher) deliver(ctx context.Context, webhook domain.Webhook, delivery domain.WebhookDelivery) {
	signature := signPayload(webhook.Secret, delivery.Payload)
	startedAt := time.Now()
	responseStatus, err := dispatcher.post(ctx, webhook, delivery, signature)
	dispatcher.observer.Observe("webhook", "deliver", time.Since(startedAt), err)

	if err == nil {
		dispatcher.markDelivered(ctx, delivery.ID, responseStatus)
		return
	}

	attempt := delivery.Attempt + 1
	var transient *transientError
	if attempt >= dispatcher.maxAttempts || !errors.As(err, &transient) {
		dispatcher.markFailed(ctx, delivery.ID, attempt, responseStatus, err)
		return
	}
	nextAttempt := dispatcher.clock().UTC().Add(dispatcher.backoffFor(attempt))
	dispatcher.scheduleRetry(ctx, delivery.ID, attempt, responseStatus, err, nextAttempt)
}

func (dispatcher *Dispatcher) markDelivered(ctx context.Context, id string, status int) {
	if dispatcher.deliveries == nil || id == "" {
		return
	}
	if err := dispatcher.deliveries.MarkDelivered(ctx, id, status, dispatcher.clock().UTC()); err != nil {
		dispatcher.logger.Warn("mark webhook delivered failed", "delivery_id", id, "error", err)
	}
}

func (dispatcher *Dispatcher) markFailed(ctx context.Context, id string, attempt, responseStatus int, deliveryErr error) {
	dispatcher.logger.Warn("webhook delivery failed", "delivery_id", id, "attempt", attempt, "error", deliveryErr)
	if dispatcher.deliveries == nil || id == "" {
		return
	}
	if err := dispatcher.deliveries.MarkFailed(ctx, id, attempt, responseStatus, deliveryErr.Error(), dispatcher.clock().UTC()); err != nil {
		dispatcher.logger.Warn("mark webhook failed failed", "delivery_id", id, "error", err)
	}
}

func (dispatcher *Dispatcher) scheduleRetry(ctx context.Context, id string, attempt, responseStatus int, deliveryErr error, nextAttemptAt time.Time) {
	dispatcher.logger.Info("webhook delivery scheduled for retry", "delivery_id", id, "attempt", attempt, "next_attempt_at", nextAttemptAt)
	if dispatcher.deliveries == nil || id == "" {
		return
	}
	if err := dispatcher.deliveries.ScheduleRetry(ctx, id, attempt, responseStatus, deliveryErr.Error(), nextAttemptAt, dispatcher.clock().UTC()); err != nil {
		dispatcher.logger.Warn("schedule webhook retry failed", "delivery_id", id, "error", err)
	}
}

func (dispatcher *Dispatcher) backoffFor(attempt int) time.Duration {
	if attempt <= 0 {
		return dispatcher.retryBaseDelay
	}
	exp := math.Pow(2, float64(attempt-1))
	delay := time.Duration(float64(dispatcher.retryBaseDelay) * exp)
	if delay > dispatcher.retryMaxDelay {
		delay = dispatcher.retryMaxDelay
	}
	return delay
}

func (dispatcher *Dispatcher) post(ctx context.Context, webhook domain.Webhook, delivery domain.WebhookDelivery, signature string) (int, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, webhook.URL, bytes.NewReader(delivery.Payload))
	if err != nil {
		return 0, fmt.Errorf("build webhook request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-ObservAI-Event", delivery.Event)
	request.Header.Set("X-ObservAI-Webhook-ID", webhook.ID)
	request.Header.Set("X-ObservAI-Delivery-ID", delivery.ID)
	request.Header.Set("X-ObservAI-Signature", "sha256="+signature)

	response, err := dispatcher.client.Do(request)
	if err != nil {
		return 0, &transientError{err: fmt.Errorf("call webhook: %w", err)}
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	if response.StatusCode >= 500 {
		return response.StatusCode, &transientError{err: fmt.Errorf("webhook returned status %d", response.StatusCode)}
	}
	if response.StatusCode >= 400 {
		return response.StatusCode, fmt.Errorf("webhook returned status %d", response.StatusCode)
	}
	return response.StatusCode, nil
}

func signPayload(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

type transientError struct{ err error }

func (e *transientError) Error() string { return e.err.Error() }
func (e *transientError) Unwrap() error { return e.err }

// Compile-time assertion that the Dispatcher satisfies the core port.
var _ ports.WebhookDispatcher = (*Dispatcher)(nil)
