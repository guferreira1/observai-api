package datadog

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// MetricTemplate is a Datadog metric query rendered per analyzed service.
//
// Use {service} as the substitution token in the query. The token is
// escaped before substitution to defend against tag injection.
type MetricTemplate struct {
	Name  string
	Query string
	Unit  string
}

// SignalCollectorOptions configures the Datadog signal collector.
type SignalCollectorOptions struct {
	Templates []MetricTemplate
}

// SignalCollector turns Datadog metric queries into normalized evidence.
type SignalCollector struct {
	client    *Client
	templates []MetricTemplate
	now       func() time.Time
}

// NewSignalCollector builds a Datadog-backed implementation of ports.SignalCollector.
func NewSignalCollector(client *Client, opts SignalCollectorOptions) *SignalCollector {
	templates := opts.Templates
	if len(templates) == 0 {
		templates = defaultDatadogTemplates()
	}
	return &SignalCollector{client: client, templates: templates, now: time.Now}
}

// Collect runs the configured templates per affected service.
func (collector *SignalCollector) Collect(ctx context.Context, request domain.AnalysisRequest) ([]domain.Evidence, error) {
	if !shouldCollectMetrics(request.Signals) {
		return []domain.Evidence{}, nil
	}
	to := request.TimeWindow.End
	if to.IsZero() {
		to = collector.now().UTC()
	}
	from := request.TimeWindow.Start
	if from.IsZero() {
		from = to.Add(-15 * time.Minute)
	}

	evidence := make([]domain.Evidence, 0)
	for _, service := range nonEmpty(request.AffectedServices) {
		for _, template := range collector.templates {
			query := strings.ReplaceAll(template.Query, "{service}", escapeTagValue(service))
			samples, err := collector.client.QueryMetric(ctx, query, from, to)
			if err != nil {
				return nil, fmt.Errorf("datadog %s for %s: %w", template.Name, service, err)
			}
			if len(samples) == 0 {
				continue
			}
			for _, sample := range samples {
				evidence = append(evidence, sampleToEvidence(template, query, service, sample, to))
			}
		}
	}
	return evidence, nil
}

func sampleToEvidence(template MetricTemplate, query, service string, sample MetricSample, fallback time.Time) domain.Evidence {
	observed := sample.Observed
	if observed.IsZero() {
		observed = fallback
	}
	unit := template.Unit
	if unit == "" {
		unit = sample.Unit
	}
	return domain.Evidence{
		Signal:   domain.SignalMetrics,
		Service:  service,
		Source:   "datadog",
		Name:     template.Name,
		Summary:  fmt.Sprintf("%s for %s = %g %s", template.Name, service, sample.Value, unit),
		Observed: observed.UTC(),
		Score:    sample.Value,
		Unit:     unit,
		Provider: "datadog",
		Query:    query,
	}
}

func escapeTagValue(value string) string {
	value = strings.ReplaceAll(value, "{", "")
	value = strings.ReplaceAll(value, "}", "")
	return strings.TrimSpace(value)
}

func shouldCollectMetrics(signals []domain.SignalType) bool {
	if len(signals) == 0 {
		return true
	}
	for _, signal := range signals {
		if signal == domain.SignalMetrics {
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

func defaultDatadogTemplates() []MetricTemplate {
	return []MetricTemplate{
		{
			Name:  "service_request_latency_avg",
			Query: `avg:trace.servlet.request{service:{service}}`,
			Unit:  "ms",
		},
		{
			Name:  "service_error_rate",
			Query: `avg:trace.servlet.request.errors{service:{service}}`,
			Unit:  "rate",
		},
	}
}
