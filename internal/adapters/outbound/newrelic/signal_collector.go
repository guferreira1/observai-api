package newrelic

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// NRQLTemplate is a New Relic NRQL query rendered per analyzed service.
//
// Use {service} as the substitution token in the query. The token is
// escaped before substitution so the embedded value cannot terminate the
// surrounding NRQL string literal.
type NRQLTemplate struct {
	Name  string
	Query string
	Unit  string
}

// SignalCollectorOptions configures the New Relic signal collector.
type SignalCollectorOptions struct {
	Templates []NRQLTemplate
}

// SignalCollector turns NRQL queries into normalized evidence.
type SignalCollector struct {
	client    *Client
	templates []NRQLTemplate
	now       func() time.Time
}

// NewSignalCollector builds a New Relic-backed implementation of ports.SignalCollector.
func NewSignalCollector(client *Client, opts SignalCollectorOptions) *SignalCollector {
	templates := opts.Templates
	if len(templates) == 0 {
		templates = defaultNRQLTemplates()
	}
	return &SignalCollector{client: client, templates: templates, now: time.Now}
}

// Collect runs the configured templates per affected service.
func (collector *SignalCollector) Collect(ctx context.Context, request domain.AnalysisRequest) ([]domain.Evidence, error) {
	if !shouldCollectMetrics(request.Signals) {
		return []domain.Evidence{}, nil
	}
	fallback := request.TimeWindow.End
	if fallback.IsZero() {
		fallback = collector.now().UTC()
	}

	evidence := make([]domain.Evidence, 0)
	for _, service := range nonEmpty(request.AffectedServices) {
		for _, template := range collector.templates {
			query := strings.ReplaceAll(template.Query, "{service}", escapeNRQLValue(service))
			samples, err := collector.client.QueryNRQL(ctx, query)
			if err != nil {
				return nil, fmt.Errorf("newrelic %s for %s: %w", template.Name, service, err)
			}
			if len(samples) == 0 {
				continue
			}
			for _, sample := range samples {
				evidence = append(evidence, sampleToEvidence(template, query, service, sample, fallback))
			}
		}
	}
	return evidence, nil
}

func sampleToEvidence(template NRQLTemplate, query, service string, sample NRQLSample, fallback time.Time) domain.Evidence {
	observed := sample.Observed
	if observed.IsZero() {
		observed = fallback
	}
	resolvedService := sample.Service
	if resolvedService == "" {
		resolvedService = service
	}
	return domain.Evidence{
		Signal:   domain.SignalMetrics,
		Service:  resolvedService,
		Source:   "newrelic",
		Name:     template.Name,
		Summary:  fmt.Sprintf("%s for %s = %g %s", template.Name, resolvedService, sample.Value, template.Unit),
		Observed: observed.UTC(),
		Score:    sample.Value,
		Unit:     template.Unit,
		Provider: "newrelic",
		Query:    query,
	}
}

func escapeNRQLValue(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return strings.ReplaceAll(value, `'`, `\'`)
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

func defaultNRQLTemplates() []NRQLTemplate {
	return []NRQLTemplate{
		{
			Name:  "transaction_duration_p95",
			Query: `SELECT percentile(duration, 95) FROM Transaction WHERE service.name = '{service}' SINCE 15 minutes ago`,
			Unit:  "ms",
		},
		{
			Name:  "error_rate",
			Query: `SELECT count(*) FROM TransactionError WHERE service.name = '{service}' SINCE 15 minutes ago`,
			Unit:  "errors",
		},
	}
}
