package elasticsearch

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// SignalCollectorOptions configures the Elasticsearch logs collector.
type SignalCollectorOptions struct {
	Index          string
	ErrorPattern   string
	TimestampField string
	ServiceField   string
	MessageField   string
}

// SignalCollector emits log volume evidence by querying Elasticsearch /
// OpenSearch for documents that match an error-shaped regex per service.
type SignalCollector struct {
	client *Client
	opts   SignalCollectorOptions
	now    func() time.Time
}

// NewSignalCollector builds a SignalCollector. Options carry safe defaults
// (`@timestamp`, `service.name`, `message`) so most setups need only the
// base URL.
func NewSignalCollector(client *Client, opts SignalCollectorOptions) *SignalCollector {
	if strings.TrimSpace(opts.ErrorPattern) == "" {
		opts.ErrorPattern = "(?i).*(error|exception|panic).*"
	}
	return &SignalCollector{client: client, opts: opts, now: time.Now}
}

// Collect issues one count query per affected service against Elasticsearch
// and returns one piece of evidence per non-zero result.
func (collector *SignalCollector) Collect(ctx context.Context, request domain.AnalysisRequest) ([]domain.Evidence, error) {
	if !shouldCollect(request.Signals) {
		return []domain.Evidence{}, nil
	}

	start, end := resolveWindow(request, collector.now)
	evidence := make([]domain.Evidence, 0)
	for _, service := range nonEmpty(request.AffectedServices) {
		count, err := collector.client.CountMatchingLogs(ctx, SearchOptions{
			Index:          collector.opts.Index,
			Service:        service,
			ServiceField:   collector.opts.ServiceField,
			MessagePattern: collector.opts.ErrorPattern,
			MessageField:   collector.opts.MessageField,
			TimestampField: collector.opts.TimestampField,
			WindowStart:    start,
			WindowEnd:      end,
		})
		if err != nil {
			return nil, fmt.Errorf("collect logs for %s: %w", service, err)
		}
		if count == 0 {
			continue
		}
		evidence = append(evidence, domain.Evidence{
			Signal:   domain.SignalLogs,
			Service:  service,
			Source:   "elasticsearch",
			Name:     "error_log_count",
			Summary:  fmt.Sprintf("error_log_count for %s = %d events in window", service, count),
			Observed: end.UTC(),
			Score:    float64(count),
			Unit:     "events",
			Provider: "elasticsearch",
			Query:    collector.opts.ErrorPattern,
		})
	}
	return evidence, nil
}

func resolveWindow(request domain.AnalysisRequest, now func() time.Time) (time.Time, time.Time) {
	start := request.TimeWindow.Start
	end := request.TimeWindow.End
	if end.IsZero() {
		end = now().UTC()
	}
	if start.IsZero() || !start.Before(end) {
		start = end.Add(-15 * time.Minute)
	}
	return start.UTC(), end.UTC()
}

func shouldCollect(signals []domain.SignalType) bool {
	if len(signals) == 0 {
		return true
	}
	for _, signal := range signals {
		if signal == domain.SignalLogs {
			return true
		}
	}
	return false
}

func nonEmpty(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
