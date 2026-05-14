package factory

import (
	"fmt"
	"strings"
	"time"

	"github.com/guferreira1/observai-api/internal/adapters/outbound/jaeger"
	"github.com/guferreira1/observai-api/internal/adapters/outbound/null"
	"github.com/guferreira1/observai-api/internal/adapters/outbound/otel"
	"github.com/guferreira1/observai-api/internal/core/ports"
	"github.com/guferreira1/observai-api/internal/platform/config"
)

// TraceClients exposes typed clients for the active trace provider so
// health probes can attach without re-establishing connections.
type TraceClients struct {
	Jaeger *jaeger.Client
	OTel   *otel.Client
}

// TraceResult is the outcome of resolving a trace provider.
type TraceResult struct {
	Provider ports.TraceProvider
	Name     string
	Type     string
	Clients  TraceClients
}

type traceBuilder func(provider config.ObservabilityProviderConfig, deps Dependencies, clients *TraceClients) (TraceResult, error)

var traceBuilders = map[string]traceBuilder{
	"jaeger": buildJaegerTrace,
	"tempo":  buildOTelTrace,
	"otel":   buildOTelTrace,
	"otlp":   buildOTelTrace,
}

// BuildTraceProvider inspects cfg.Observability.Providers for the first
// trace-capable entry and constructs its adapter. When no trace provider is
// configured the null adapter is returned, which makes the trace endpoint
// fail-closed with ErrProviderNotConfigured.
func BuildTraceProvider(cfg config.Config, deps Dependencies) (TraceResult, error) {
	for _, provider := range cfg.Observability.Providers {
		builder, ok := traceBuilders[strings.ToLower(strings.TrimSpace(provider.Type))]
		if !ok {
			continue
		}
		clients := TraceClients{}
		result, err := builder(provider, deps, &clients)
		if err != nil {
			return TraceResult{}, fmt.Errorf("init trace provider %q: %w", capabilityName(provider), err)
		}
		result.Clients = clients
		deps.Logger.Info("trace provider enabled", "type", provider.Type, "name", result.Name)
		return result, nil
	}

	deps.Logger.Warn("no trace provider configured; /v1/analyses/{id}/traces will return provider-not-configured")
	return TraceResult{Provider: null.NewTraceProvider(), Name: "none", Type: "none"}, nil
}

func buildJaegerTrace(provider config.ObservabilityProviderConfig, deps Dependencies, clients *TraceClients) (TraceResult, error) {
	url := strings.TrimSpace(provider.URL)
	if url == "" {
		return TraceResult{}, fmt.Errorf("jaeger trace provider requires a non-empty url")
	}
	client, err := jaeger.NewClient(jaeger.ClientOptions{
		BaseURL:  url,
		Timeout:  defaultTimeout(provider.Timeout, 15*time.Second),
		Observer: deps.Observer,
	})
	if err != nil {
		return TraceResult{}, err
	}
	clients.Jaeger = client
	return TraceResult{
		Provider: jaeger.NewTraceProvider(client),
		Name:     capabilityName(provider),
		Type:     "jaeger",
	}, nil
}

func buildOTelTrace(provider config.ObservabilityProviderConfig, deps Dependencies, clients *TraceClients) (TraceResult, error) {
	url := strings.TrimSpace(provider.URL)
	if url == "" {
		return TraceResult{}, fmt.Errorf("otel trace provider requires a non-empty url")
	}
	client, err := otel.NewClient(otel.ClientOptions{
		BaseURL:  url,
		Timeout:  defaultTimeout(provider.Timeout, 15*time.Second),
		Observer: deps.Observer,
	})
	if err != nil {
		return TraceResult{}, err
	}
	clients.OTel = client
	return TraceResult{
		Provider: otel.NewTraceProvider(client),
		Name:     capabilityName(provider),
		Type:     strings.ToLower(strings.TrimSpace(provider.Type)),
	}, nil
}
