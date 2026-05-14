package main

import (
	"log/slog"

	"github.com/guferreira1/observai-api/internal/adapters/outbound/factory"
	"github.com/guferreira1/observai-api/internal/adapters/outbound/ollama"
	prometheusadapter "github.com/guferreira1/observai-api/internal/adapters/outbound/prometheus"
	"github.com/guferreira1/observai-api/internal/adapters/outbound/prompts"
	coreports "github.com/guferreira1/observai-api/internal/core/ports"
	"github.com/guferreira1/observai-api/internal/platform/config"
	"github.com/guferreira1/observai-api/internal/platform/observability"
)

// providers groups the runtime adapters used by the analysis and chat use cases.
//
// The composition root populates this struct through the outbound factory,
// which dispatches construction by provider type. Typed client handles are
// preserved so health probes can ping the underlying backends without
// re-establishing connections.
type providers struct {
	collector         coreports.SignalCollector
	generator         coreports.AnalysisGenerator
	responder         coreports.ChatResponder
	traces            coreports.TraceProvider
	traceProviderName string
	prometheus        *prometheusadapter.Client
	ollama            *ollama.Client
	llmProvider       string
	llmModel          string
	observabilityList []observabilityProvider
}

type observabilityProvider struct {
	name    string
	signals []string
}

// newProviders resolves observability and LLM adapters via the outbound factory.
//
// The dispatcher maps inside the factory enforce one builder per provider
// type. When the operator declared no provider, the null adapters take
// over and the request surface reports provider-not-configured per call.
func newProviders(cfg config.Config, log *slog.Logger, observer observability.ProviderObserver, credentials coreports.CredentialStore) (providers, error) {
	dependencies := factory.Dependencies{
		Logger:       log,
		Observer:     observer,
		PromptLoader: prompts.NewFileLoader(cfg.Prompts.Dir),
		Credentials:  credentials,
	}

	observabilityResult, err := factory.BuildObservability(cfg, dependencies)
	if err != nil {
		return providers{}, err
	}

	llmResult, err := factory.BuildLLM(cfg, dependencies)
	if err != nil {
		return providers{}, err
	}

	traceResult, err := factory.BuildTraceProvider(cfg, dependencies)
	if err != nil {
		return providers{}, err
	}

	return providers{
		collector:         observabilityResult.Collector,
		generator:         llmResult.Generator,
		responder:         llmResult.Responder,
		traces:            traceResult.Provider,
		traceProviderName: traceResult.Name,
		prometheus:        observabilityResult.Clients.Prometheus,
		ollama:            llmResult.Clients.Ollama,
		llmProvider:       llmResult.Provider,
		llmModel:          llmResult.Model,
		observabilityList: toObservabilityProviders(observabilityResult.Capabilities),
	}, nil
}

func toObservabilityProviders(capabilities []factory.ProviderCapability) []observabilityProvider {
	if len(capabilities) == 0 {
		return []observabilityProvider{{name: "none", signals: []string{}}}
	}
	out := make([]observabilityProvider, 0, len(capabilities))
	for _, capability := range capabilities {
		out = append(out, observabilityProvider{name: capability.Name, signals: capability.Signals})
	}
	return out
}
