package usecase

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/core/ports"
)

// WebhookSubscriptions is the use case for managing webhook subscriptions,
// dispatching events and tracking delivery history.
type WebhookSubscriptions struct {
	repository ports.WebhookRepository
	deliveries ports.WebhookDeliveryRepository
	dispatcher ports.WebhookDispatcher
	ids        ports.IDGenerator
	now        func() time.Time
}

// NewWebhookSubscriptions creates a use case for webhook administration.
//
// When deliveries is non-nil the use case persists each dispatch as a
// delivery row before invoking the dispatcher, so operators can inspect
// history, retry failed deliveries and the scheduler can sweep pending
// entries.
func NewWebhookSubscriptions(repository ports.WebhookRepository, deliveries ports.WebhookDeliveryRepository, dispatcher ports.WebhookDispatcher, ids ports.IDGenerator) *WebhookSubscriptions {
	return &WebhookSubscriptions{repository: repository, deliveries: deliveries, dispatcher: dispatcher, ids: ids, now: time.Now}
}

// Create persists a new webhook subscription. A random HMAC secret is
// generated when the caller did not provide one; secrets are returned to
// the operator so they can configure verification on the receiver side.
func (useCase *WebhookSubscriptions) Create(ctx context.Context, name string, target string, event string, secret string) (domain.Webhook, error) {
	cleanedURL, cleanedEvent, err := validateWebhookTarget(target, event)
	if err != nil {
		return domain.Webhook{}, err
	}

	id, err := useCase.ids.NextID(ctx)
	if err != nil {
		return domain.Webhook{}, fmt.Errorf("generate webhook id: %w", err)
	}

	usedSecret := strings.TrimSpace(secret)
	if usedSecret == "" {
		usedSecret, err = generateWebhookSecret()
		if err != nil {
			return domain.Webhook{}, fmt.Errorf("generate webhook secret: %w", err)
		}
	}

	webhook := domain.Webhook{
		ID:        id,
		Name:      strings.TrimSpace(name),
		URL:       cleanedURL,
		Secret:    usedSecret,
		Event:     cleanedEvent,
		CreatedAt: useCase.now().UTC(),
	}
	if err := useCase.repository.Create(ctx, webhook); err != nil {
		return domain.Webhook{}, fmt.Errorf("persist webhook: %w", err)
	}
	return webhook, nil
}

// Update replaces mutable subscription fields while preserving the secret
// when the caller omits a replacement.
func (useCase *WebhookSubscriptions) Update(ctx context.Context, id string, name string, target string, event string, secret string) (domain.Webhook, error) {
	cleanedID := strings.TrimSpace(id)
	if cleanedID == "" {
		return domain.Webhook{}, domain.ErrWebhookNotFound
	}
	cleanedURL, cleanedEvent, err := validateWebhookTarget(target, event)
	if err != nil {
		return domain.Webhook{}, err
	}
	current, err := useCase.repository.Find(ctx, cleanedID)
	if err != nil {
		return domain.Webhook{}, err
	}
	usedSecret := strings.TrimSpace(secret)
	if usedSecret == "" {
		usedSecret = current.Secret
	}
	current.Name = strings.TrimSpace(name)
	current.URL = cleanedURL
	current.Event = cleanedEvent
	current.Secret = usedSecret
	if err := useCase.repository.Update(ctx, current); err != nil {
		return domain.Webhook{}, err
	}
	return useCase.repository.Find(ctx, cleanedID)
}

// Find returns the webhook subscription matching the supplied identifier.
func (useCase *WebhookSubscriptions) Find(ctx context.Context, id string) (domain.Webhook, error) {
	cleaned := strings.TrimSpace(id)
	if cleaned == "" {
		return domain.Webhook{}, domain.ErrWebhookNotFound
	}
	return useCase.repository.Find(ctx, cleaned)
}

// List returns persisted webhooks newest first.
func (useCase *WebhookSubscriptions) List(ctx context.Context, limit int, offset int) ([]domain.Webhook, error) {
	if limit <= 0 {
		limit = 50
	}
	return useCase.repository.List(ctx, limit, offset)
}

// Disable marks the supplied webhook as disabled.
func (useCase *WebhookSubscriptions) Disable(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("%w: id is required", domain.ErrInvalidWebhook)
	}
	return useCase.repository.Disable(ctx, id)
}

// DispatchEvent fan-outs the payload to every active subscriber of the
// supplied event. Each delivery is persisted (when a delivery repository
// is configured) and then enqueued asynchronously through the configured
// WebhookDispatcher; the dispatcher reports back to the delivery row via
// the repository when the attempt completes.
func (useCase *WebhookSubscriptions) DispatchEvent(ctx context.Context, event string, payload []byte) error {
	if useCase.dispatcher == nil {
		return nil
	}
	webhooks, err := useCase.repository.ListActive(ctx, event)
	if err != nil {
		return fmt.Errorf("list webhooks for %s: %w", event, err)
	}
	for _, webhook := range webhooks {
		if err := useCase.dispatchOnce(ctx, webhook, event, payload); err != nil {
			return err
		}
	}
	return nil
}

// ListDeliveries returns persisted delivery attempts newest first.
func (useCase *WebhookSubscriptions) ListDeliveries(ctx context.Context, limit int, offset int) ([]domain.WebhookDelivery, error) {
	if useCase.deliveries == nil {
		return []domain.WebhookDelivery{}, nil
	}
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return useCase.deliveries.List(ctx, limit, offset)
}

// Retry re-attempts the supplied delivery. The previous payload is reused
// so the receiver sees the exact same message bytes.
func (useCase *WebhookSubscriptions) Retry(ctx context.Context, deliveryID string) (domain.WebhookDelivery, error) {
	if useCase.deliveries == nil || useCase.dispatcher == nil {
		return domain.WebhookDelivery{}, fmt.Errorf("retry not supported without delivery storage")
	}
	cleaned := strings.TrimSpace(deliveryID)
	if cleaned == "" {
		return domain.WebhookDelivery{}, domain.ErrWebhookDeliveryNotFound
	}
	delivery, err := useCase.deliveries.Find(ctx, cleaned)
	if err != nil {
		return domain.WebhookDelivery{}, err
	}
	webhook, err := useCase.repository.Find(ctx, delivery.SubscriptionID)
	if err != nil {
		return domain.WebhookDelivery{}, err
	}
	now := useCase.now().UTC()
	delivery.Status = domain.WebhookDeliveryPending
	delivery.LastError = ""
	delivery.NextAttemptAt = nil
	delivery.UpdatedAt = now
	if err := useCase.deliveries.ScheduleRetry(ctx, delivery.ID, delivery.Attempt, 0, "", now, now); err != nil {
		return domain.WebhookDelivery{}, err
	}
	if err := useCase.dispatcher.Dispatch(ctx, webhook, delivery); err != nil {
		return domain.WebhookDelivery{}, fmt.Errorf("dispatch retry: %w", err)
	}
	return delivery, nil
}

// Test sends a synthetic webhook.test event to the supplied subscription
// so operators can verify the receiver without waiting for a real event.
func (useCase *WebhookSubscriptions) Test(ctx context.Context, subscriptionID string) (domain.WebhookDelivery, error) {
	cleaned := strings.TrimSpace(subscriptionID)
	if cleaned == "" {
		return domain.WebhookDelivery{}, domain.ErrWebhookNotFound
	}
	webhook, err := useCase.repository.Find(ctx, cleaned)
	if err != nil {
		return domain.WebhookDelivery{}, err
	}
	payload := []byte(fmt.Sprintf(`{"event":%q,"subscriptionId":%q,"at":%q}`,
		domain.WebhookEventTest,
		webhook.ID,
		useCase.now().UTC().Format(time.RFC3339),
	))
	delivery, err := useCase.persistDelivery(ctx, webhook.ID, domain.WebhookEventTest, payload)
	if err != nil {
		return domain.WebhookDelivery{}, err
	}
	if useCase.dispatcher != nil {
		if err := useCase.dispatcher.Dispatch(ctx, webhook, delivery); err != nil {
			return domain.WebhookDelivery{}, fmt.Errorf("dispatch test webhook: %w", err)
		}
	}
	return delivery, nil
}

// DispatchPending re-enqueues every pending delivery due on or before
// cutoff. Returns the number of deliveries kicked off.
func (useCase *WebhookSubscriptions) DispatchPending(ctx context.Context, cutoff time.Time, limit int) (int, error) {
	if useCase.deliveries == nil || useCase.dispatcher == nil {
		return 0, nil
	}
	if limit <= 0 {
		limit = 50
	}
	pending, err := useCase.deliveries.ListPending(ctx, cutoff, limit)
	if err != nil {
		return 0, err
	}
	dispatched := 0
	for _, delivery := range pending {
		webhook, err := useCase.repository.Find(ctx, delivery.SubscriptionID)
		if err != nil {
			if errors.Is(err, domain.ErrWebhookNotFound) {
				continue
			}
			return dispatched, err
		}
		if err := useCase.dispatcher.Dispatch(ctx, webhook, delivery); err != nil {
			return dispatched, fmt.Errorf("dispatch pending: %w", err)
		}
		dispatched++
	}
	return dispatched, nil
}

func (useCase *WebhookSubscriptions) dispatchOnce(ctx context.Context, webhook domain.Webhook, event string, payload []byte) error {
	delivery, err := useCase.persistDelivery(ctx, webhook.ID, event, payload)
	if err != nil {
		return err
	}
	return useCase.dispatcher.Dispatch(ctx, webhook, delivery)
}

func (useCase *WebhookSubscriptions) persistDelivery(ctx context.Context, subscriptionID, event string, payload []byte) (domain.WebhookDelivery, error) {
	now := useCase.now().UTC()
	delivery := domain.WebhookDelivery{
		SubscriptionID: subscriptionID,
		Event:          event,
		Payload:        payload,
		Status:         domain.WebhookDeliveryPending,
		Attempt:        0,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if useCase.deliveries == nil {
		return delivery, nil
	}
	id, err := useCase.ids.NextID(ctx)
	if err != nil {
		return domain.WebhookDelivery{}, fmt.Errorf("generate delivery id: %w", err)
	}
	delivery.ID = id
	if err := useCase.deliveries.Create(ctx, delivery); err != nil {
		return domain.WebhookDelivery{}, fmt.Errorf("persist webhook delivery: %w", err)
	}
	return delivery, nil
}

func generateWebhookSecret() (string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}

func validateWebhookTarget(target string, event string) (string, string, error) {
	cleanedURL := strings.TrimSpace(target)
	parsed, err := url.Parse(cleanedURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", "", fmt.Errorf("%w: url must be a valid http(s) endpoint", domain.ErrInvalidWebhook)
	}
	cleanedEvent := strings.TrimSpace(event)
	if cleanedEvent == "" {
		return "", "", fmt.Errorf("%w: event is required", domain.ErrInvalidWebhook)
	}
	return cleanedURL, cleanedEvent, nil
}
