package loki

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// LogTemplate renders a LogQL query per affected service. {service} is the
// only substitution token recognized; everything else is forwarded verbatim
// so operators can compose richer aggregations without enabling arbitrary
// user input.
type LogTemplate struct {
	Name       string
	Expression string
	Unit       string
}

// SignalCollectorOptions configures the Loki collector.
type SignalCollectorOptions struct {
	Templates []LogTemplate
	Step      time.Duration
}

// SignalCollector turns Loki log volume queries into normalized evidence.
type SignalCollector struct {
	client    *Client
	templates []LogTemplate
	step      time.Duration
	now       func() time.Time
}

// NewSignalCollector builds a Loki-backed implementation of ports.SignalCollector.
func NewSignalCollector(client *Client, opts SignalCollectorOptions) *SignalCollector {
	templates := opts.Templates
	if len(templates) == 0 {
		templates = defaultTemplates()
	}
	step := opts.Step
	if step <= 0 {
		step = 60 * time.Second
	}
	return &SignalCollector{
		client:    client,
		templates: templates,
		step:      step,
		now:       time.Now,
	}
}

// Collect evaluates every template per affected service over the request
// time window and emits one Evidence per non-empty series.
func (collector *SignalCollector) Collect(ctx context.Context, request domain.AnalysisRequest) ([]domain.Evidence, error) {
	if !shouldCollect(request.Signals) {
		return []domain.Evidence{}, nil
	}

	start, end := resolveWindow(request, collector.now)

	evidence := make([]domain.Evidence, 0)
	for _, service := range nonEmpty(request.AffectedServices) {
		for _, template := range collector.templates {
			expression := renderTemplate(template.Expression, service)
			samples, err := collector.client.QueryRange(ctx, expression, start, end, collector.step)
			if err != nil {
				return nil, fmt.Errorf("collect %s for %s: %w", template.Name, service, err)
			}
			for _, sample := range samples {
				evidence = append(evidence, sampleToEvidence(template, expression, service, sample, end))
			}
		}
	}

	return evidence, nil
}

func sampleToEvidence(template LogTemplate, expression string, service string, sample Sample, evaluatedAt time.Time) domain.Evidence {
	observed := sample.Time
	if observed.IsZero() {
		observed = evaluatedAt
	}
	return domain.Evidence{
		Signal:     domain.SignalLogs,
		Service:    service,
		Source:     "loki",
		Name:       template.Name,
		Summary:    fmt.Sprintf("%s for %s = %g %s", template.Name, service, sample.Value, template.Unit),
		Observed:   observed.UTC(),
		Score:      sample.Value,
		Unit:       template.Unit,
		Provider:   "loki",
		Query:      expression,
		Attributes: filteredLabels(sample.Labels),
	}
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

func renderTemplate(expression string, service string) string {
	return strings.ReplaceAll(expression, "{service}", escapeLabel(service))
}

// escapeLabel escapes characters that would otherwise allow label-value
// injection in LogQL string literals. Only `\` and `"` need to be escaped.
func escapeLabel(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return strings.ReplaceAll(value, `"`, `\"`)
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

func filteredLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	out := make(map[string]string, len(labels))
	for key, value := range labels {
		out[key] = value
	}
	return out
}

func defaultTemplates() []LogTemplate {
	return []LogTemplate{
		{
			Name:       "error_log_volume",
			Expression: `sum by (service) (count_over_time({service="{service}"} |~ "(?i)(error|exception|panic)" [5m]))`,
			Unit:       "events_per_5m",
		},
	}
}
