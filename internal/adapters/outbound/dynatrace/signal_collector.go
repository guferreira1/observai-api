package dynatrace

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// MetricTemplate is a Dynatrace metric selector rendered per analyzed service.
//
// Use {service} as the substitution token in the selector. The token is
// escaped before substitution to defend against metric-selector injection.
type MetricTemplate struct {
	Name       string
	Selector   string
	Resolution string
	Unit       string
}

// SignalCollectorOptions configures the Dynatrace signal collector.
type SignalCollectorOptions struct {
	Templates []MetricTemplate
}

// SignalCollector turns Dynatrace metric queries into normalized evidence.
type SignalCollector struct {
	client    *Client
	templates []MetricTemplate
	now       func() time.Time
}

// NewSignalCollector builds a Dynatrace-backed implementation of ports.SignalCollector.
func NewSignalCollector(client *Client, opts SignalCollectorOptions) *SignalCollector {
	templates := opts.Templates
	if len(templates) == 0 {
		templates = defaultDynatraceTemplates()
	}
	return &SignalCollector{client: client, templates: templates, now: time.Now}
}

// Collect runs the configured templates per affected service and emits evidence.
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
	for _, service := range nonEmptyServices(request.AffectedServices) {
		for _, template := range collector.templates {
			selector := strings.ReplaceAll(template.Selector, "{service}", escapeSelectorValue(service))

			samples, err := collector.client.QueryMetric(ctx, selector, template.Resolution, from, to)
			if err != nil {
				return nil, fmt.Errorf("dynatrace metric %s for %s: %w", template.Name, service, err)
			}
			if len(samples) == 0 {
				continue
			}
			for _, sample := range samples {
				evidence = append(evidence, sampleToEvidence(template, selector, service, sample, to))
			}
		}
	}
	return evidence, nil
}

func sampleToEvidence(template MetricTemplate, selector, service string, sample MetricSample, fallback time.Time) domain.Evidence {
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
		Source:   "dynatrace",
		Name:     template.Name,
		Summary:  fmt.Sprintf("%s for %s = %g %s", template.Name, service, sample.Value, unit),
		Observed: observed.UTC(),
		Score:    sample.Value,
		Unit:     unit,
		Provider: "dynatrace",
		Query:    selector,
	}
}

func escapeSelectorValue(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return strings.ReplaceAll(value, `"`, `\"`)
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

func nonEmptyServices(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func defaultDynatraceTemplates() []MetricTemplate {
	return []MetricTemplate{
		{
			Name:     "service_response_time_avg",
			Selector: `builtin:service.response.time:filter(eq("dt.entity.service.name","{service}")):avg`,
			Unit:     "ms",
		},
		{
			Name:     "service_error_rate",
			Selector: `builtin:service.errors.total.rate:filter(eq("dt.entity.service.name","{service}"))`,
			Unit:     "rate",
		},
	}
}
