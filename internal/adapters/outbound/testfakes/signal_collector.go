package testfakes

import (
	"context"
	"strconv"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// SignalCollector returns deterministic evidence for tests.
type SignalCollector struct{}

// NewSignalCollector creates a deterministic signal collector for tests.
func NewSignalCollector() *SignalCollector {
	return &SignalCollector{}
}

// Collect returns deterministic evidence based on the requested signal types.
func (collector *SignalCollector) Collect(_ context.Context, request domain.AnalysisRequest) ([]domain.Evidence, error) {
	signals := request.Signals
	if len(signals) == 0 {
		signals = []domain.SignalType{
			domain.SignalLogs,
			domain.SignalMetrics,
			domain.SignalTraces,
			domain.SignalAPM,
		}
	}

	service := "unknown-service"
	if len(request.AffectedServices) > 0 {
		service = request.AffectedServices[0]
	}

	observed := request.TimeWindow.End
	if observed.IsZero() {
		observed = time.Unix(0, 0).UTC()
	}

	evidence := make([]domain.Evidence, 0, len(signals))
	for index, signal := range signals {
		evidence = append(evidence, domain.Evidence{
			Signal:   signal,
			Service:  service,
			Source:   "testfake",
			Name:     string(signal) + "_summary",
			Summary:  "deterministic " + string(signal) + " evidence for " + request.Goal,
			Observed: observed,
			Score:    1,
			Unit:     "count",
			Attributes: map[string]string{
				"correlationId": "test-correlation-" + service,
				"logId":         "log-" + service,
				"metricId":      "metric-" + service,
				"spanId":        "span-" + service,
				"traceId":       "trace-" + service,
				"index":         strconv.Itoa(index),
			},
		})
	}

	return evidence, nil
}
