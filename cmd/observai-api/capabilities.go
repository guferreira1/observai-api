package main

import (
	"sync"

	inboundhttp "github.com/guferreira1/observai-api/internal/adapters/inbound/http"
	"github.com/guferreira1/observai-api/internal/platform/config"
)

// buildCapabilities assembles the non-sensitive runtime descriptor exposed by GET /v1/capabilities.
//
// It only echoes information that is safe to expose: deployment mode, provider names,
// configured LLM model, request limits and the build version. Secrets, URLs and
// tokens never appear here.
func buildCapabilities(cfg config.Config, runtime providers, version string) inboundhttp.CapabilitiesResponse {
	return inboundhttp.CapabilitiesResponse{
		Mode:    string(cfg.Mode),
		Version: version,
		LLM: inboundhttp.CapabilityLLM{
			Provider: runtime.llmProvider,
			Model:    runtime.llmModel,
		},
		Observability: toCapabilityProviders(runtime.observabilityList),
		Limits: inboundhttp.CapabilityLimits{
			HTTPRequestTimeoutMs: cfg.HTTPRequestTimeout.Milliseconds(),
			HTTPMaxBodyBytes:     cfg.HTTPMaxBodyBytes,
			RateLimitRPS:         cfg.HTTPRateLimit.RequestsPerSecond,
			RateLimitBurst:       cfg.HTTPRateLimit.Burst,
		},
	}
}

type capabilitiesStore struct {
	mu      sync.RWMutex
	current inboundhttp.CapabilitiesResponse
}

func newCapabilitiesStore(initial inboundhttp.CapabilitiesResponse) *capabilitiesStore {
	return &capabilitiesStore{current: cloneCapabilities(initial)}
}

func (store *capabilitiesStore) Get() inboundhttp.CapabilitiesResponse {
	store.mu.RLock()
	defer store.mu.RUnlock()
	return cloneCapabilities(store.current)
}

func (store *capabilitiesStore) Set(next inboundhttp.CapabilitiesResponse) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.current = cloneCapabilities(next)
}

func (store *capabilitiesStore) ProviderSummary() inboundhttp.ProviderSummary {
	capabilities := store.Get()
	return inboundhttp.ProviderSummary{
		Mode:          capabilities.Mode,
		LLM:           capabilities.LLM.Provider,
		Observability: providerNames(capabilities.Observability),
	}
}

func toCapabilityProviders(items []observabilityProvider) []inboundhttp.CapabilityProvider {
	observability := make([]inboundhttp.CapabilityProvider, 0, len(items))
	for _, item := range items {
		observability = append(observability, inboundhttp.CapabilityProvider{
			Provider: item.name,
			Signals:  append([]string(nil), item.signals...),
		})
	}
	return observability
}

func cloneCapabilities(source inboundhttp.CapabilitiesResponse) inboundhttp.CapabilitiesResponse {
	clone := source
	clone.Observability = make([]inboundhttp.CapabilityProvider, 0, len(source.Observability))
	for _, provider := range source.Observability {
		clone.Observability = append(clone.Observability, inboundhttp.CapabilityProvider{
			Provider: provider.Provider,
			Signals:  append([]string(nil), provider.Signals...),
		})
	}
	return clone
}

func providerNames(capabilities []inboundhttp.CapabilityProvider) []string {
	names := make([]string, 0, len(capabilities))
	for _, capability := range capabilities {
		names = append(names, capability.Provider)
	}
	return names
}
