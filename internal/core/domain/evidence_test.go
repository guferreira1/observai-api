package domain

import (
	"testing"
	"time"
)

func TestEvidenceCorrelationKeyPrefersTraceID(t *testing.T) {
	evidence := Evidence{
		Service:    "checkout",
		Attributes: map[string]string{"traceId": "abc123", "spanId": "span-1"},
		Observed:   time.Now(),
	}
	if got := evidence.CorrelationKey(); got != "trace:abc123" {
		t.Fatalf("expected trace key, got %q", got)
	}
}

func TestEvidenceCorrelationKeyFallsBackToServiceSpan(t *testing.T) {
	evidence := Evidence{
		Service:    "checkout",
		Attributes: map[string]string{"spanId": "span-1"},
	}
	if got := evidence.CorrelationKey(); got != "span:checkout:span-1" {
		t.Fatalf("expected span key, got %q", got)
	}
}

func TestEvidenceCorrelationKeyMinuteBucket(t *testing.T) {
	evidence := Evidence{
		Service:  "checkout",
		Observed: time.Date(2026, 5, 14, 12, 34, 56, 0, time.UTC),
	}
	if got := evidence.CorrelationKey(); got != "minute:checkout:2026-05-14T12:34" {
		t.Fatalf("expected minute key, got %q", got)
	}
}

func TestGroupEvidenceByCorrelationOrdersByGroupSize(t *testing.T) {
	evidence := []Evidence{
		{Service: "checkout", Attributes: map[string]string{"traceId": "a"}},
		{Service: "checkout", Attributes: map[string]string{"traceId": "a"}},
		{Service: "payments", Attributes: map[string]string{"traceId": "b"}},
	}
	groups := GroupEvidenceByCorrelation(evidence)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	if groups[0].Key != "trace:a" || len(groups[0].Evidence) != 2 {
		t.Fatalf("biggest group first: %+v", groups[0])
	}
	if groups[1].Key != "trace:b" || len(groups[1].Evidence) != 1 {
		t.Fatalf("second group: %+v", groups[1])
	}
}

func TestNewMetricEvidenceForcesSignal(t *testing.T) {
	metric := NewMetricEvidence(Evidence{Service: "svc", Signal: SignalLogs})
	if metric.Signal != SignalMetrics {
		t.Fatalf("expected SignalMetrics, got %s", metric.Signal)
	}
}
