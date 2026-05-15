package usecase

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

type stubWebhookRepo struct {
	mu       sync.Mutex
	webhooks map[string]domain.Webhook
}

func newStubWebhookRepo() *stubWebhookRepo {
	return &stubWebhookRepo{webhooks: make(map[string]domain.Webhook)}
}

func (stub *stubWebhookRepo) Create(_ context.Context, webhook domain.Webhook) error {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	stub.webhooks[webhook.ID] = webhook
	return nil
}

func (stub *stubWebhookRepo) Find(_ context.Context, id string) (domain.Webhook, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	webhook, ok := stub.webhooks[id]
	if !ok {
		return domain.Webhook{}, domain.ErrWebhookNotFound
	}
	return webhook, nil
}

func (stub *stubWebhookRepo) List(_ context.Context, _, _ int) ([]domain.Webhook, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	out := make([]domain.Webhook, 0, len(stub.webhooks))
	for _, webhook := range stub.webhooks {
		out = append(out, webhook)
	}
	return out, nil
}

func (stub *stubWebhookRepo) ListActive(_ context.Context, event string) ([]domain.Webhook, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	out := make([]domain.Webhook, 0)
	for _, webhook := range stub.webhooks {
		if webhook.Event == event && webhook.DisabledAt == nil {
			out = append(out, webhook)
		}
	}
	return out, nil
}

func (stub *stubWebhookRepo) Update(_ context.Context, webhook domain.Webhook) error {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if _, ok := stub.webhooks[webhook.ID]; !ok {
		return domain.ErrWebhookNotFound
	}
	stub.webhooks[webhook.ID] = webhook
	return nil
}

func (stub *stubWebhookRepo) Disable(_ context.Context, id string) error {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	webhook, ok := stub.webhooks[id]
	if !ok {
		return domain.ErrWebhookNotFound
	}
	now := time.Now().UTC()
	webhook.DisabledAt = &now
	stub.webhooks[id] = webhook
	return nil
}

type stubDeliveryRepo struct {
	mu         sync.Mutex
	deliveries map[string]domain.WebhookDelivery
}

func newStubDeliveryRepo() *stubDeliveryRepo {
	return &stubDeliveryRepo{deliveries: make(map[string]domain.WebhookDelivery)}
}

func (stub *stubDeliveryRepo) Create(_ context.Context, delivery domain.WebhookDelivery) error {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	stub.deliveries[delivery.ID] = delivery
	return nil
}

func (stub *stubDeliveryRepo) Find(_ context.Context, id string) (domain.WebhookDelivery, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	delivery, ok := stub.deliveries[id]
	if !ok {
		return domain.WebhookDelivery{}, domain.ErrWebhookDeliveryNotFound
	}
	return delivery, nil
}

func (stub *stubDeliveryRepo) List(_ context.Context, _, _ int) ([]domain.WebhookDelivery, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	out := make([]domain.WebhookDelivery, 0, len(stub.deliveries))
	for _, delivery := range stub.deliveries {
		out = append(out, delivery)
	}
	return out, nil
}

func (stub *stubDeliveryRepo) ListPending(_ context.Context, cutoff time.Time, _ int) ([]domain.WebhookDelivery, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	out := make([]domain.WebhookDelivery, 0)
	for _, delivery := range stub.deliveries {
		if delivery.Status != domain.WebhookDeliveryPending {
			continue
		}
		if delivery.NextAttemptAt != nil && delivery.NextAttemptAt.After(cutoff) {
			continue
		}
		out = append(out, delivery)
	}
	return out, nil
}

func (stub *stubDeliveryRepo) MarkDelivered(_ context.Context, id string, responseStatus int, when time.Time) error {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	delivery, ok := stub.deliveries[id]
	if !ok {
		return domain.ErrWebhookDeliveryNotFound
	}
	delivery.Status = domain.WebhookDeliveryDelivered
	delivery.ResponseStatus = responseStatus
	delivered := when.UTC()
	delivery.DeliveredAt = &delivered
	delivery.UpdatedAt = when.UTC()
	stub.deliveries[id] = delivery
	return nil
}

func (stub *stubDeliveryRepo) MarkFailed(_ context.Context, id string, attempt int, responseStatus int, lastError string, when time.Time) error {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	delivery, ok := stub.deliveries[id]
	if !ok {
		return domain.ErrWebhookDeliveryNotFound
	}
	delivery.Status = domain.WebhookDeliveryFailed
	delivery.Attempt = attempt
	delivery.ResponseStatus = responseStatus
	delivery.LastError = lastError
	delivery.UpdatedAt = when.UTC()
	stub.deliveries[id] = delivery
	return nil
}

func (stub *stubDeliveryRepo) ScheduleRetry(_ context.Context, id string, attempt int, responseStatus int, lastError string, nextAttemptAt time.Time, when time.Time) error {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	delivery, ok := stub.deliveries[id]
	if !ok {
		return domain.ErrWebhookDeliveryNotFound
	}
	delivery.Status = domain.WebhookDeliveryPending
	delivery.Attempt = attempt
	delivery.ResponseStatus = responseStatus
	delivery.LastError = lastError
	next := nextAttemptAt.UTC()
	delivery.NextAttemptAt = &next
	delivery.UpdatedAt = when.UTC()
	stub.deliveries[id] = delivery
	return nil
}

type recordingDispatcher struct {
	mu       sync.Mutex
	requests []dispatchRecord
}

type dispatchRecord struct {
	webhook  domain.Webhook
	delivery domain.WebhookDelivery
}

func (recorder *recordingDispatcher) Dispatch(_ context.Context, webhook domain.Webhook, delivery domain.WebhookDelivery) error {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	recorder.requests = append(recorder.requests, dispatchRecord{webhook: webhook, delivery: delivery})
	return nil
}

func newWebhookFixture() (*WebhookSubscriptions, *stubWebhookRepo, *stubDeliveryRepo, *recordingDispatcher) {
	repo := newStubWebhookRepo()
	deliveries := newStubDeliveryRepo()
	dispatcher := &recordingDispatcher{}
	useCase := NewWebhookSubscriptions(repo, deliveries, dispatcher, &sequentialIDs{})
	return useCase, repo, deliveries, dispatcher
}

func TestWebhookDispatchEventPersistsDelivery(t *testing.T) {
	useCase, repo, deliveries, dispatcher := newWebhookFixture()
	webhook, err := useCase.Create(context.Background(), "alerts", "https://hook.example.com", domain.WebhookEventAnalysisCompleted, "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_ = repo

	if err := useCase.DispatchEvent(context.Background(), domain.WebhookEventAnalysisCompleted, []byte(`{"x":1}`)); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	if len(dispatcher.requests) != 1 || dispatcher.requests[0].webhook.ID != webhook.ID {
		t.Fatalf("dispatcher should have been called once for the webhook, got %+v", dispatcher.requests)
	}
	stored := dispatcher.requests[0].delivery
	if stored.ID == "" || stored.Status != domain.WebhookDeliveryPending || stored.Event != domain.WebhookEventAnalysisCompleted {
		t.Fatalf("unexpected delivery passed to dispatcher: %+v", stored)
	}
	persisted, err := deliveries.Find(context.Background(), stored.ID)
	if err != nil {
		t.Fatalf("persisted delivery not found: %v", err)
	}
	if persisted.SubscriptionID != webhook.ID {
		t.Fatalf("delivery subscription mismatch: %+v", persisted)
	}
}

func TestWebhookRetryReschedulesAndDispatches(t *testing.T) {
	useCase, _, deliveries, dispatcher := newWebhookFixture()
	webhook, _ := useCase.Create(context.Background(), "alerts", "https://hook.example.com", domain.WebhookEventAnalysisFailed, "")
	_ = useCase.DispatchEvent(context.Background(), domain.WebhookEventAnalysisFailed, []byte(`{"err":1}`))
	original := dispatcher.requests[0].delivery
	if err := deliveries.MarkFailed(context.Background(), original.ID, 3, 502, "upstream 502", time.Now().UTC()); err != nil {
		t.Fatalf("mark failed: %v", err)
	}

	retried, err := useCase.Retry(context.Background(), original.ID)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if retried.Status != domain.WebhookDeliveryPending {
		t.Fatalf("expected pending status after retry, got %s", retried.Status)
	}
	if len(dispatcher.requests) != 2 || dispatcher.requests[1].webhook.ID != webhook.ID {
		t.Fatalf("dispatcher should have been called again, got %+v", dispatcher.requests)
	}
}

func TestWebhookUpdatePreservesSecretWhenOmitted(t *testing.T) {
	useCase, _, _, _ := newWebhookFixture()
	webhook, err := useCase.Create(context.Background(), "alerts", "https://hook.example.com", domain.WebhookEventAnalysisCompleted, "secret")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	updated, err := useCase.Update(context.Background(), webhook.ID, "ops", "https://ops.example.com", domain.WebhookEventAnalysisFailed, "")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != "ops" || updated.URL != "https://ops.example.com" || updated.Event != domain.WebhookEventAnalysisFailed {
		t.Fatalf("unexpected update: %+v", updated)
	}
	if updated.Secret != "secret" {
		t.Fatalf("expected secret to be preserved")
	}
}

func TestWebhookTestProducesPersistedAttempt(t *testing.T) {
	useCase, _, deliveries, dispatcher := newWebhookFixture()
	webhook, _ := useCase.Create(context.Background(), "ops", "https://hook.example.com", domain.WebhookEventAnalysisCompleted, "")

	delivery, err := useCase.Test(context.Background(), webhook.ID)
	if err != nil {
		t.Fatalf("test: %v", err)
	}
	if delivery.Event != domain.WebhookEventTest {
		t.Fatalf("expected webhook.test event, got %s", delivery.Event)
	}
	if _, err := deliveries.Find(context.Background(), delivery.ID); err != nil {
		t.Fatalf("test delivery was not persisted: %v", err)
	}
	if len(dispatcher.requests) != 1 || dispatcher.requests[0].delivery.Event != domain.WebhookEventTest {
		t.Fatalf("dispatcher should have received the test event, got %+v", dispatcher.requests)
	}
}

func TestWebhookRetryUnknownDeliveryReturnsNotFound(t *testing.T) {
	useCase, _, _, _ := newWebhookFixture()
	_, err := useCase.Retry(context.Background(), "missing")
	if !errors.Is(err, domain.ErrWebhookDeliveryNotFound) {
		t.Fatalf("expected ErrWebhookDeliveryNotFound, got %v", err)
	}
}

func TestWebhookDispatchPendingFlushesQueue(t *testing.T) {
	useCase, _, deliveries, dispatcher := newWebhookFixture()
	webhook, _ := useCase.Create(context.Background(), "ops", "https://hook.example.com", domain.WebhookEventAnalysisCompleted, "")
	_ = useCase.DispatchEvent(context.Background(), domain.WebhookEventAnalysisCompleted, []byte(`{"a":1}`))
	pendingID := dispatcher.requests[0].delivery.ID
	dispatcher.requests = nil

	if err := deliveries.ScheduleRetry(context.Background(), pendingID, 1, 0, "", time.Now().Add(-time.Minute), time.Now()); err != nil {
		t.Fatalf("schedule retry: %v", err)
	}
	count, err := useCase.DispatchPending(context.Background(), time.Now(), 10)
	if err != nil {
		t.Fatalf("dispatch pending: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 dispatched, got %d", count)
	}
	if len(dispatcher.requests) != 1 || dispatcher.requests[0].webhook.ID != webhook.ID {
		t.Fatalf("dispatcher should have re-attempted the delivery: %+v", dispatcher.requests)
	}
}
