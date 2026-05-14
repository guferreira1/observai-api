package factory

import (
	"fmt"
	"strings"
	"time"

	"context"

	"github.com/guferreira1/observai-api/internal/adapters/outbound/composite"
	"github.com/guferreira1/observai-api/internal/adapters/outbound/elasticsearch"
	"github.com/guferreira1/observai-api/internal/adapters/outbound/loki"
	"github.com/guferreira1/observai-api/internal/adapters/outbound/null"
	prometheusadapter "github.com/guferreira1/observai-api/internal/adapters/outbound/prometheus"
	"github.com/guferreira1/observai-api/internal/core/policy"
	"github.com/guferreira1/observai-api/internal/core/ports"
	"github.com/guferreira1/observai-api/internal/platform/config"
)

// CollectorClients exposes the typed clients constructed alongside named
// collectors. Health probes and capability reporting depend on these handles
// to decide reachability without re-establishing connections.
type CollectorClients struct {
	Prometheus    *prometheusadapter.Client
	Loki          *loki.Client
	Elasticsearch *elasticsearch.Client
}

// ObservabilityResult is the outcome of wiring observability providers.
type ObservabilityResult struct {
	Collector    ports.SignalCollector
	Capabilities []ProviderCapability
	Clients      CollectorClients
}

// collectorBuilder constructs a named collector and a capability descriptor.
//
// The builder closes over typed adapter handles by writing them into the
// supplied CollectorClients; callers (cmd/observai-api/health.go) consult
// the populated handles to attach readiness probes.
type collectorBuilder func(provider config.ObservabilityProviderConfig, deps Dependencies, clients *CollectorClients) (composite.NamedCollector, ProviderCapability, error)

// collectorBuilders is the dispatcher map keyed by provider type. Adding a
// new observability provider means registering a builder here; the use
// case and HTTP layer do not change.
var collectorBuilders = map[string]collectorBuilder{
	"prometheus":    buildPrometheusCollector,
	"loki":          buildLokiCollector,
	"elasticsearch": buildElasticsearchCollector,
	"opensearch":    buildElasticsearchCollector,
}

// BuildObservability translates cfg.Observability into a single composite
// SignalCollector. When the operator did not configure any provider the
// composite is replaced by the null collector so requests fail closed
// instead of returning fabricated data.
func BuildObservability(cfg config.Config, deps Dependencies) (ObservabilityResult, error) {
	if len(cfg.Observability.Providers) == 0 {
		deps.Logger.Warn("no observability provider configured; signal collection will return provider-not-configured")
		return ObservabilityResult{Collector: null.NewSignalCollector()}, nil
	}

	clients := CollectorClients{}
	namedCollectors := make([]composite.NamedCollector, 0, len(cfg.Observability.Providers))
	capabilities := make([]ProviderCapability, 0, len(cfg.Observability.Providers))

	for _, provider := range cfg.Observability.Providers {
		providerType := strings.ToLower(strings.TrimSpace(provider.Type))

		if _, isTraceOnly := traceBuilders[providerType]; isTraceOnly {
			capabilities = append(capabilities, ProviderCapability{
				Name:    capabilityName(provider),
				Type:    providerType,
				Signals: nonEmptyStrings(provider.Signals, "traces"),
			})
			continue
		}

		builder, ok := collectorBuilders[providerType]
		if !ok {
			return ObservabilityResult{}, fmt.Errorf("unsupported observability provider type %q (supported: %s)", provider.Type, supportedCollectorTypes())
		}

		named, capability, err := builder(provider, deps, &clients)
		if err != nil {
			return ObservabilityResult{}, fmt.Errorf("init observability provider %q: %w", capabilityName(provider), err)
		}
		namedCollectors = append(namedCollectors, named)
		capabilities = append(capabilities, capability)
		deps.Logger.Info("observability provider enabled", "type", provider.Type, "name", named.Name)
	}

	if len(namedCollectors) == 0 {
		return ObservabilityResult{
			Collector:    null.NewSignalCollector(),
			Capabilities: capabilities,
			Clients:      clients,
		}, nil
	}

	collector := composite.NewSignalCollector(namedCollectors, composite.Options{
		ErrorPolicy: policy.NewPartialFailureCollectorErrorPolicy(),
		Logger:      deps.Logger,
	})

	return ObservabilityResult{
		Collector:    collector,
		Capabilities: capabilities,
		Clients:      clients,
	}, nil
}

func buildPrometheusCollector(provider config.ObservabilityProviderConfig, deps Dependencies, clients *CollectorClients) (composite.NamedCollector, ProviderCapability, error) {
	url := strings.TrimSpace(provider.URL)
	if url == "" {
		return composite.NamedCollector{}, ProviderCapability{}, fmt.Errorf("prometheus provider requires a non-empty url")
	}

	client, err := prometheusadapter.NewClient(prometheusadapter.ClientOptions{
		BaseURL:  url,
		Timeout:  defaultTimeout(provider.Timeout, 10*time.Second),
		Observer: deps.Observer,
	})
	if err != nil {
		return composite.NamedCollector{}, ProviderCapability{}, err
	}

	clients.Prometheus = client

	name := capabilityName(provider)
	collector := prometheusadapter.NewSignalCollector(client, prometheusadapter.SignalCollectorOptions{})
	signals := nonEmptyStrings(provider.Signals, "metrics")

	return composite.NamedCollector{
			Name:      name,
			Collector: collector,
		}, ProviderCapability{
			Name:    name,
			Type:    "prometheus",
			Signals: signals,
		}, nil
}

func buildLokiCollector(provider config.ObservabilityProviderConfig, deps Dependencies, clients *CollectorClients) (composite.NamedCollector, ProviderCapability, error) {
	url := strings.TrimSpace(provider.URL)
	if url == "" {
		return composite.NamedCollector{}, ProviderCapability{}, fmt.Errorf("loki provider requires a non-empty url")
	}

	client, err := loki.NewClient(loki.ClientOptions{
		BaseURL:  url,
		Timeout:  defaultTimeout(provider.Timeout, 10*time.Second),
		Observer: deps.Observer,
	})
	if err != nil {
		return composite.NamedCollector{}, ProviderCapability{}, err
	}

	clients.Loki = client

	name := capabilityName(provider)
	collector := loki.NewSignalCollector(client, loki.SignalCollectorOptions{})
	signals := nonEmptyStrings(provider.Signals, "logs")

	return composite.NamedCollector{
			Name:      name,
			Collector: collector,
		}, ProviderCapability{
			Name:    name,
			Type:    "loki",
			Signals: signals,
		}, nil
}

func buildElasticsearchCollector(provider config.ObservabilityProviderConfig, deps Dependencies, clients *CollectorClients) (composite.NamedCollector, ProviderCapability, error) {
	url := strings.TrimSpace(provider.URL)
	if url == "" {
		return composite.NamedCollector{}, ProviderCapability{}, fmt.Errorf("elasticsearch provider requires a non-empty url")
	}

	clientOpts := elasticsearch.ClientOptions{
		BaseURL:  url,
		Username: strings.TrimSpace(provider.Options["username"]),
		Password: strings.TrimSpace(provider.Options["password"]),
		Timeout:  defaultTimeout(provider.Timeout, 10*time.Second),
		Observer: deps.Observer,
	}
	if reference := strings.TrimSpace(provider.Options["api_key"]); reference != "" {
		resolved, err := resolveCredential(context.Background(), deps.Credentials, reference)
		if err != nil {
			return composite.NamedCollector{}, ProviderCapability{}, fmt.Errorf("resolve elasticsearch api key: %w", err)
		}
		clientOpts.APIKey = resolved
	}
	if reference := strings.TrimSpace(provider.Options["password_ref"]); reference != "" {
		resolved, err := resolveCredential(context.Background(), deps.Credentials, reference)
		if err != nil {
			return composite.NamedCollector{}, ProviderCapability{}, fmt.Errorf("resolve elasticsearch password: %w", err)
		}
		clientOpts.Password = resolved
	}

	client, err := elasticsearch.NewClient(clientOpts)
	if err != nil {
		return composite.NamedCollector{}, ProviderCapability{}, err
	}
	clients.Elasticsearch = client

	name := capabilityName(provider)
	collector := elasticsearch.NewSignalCollector(client, elasticsearch.SignalCollectorOptions{
		Index:          provider.Options["index"],
		ErrorPattern:   provider.Options["error_pattern"],
		TimestampField: provider.Options["timestamp_field"],
		ServiceField:   provider.Options["service_field"],
		MessageField:   provider.Options["message_field"],
	})
	signals := nonEmptyStrings(provider.Signals, "logs")

	providerType := strings.ToLower(strings.TrimSpace(provider.Type))
	if providerType == "" {
		providerType = "elasticsearch"
	}

	return composite.NamedCollector{
			Name:      name,
			Collector: collector,
		}, ProviderCapability{
			Name:    name,
			Type:    providerType,
			Signals: signals,
		}, nil
}

func supportedCollectorTypes() string {
	types := make([]string, 0, len(collectorBuilders))
	for providerType := range collectorBuilders {
		types = append(types, providerType)
	}
	return strings.Join(types, ", ")
}

func capabilityName(provider config.ObservabilityProviderConfig) string {
	name := strings.TrimSpace(provider.Name)
	if name == "" {
		return strings.TrimSpace(provider.Type)
	}
	return name
}

func nonEmptyStrings(values []string, fallback ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		out = append(out, fallback...)
	}
	return out
}
