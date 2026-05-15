package usecase

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// WebhookNotifier adapts the WebhookSubscriptions use case to the
// AnalysisCompletionNotifier interface expected by the Analysis worker.
//
// The notifier marshals the analysis payload, attaches the standard event
// metadata and delegates the fan-out to the subscriptions use case, which
// in turn calls the webhooks dispatcher. Failures are logged but never
// propagated so they cannot fail the analysis job.
type WebhookNotifier struct {
	subscriptions *WebhookSubscriptions
	logger        *slog.Logger
}

// NewWebhookNotifier builds an AnalysisCompletionNotifier that fan-outs
// terminal job transitions to subscribed webhooks.
func NewWebhookNotifier(subscriptions *WebhookSubscriptions, logger *slog.Logger) *WebhookNotifier {
	if logger == nil {
		logger = slog.Default()
	}
	return &WebhookNotifier{subscriptions: subscriptions, logger: logger}
}

type webhookPayload struct {
	Event     string         `json:"event"`
	Timestamp time.Time      `json:"timestamp"`
	Data      map[string]any `json:"data"`
}

// NotifyAnalysisCompleted dispatches the analysis.completed event.
func (notifier *WebhookNotifier) NotifyAnalysisCompleted(ctx context.Context, result domain.AnalysisResult) {
	notifier.dispatch(ctx, domain.WebhookEventAnalysisCompleted, map[string]any{
		"analysisId":       result.ID,
		"summary":          result.Summary,
		"severity":         string(result.Severity),
		"confidence":       string(result.Confidence),
		"affectedServices": result.AffectedServices,
		"traceId":          result.TraceID,
		"createdAt":        result.CreatedAt,
	})
}

// NotifyAnalysisFailed dispatches the analysis.failed event.
func (notifier *WebhookNotifier) NotifyAnalysisFailed(ctx context.Context, jobID string, request domain.AnalysisRequest, reason string) {
	notifier.dispatch(ctx, domain.WebhookEventAnalysisFailed, map[string]any{
		"jobId":            jobID,
		"goal":             request.Goal,
		"affectedServices": request.AffectedServices,
		"reason":           reason,
	})
}

// NotifyAnalysisCanceled dispatches the analysis.canceled event.
func (notifier *WebhookNotifier) NotifyAnalysisCanceled(ctx context.Context, jobID string, request domain.AnalysisRequest) {
	notifier.dispatch(ctx, domain.WebhookEventAnalysisCanceled, map[string]any{
		"jobId":            jobID,
		"goal":             request.Goal,
		"affectedServices": request.AffectedServices,
	})
}

func (notifier *WebhookNotifier) dispatch(ctx context.Context, event string, data map[string]any) {
	if notifier.subscriptions == nil {
		return
	}
	payload, err := json.Marshal(webhookPayload{
		Event:     event,
		Timestamp: time.Now().UTC(),
		Data:      data,
	})
	if err != nil {
		notifier.logger.Warn("webhook payload encode failed", "event", event, "error", err)
		return
	}
	if err := notifier.subscriptions.DispatchEvent(ctx, event, payload); err != nil {
		notifier.logger.Warn("webhook dispatch failed", "event", event, "error", err)
	}
}
