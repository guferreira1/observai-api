package jaeger

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

const defaultTraceSearchLimit = 5

// SignalCollectorOptions configures Jaeger trace evidence collection.
type SignalCollectorOptions struct {
	Limit int
	Now   func() time.Time
}

// SignalCollector turns Jaeger trace search results into normalized evidence.
type SignalCollector struct {
	client *Client
	limit  int
	now    func() time.Time
}

// NewSignalCollector builds a Jaeger-backed trace signal collector.
func NewSignalCollector(client *Client, opts SignalCollectorOptions) *SignalCollector {
	limit := opts.Limit
	if limit <= 0 {
		limit = defaultTraceSearchLimit
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &SignalCollector{client: client, limit: limit, now: now}
}

// Collect searches Jaeger for recent traces matching the requested services.
func (collector *SignalCollector) Collect(ctx context.Context, request domain.AnalysisRequest) ([]domain.Evidence, error) {
	if !shouldCollectTraces(request.Signals) {
		return nil, nil
	}

	windowStart, windowEnd := collector.searchWindow(request.TimeWindow)
	evidence := make([]domain.Evidence, 0, len(request.AffectedServices))
	for _, service := range uniqueServices(request.AffectedServices) {
		traces, err := collector.client.SearchTraces(ctx, TraceSearchRequest{
			Service: service,
			Start:   windowStart,
			End:     windowEnd,
			Limit:   collector.limit,
		})
		if err != nil {
			return nil, fmt.Errorf("search jaeger traces for %s: %w", service, err)
		}
		if len(traces) == 0 {
			continue
		}
		evidence = append(evidence, traceEvidence(service, pickRepresentativeTrace(traces)))
	}
	return evidence, nil
}

func shouldCollectTraces(signals []domain.SignalType) bool {
	if len(signals) == 0 {
		return true
	}
	for _, signal := range signals {
		if signal == domain.SignalTraces {
			return true
		}
	}
	return false
}

func (collector *SignalCollector) searchWindow(window domain.TimeWindow) (time.Time, time.Time) {
	end := window.End
	if end.IsZero() {
		end = collector.now().UTC()
	}
	start := window.Start
	if start.IsZero() {
		start = end.Add(-1 * time.Hour)
	}
	return start.UTC(), end.UTC()
}

func uniqueServices(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	services := make([]string, 0, len(values))
	for _, raw := range values {
		service := strings.TrimSpace(raw)
		if service == "" {
			continue
		}
		if _, ok := seen[service]; ok {
			continue
		}
		seen[service] = struct{}{}
		services = append(services, service)
	}
	return services
}

func pickRepresentativeTrace(traces []Trace) Trace {
	if len(traces) == 0 {
		return Trace{}
	}
	sort.SliceStable(traces, func(left, right int) bool {
		return traceDurationUS(traces[left]) > traceDurationUS(traces[right])
	})
	return traces[0]
}

func traceEvidence(service string, trace Trace) domain.Evidence {
	durationUS := traceDurationUS(trace)
	errorCount := errorSpanCount(trace.Spans)
	attributes := map[string]string{
		"traceId":        trace.TraceID,
		"spanCount":      strconv.Itoa(len(trace.Spans)),
		"durationMs":     fmt.Sprintf("%.2f", float64(durationUS)/1000.0),
		"errorSpanCount": strconv.Itoa(errorCount),
	}
	if operation := rootOperation(trace); operation != "" {
		attributes["rootOperation"] = operation
	}

	return domain.Evidence{
		Signal:     domain.SignalTraces,
		Severity:   traceSeverity(errorCount),
		Service:    service,
		Source:     "jaeger",
		Provider:   "jaeger",
		Name:       "jaeger_trace_sample",
		Summary:    traceSummary(trace, service, durationUS, errorCount),
		Observed:   traceObservedAt(trace),
		Score:      1,
		Confidence: 1,
		Unit:       "trace",
		Reference:  trace.TraceID,
		Attributes: attributes,
	}
}

func traceSummary(trace Trace, service string, durationUS int64, errorCount int) string {
	durationMs := float64(durationUS) / 1000.0
	if errorCount > 0 {
		return fmt.Sprintf("Jaeger found trace %s for %s with %d spans, %.2fms duration and %d error spans.", trace.TraceID, service, len(trace.Spans), durationMs, errorCount)
	}
	return fmt.Sprintf("Jaeger found trace %s for %s with %d spans and %.2fms duration.", trace.TraceID, service, len(trace.Spans), durationMs)
}

func traceDurationUS(trace Trace) int64 {
	if len(trace.Spans) == 0 {
		return 0
	}
	start := trace.Spans[0].StartTimeUS
	end := trace.Spans[0].StartTimeUS + trace.Spans[0].DurationUS
	for _, span := range trace.Spans[1:] {
		if span.StartTimeUS < start {
			start = span.StartTimeUS
		}
		spanEnd := span.StartTimeUS + span.DurationUS
		if spanEnd > end {
			end = spanEnd
		}
	}
	if end < start {
		return 0
	}
	return end - start
}

func errorSpanCount(spans []Span) int {
	total := 0
	for _, span := range spans {
		if value := strings.ToLower(strings.TrimSpace(span.Tags["error"])); value == "true" || value == "1" {
			total++
			continue
		}
		if strings.EqualFold(strings.TrimSpace(span.Tags["otel.status_code"]), "ERROR") {
			total++
		}
	}
	return total
}

func traceSeverity(errorCount int) domain.Severity {
	if errorCount > 0 {
		return domain.SeverityHigh
	}
	return domain.SeverityLow
}

func rootOperation(trace Trace) string {
	for _, span := range trace.Spans {
		if span.ParentSpanID == "" {
			return span.OperationName
		}
	}
	if len(trace.Spans) == 0 {
		return ""
	}
	return trace.Spans[0].OperationName
}

func traceObservedAt(trace Trace) time.Time {
	if len(trace.Spans) == 0 || trace.Spans[0].StartTimeUS <= 0 {
		return time.Time{}
	}
	earliest := trace.Spans[0].StartTimeUS
	for _, span := range trace.Spans[1:] {
		if span.StartTimeUS > 0 && span.StartTimeUS < earliest {
			earliest = span.StartTimeUS
		}
	}
	return time.Unix(0, earliest*int64(time.Microsecond)).UTC()
}
