package usecase

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/core/ports"
)

// WebhookSubscriptions is the use case for managing webhook subscriptions
// and dispatching events.
type WebhookSubscriptions struct {
	repository ports.WebhookRepository
	dispatcher ports.WebhookDispatcher
	ids        ports.IDGenerator
	now        func() time.Time
}

// NewWebhookSubscriptions creates a use case for webhook administration.
func NewWebhookSubscriptions(repository ports.WebhookRepository, dispatcher ports.WebhookDispatcher, ids ports.IDGenerator) *WebhookSubscriptions {
	return &WebhookSubscriptions{repository: repository, dispatcher: dispatcher, ids: ids, now: time.Now}
}

// Create persists a new webhook subscription. A random HMAC secret is
// generated when the caller did not provide one; secrets are returned to
// the operator so they can configure verification on the receiver side.
func (useCase *WebhookSubscriptions) Create(ctx context.Context, name string, target string, event string, secret string) (domain.Webhook, error) {
	cleanedURL := strings.TrimSpace(target)
	parsed, err := url.Parse(cleanedURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return domain.Webhook{}, fmt.Errorf("%w: url must be a valid http(s) endpoint", domain.ErrInvalidWebhook)
	}
	cleanedEvent := strings.TrimSpace(event)
	if cleanedEvent == "" {
		return domain.Webhook{}, fmt.Errorf("%w: event is required", domain.ErrInvalidWebhook)
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
// supplied event. Each delivery is enqueued asynchronously through the
// configured WebhookDispatcher.
func (useCase *WebhookSubscriptions) DispatchEvent(ctx context.Context, event string, payload []byte) error {
	if useCase.dispatcher == nil {
		return nil
	}
	webhooks, err := useCase.repository.ListActive(ctx, event)
	if err != nil {
		return fmt.Errorf("list webhooks for %s: %w", event, err)
	}
	for _, webhook := range webhooks {
		if err := useCase.dispatcher.Dispatch(ctx, webhook, event, payload); err != nil {
			return fmt.Errorf("dispatch webhook %s: %w", webhook.ID, err)
		}
	}
	return nil
}

func generateWebhookSecret() (string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}
