// Package webhooks implements an HTTP webhook dispatcher with HMAC-SHA256
// payload signing and bounded exponential retries.
//
// Delivery happens on a background goroutine so callers (analysis worker)
// are not blocked by slow receivers. Failed deliveries are retried using
// the configured policy and logged through the standard slog logger.
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
	"net/http"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/core/ports"
	"github.com/guferreira1/observai-api/internal/platform/observability"
	"github.com/guferreira1/observai-api/internal/platform/retry"
)

// DispatcherOptions configures the HTTP dispatcher.
type DispatcherOptions struct {
	Timeout     time.Duration
	Logger      *slog.Logger
	Observer    observability.ProviderObserver
	RetryPolicy retry.Policy
}

// Dispatcher implements ports.WebhookDispatcher over net/http.
type Dispatcher struct {
	client      *http.Client
	logger      *slog.Logger
	observer    observability.ProviderObserver
	retryPolicy retry.Policy
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
	policy := opts.RetryPolicy
	if policy.MaxAttempts <= 0 {
		policy = retry.Default()
	}
	return &Dispatcher{
		client:      &http.Client{Timeout: timeout},
		logger:      logger,
		observer:    observer,
		retryPolicy: policy,
	}
}

// Dispatch enqueues a single webhook delivery. The call returns
// immediately; delivery happens on a goroutine so the producer is not
// blocked by the receiver.
func (dispatcher *Dispatcher) Dispatch(ctx context.Context, webhook domain.Webhook, event string, payload []byte) error {
	go dispatcher.deliver(context.WithoutCancel(ctx), webhook, event, payload)
	return nil
}

func (dispatcher *Dispatcher) deliver(ctx context.Context, webhook domain.Webhook, event string, payload []byte) {
	signature := signPayload(webhook.Secret, payload)
	startedAt := time.Now()
	err := retry.Do(ctx, dispatcher.retryPolicy, isRetryable, func(int) error {
		return dispatcher.post(ctx, webhook, event, payload, signature)
	})
	dispatcher.observer.Observe("webhook", "deliver", time.Since(startedAt), err)
	if err != nil {
		dispatcher.logger.Warn("webhook delivery failed",
			"webhook_id", webhook.ID,
			"event", event,
			"error", err,
		)
	}
}

func (dispatcher *Dispatcher) post(ctx context.Context, webhook domain.Webhook, event string, payload []byte, signature string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, webhook.URL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build webhook request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-ObservAI-Event", event)
	request.Header.Set("X-ObservAI-Webhook-ID", webhook.ID)
	request.Header.Set("X-ObservAI-Signature", "sha256="+signature)

	response, err := dispatcher.client.Do(request)
	if err != nil {
		return &transientError{err: fmt.Errorf("call webhook: %w", err)}
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	if response.StatusCode >= 500 {
		return &transientError{err: fmt.Errorf("webhook returned status %d", response.StatusCode)}
	}
	if response.StatusCode >= 400 {
		return fmt.Errorf("webhook returned status %d", response.StatusCode)
	}
	return nil
}

func signPayload(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

type transientError struct{ err error }

func (e *transientError) Error() string { return e.err.Error() }
func (e *transientError) Unwrap() error { return e.err }

func isRetryable(err error) bool {
	var transient *transientError
	return errors.As(err, &transient)
}

// Compile-time assertion that the Dispatcher satisfies the core port.
var _ ports.WebhookDispatcher = (*Dispatcher)(nil)
