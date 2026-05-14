package jaeger

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// TraceProvider implements ports.TraceProvider over a Jaeger query API.
type TraceProvider struct {
	client *Client
}

// NewTraceProvider builds a Jaeger-backed trace provider.
func NewTraceProvider(client *Client) *TraceProvider {
	return &TraceProvider{client: client}
}

// FetchSpans retrieves the spans for the supplied trace ID and converts
// them into the provider-agnostic domain.Span shape.
func (provider *TraceProvider) FetchSpans(ctx context.Context, traceID string) ([]domain.Span, error) {
	trace, err := provider.client.FetchTrace(ctx, traceID)
	if err != nil {
		return nil, fmt.Errorf("fetch jaeger trace: %w", err)
	}

	if len(trace.Spans) == 0 {
		return []domain.Span{}, nil
	}

	parents := indexParents(trace.Spans)
	childDurations := map[string]int64{}
	for _, span := range trace.Spans {
		if span.ParentSpanID == "" {
			continue
		}
		childDurations[span.ParentSpanID] += span.DurationUS
	}

	spans := make([]domain.Span, 0, len(trace.Spans))
	for _, raw := range trace.Spans {
		service := ""
		if process, ok := trace.Processes[raw.ProcessID]; ok {
			service = process.ServiceName
		}
		durationMs := float64(raw.DurationUS) / 1000.0
		selfMs := durationMs
		if child := childDurations[raw.SpanID]; child > 0 {
			selfMs = (float64(raw.DurationUS) - float64(child)) / 1000.0
			if selfMs < 0 {
				selfMs = 0
			}
		}
		spans = append(spans, domain.Span{
			TraceID:      raw.TraceID,
			SpanID:       raw.SpanID,
			ParentSpanID: raw.ParentSpanID,
			Service:      service,
			Operation:    raw.OperationName,
			StartTime:    time.Unix(0, raw.StartTimeUS*int64(time.Microsecond)).UTC(),
			DurationMs:   durationMs,
			SelfTimeMs:   selfMs,
			Status:       statusFromTags(raw.Tags),
			Attributes:   normalizedAttributes(raw.Tags),
		})
	}

	_ = parents
	_ = strconv.Itoa
	return spans, nil
}

func indexParents(spans []Span) map[string]string {
	out := make(map[string]string, len(spans))
	for _, span := range spans {
		out[span.SpanID] = span.ParentSpanID
	}
	return out
}

func statusFromTags(tags map[string]string) domain.SpanStatus {
	if value, ok := tags["error"]; ok && (value == "true" || value == "1") {
		return domain.SpanStatusError
	}
	if value, ok := tags["otel.status_code"]; ok {
		switch value {
		case "ERROR":
			return domain.SpanStatusError
		case "OK":
			return domain.SpanStatusOk
		}
	}
	if _, ok := tags["span.kind"]; ok {
		return domain.SpanStatusOk
	}
	return domain.SpanStatusUnset
}

func normalizedAttributes(tags map[string]string) map[string]string {
	if len(tags) == 0 {
		return nil
	}
	out := make(map[string]string, len(tags))
	for key, value := range tags {
		out[key] = value
	}
	return out
}
