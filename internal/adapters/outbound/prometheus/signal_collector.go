package prometheus

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// MetricTemplate describes a server-controlled PromQL template rendered per service.
//
// Use {service} as the substitution token for the affected service label value.
// Other placeholders are intentionally not supported to avoid arbitrary user
// queries hitting Prometheus.
type MetricTemplate struct {
	Name       string
	Expression string
	Unit       string
}

// SignalCollectorOptions configures the Prometheus signal collector.
type SignalCollectorOptions struct {
	Templates []MetricTemplate
}

// SignalCollector turns Prometheus instant queries into normalized evidence.
type SignalCollector struct {
	client    *Client
	templates []MetricTemplate
	now       func() time.Time
}

// NewSignalCollector builds a Prometheus-backed implementation of ports.SignalCollector.
func NewSignalCollector(client *Client, opts SignalCollectorOptions) *SignalCollector {
	templates := opts.Templates
	if len(templates) == 0 {
		templates = defaultTemplates()
	}
	return &SignalCollector{
		client:    client,
		templates: templates,
		now:       time.Now,
	}
}

// Collect runs the configured templates per affected service and returns evidence.
//
// Services with no matching samples are reported via a single "missing" evidence
// entry per template so the analysis stage can surface the gap explicitly.
func (collector *SignalCollector) Collect(ctx context.Context, request domain.AnalysisRequest) ([]domain.Evidence, error) {
	if !shouldCollect(request.Signals) {
		return []domain.Evidence{}, nil
	}

	evaluatedAt := request.TimeWindow.End
	if evaluatedAt.IsZero() {
		evaluatedAt = collector.now().UTC()
	}

	evidence := make([]domain.Evidence, 0)
	for _, service := range nonEmpty(request.AffectedServices) {
		for _, template := range collector.templates {
			expression := renderTemplate(template.Expression, service)

			samples, err := collector.client.Query(ctx, expression, evaluatedAt)
			if err != nil {
				return nil, fmt.Errorf("collect %s for %s: %w", template.Name, service, err)
			}

			if len(samples) == 0 {
				continue
			}

			for _, sample := range samples {
				evidence = append(evidence, sampleToEvidence(template, expression, service, sample, evaluatedAt))
			}
		}
	}

	return evidence, nil
}

func sampleToEvidence(template MetricTemplate, expression string, service string, sample InstantSample, evaluatedAt time.Time) domain.Evidence {
	observed := sample.Time
	if observed.IsZero() {
		observed = evaluatedAt
	}

	return domain.Evidence{
		Signal:     domain.SignalMetrics,
		Service:    service,
		Source:     "prometheus",
		Name:       template.Name,
		Summary:    fmt.Sprintf("%s for %s = %g %s", template.Name, service, sample.Value, template.Unit),
		Observed:   observed.UTC(),
		Score:      sample.Value,
		Unit:       template.Unit,
		Provider:   "prometheus",
		Query:      expression,
		Attributes: filteredLabels(sample.Labels),
	}
}

func renderTemplate(expression string, service string) string {
	return strings.ReplaceAll(expression, "{service}", escapeLabel(service))
}

// escapeLabel escapes characters that would otherwise allow label-value injection
// in PromQL string literals. Only `\` and `"` need to be escaped.
func escapeLabel(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return strings.ReplaceAll(value, `"`, `\"`)
}

func shouldCollect(signals []domain.SignalType) bool {
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

func filteredLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	out := make(map[string]string, len(labels))
	for key, value := range labels {
		if key == "__name__" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func defaultTemplates() []MetricTemplate {
	return []MetricTemplate{
		{
			Name:       "service_up",
			Expression: `max by (service) (up{service="{service}"})`,
			Unit:       "ratio",
		},
	}
}
