package main

import (
	"context"
	"errors"
	"log/slog"

	"github.com/guferreira1/observai-api/internal/adapters/outbound/factory"
	"github.com/guferreira1/observai-api/internal/adapters/outbound/ollama"
	prometheusadapter "github.com/guferreira1/observai-api/internal/adapters/outbound/prometheus"
	"github.com/guferreira1/observai-api/internal/adapters/outbound/prompts"
	coreports "github.com/guferreira1/observai-api/internal/core/ports"
	"github.com/guferreira1/observai-api/internal/platform/config"
	"github.com/guferreira1/observai-api/internal/platform/crypto"
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

// providerInventoryFromConfig adapts the current configuration into a
// snapshot for the Setup use case. Kept for the local/dev fallback when no
// DB-backed repository is wired.
func providerInventoryFromConfig(cfg config.Config) coreports.ProviderInventory {
	return coreports.ProviderInventoryFunc(func() coreports.ProviderInventorySnapshot {
		return coreports.ProviderInventorySnapshot{
			ObservabilityProviders: len(cfg.Observability.Providers),
			LLMProviders:           len(cfg.LLM.Providers),
		}
	})
}

// providerInventoryFromStores prefers the DB-backed provider/LLM
// configurations when available, falling back to the YAML/env-derived
// counts otherwise. The snapshot is computed at call time so the setup
// status endpoint reflects the live state without restart.
func providerInventoryFromStores(store analysisStore, cfg config.Config) coreports.ProviderInventory {
	if store.providerConfigs == nil || store.llmConfigs == nil {
		return providerInventoryFromConfig(cfg)
	}
	return coreports.ProviderInventoryFunc(func() coreports.ProviderInventorySnapshot {
		ctx, cancel := context.WithTimeout(context.Background(), 2*1e9)
		defer cancel()
		providerCount, err := store.providerConfigs.Count(ctx)
		if err != nil {
			providerCount = int64(len(cfg.Observability.Providers))
		}
		llmCount, err := store.llmConfigs.Count(ctx)
		if err != nil {
			llmCount = int64(len(cfg.LLM.Providers))
		}
		return coreports.ProviderInventorySnapshot{
			ObservabilityProviders: int(providerCount),
			LLMProviders:           int(llmCount),
		}
	})
}

// buildEncryptionCipher returns the cipher used by the provider/LLM
// configuration use cases. In local mode without a configured key the
// function generates a volatile key for the session and warns; production
// startup already enforces the presence of the key via config.Validate.
func buildEncryptionCipher(cfg config.Config, log *slog.Logger) (coreports.Cipher, error) {
	key, err := cfg.LoadEncryptionKey()
	if err != nil {
		if !errors.Is(err, config.ErrEncryptionKeyMissing) {
			return nil, err
		}
		volatile, err := crypto.GenerateKey()
		if err != nil {
			return nil, err
		}
		log.Warn("encryption key missing; generated volatile in-memory key for local session")
		key = volatile
	}
	return crypto.NewAESGCMCipher(key)
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
