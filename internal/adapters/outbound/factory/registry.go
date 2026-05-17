package factory

import (
	"sort"
	"strings"
)

// Observability provider identifiers exposed by adapters that this build
// ships. They live in the adapter layer so the core domain stays
// provider-agnostic; the dispatcher maps in observability.go and trace.go
// are the actual source of truth.
const (
	ProviderTypePrometheus    = "prometheus"
	ProviderTypeLoki          = "loki"
	ProviderTypeJaeger        = "jaeger"
	ProviderTypeElasticsearch = "elasticsearch"
	ProviderTypeOpenSearch    = "opensearch"
	ProviderTypeOTEL          = "otel"
	ProviderTypeTempo         = "tempo"
	ProviderTypeDynatrace     = "dynatrace"
	ProviderTypeDatadog       = "datadog"
	ProviderTypeNewRelic      = "newrelic"
)

// LLM provider identifiers exposed by adapters that this build ships.
const (
	LLMProviderTypeOllama     = "ollama"
	LLMProviderTypeOpenAI     = "openai"
	LLMProviderTypeAnthropic  = "anthropic"
	LLMProviderTypeAzure      = "azure"
	LLMProviderTypeOpenRouter = "openrouter"
)

// llmCanonicalAliases maps accepted alternative spellings to the
// canonical LLM provider key so the registry returns a single form
// the persistence layer can store.
var llmCanonicalAliases = map[string]string{
	"openai-compatible": LLMProviderTypeOpenAI,
	"openai_compatible": LLMProviderTypeOpenAI,
}

// ObservabilityRegistry implements ports.ObservabilityProviderRegistry by
// querying the collector and trace dispatcher maps. The empty struct
// keeps the receiver allocation-free.
type ObservabilityRegistry struct{}

// NewObservabilityRegistry returns the registry backed by this build's
// collector and trace dispatcher maps.
func NewObservabilityRegistry() ObservabilityRegistry {
	return ObservabilityRegistry{}
}

// IsSupported reports whether the supplied type is registered as either
// a signal collector or a trace-only provider.
func (ObservabilityRegistry) IsSupported(providerType string) bool {
	cleaned := strings.ToLower(strings.TrimSpace(providerType))
	if cleaned == "" {
		return false
	}
	if _, ok := collectorBuilders[cleaned]; ok {
		return true
	}
	_, ok := traceBuilders[cleaned]
	return ok
}

// SupportedTypes returns the union of collector and trace types in
// stable alphabetical order.
func (ObservabilityRegistry) SupportedTypes() []string {
	seen := make(map[string]struct{}, len(collectorBuilders)+len(traceBuilders))
	for key := range collectorBuilders {
		seen[key] = struct{}{}
	}
	for key := range traceBuilders {
		seen[key] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for key := range seen {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// LLMRegistry implements ports.LLMProviderRegistry by querying the LLM
// builder map and the alias table.
type LLMRegistry struct{}

// NewLLMRegistry returns the registry backed by this build's LLM
// builder map.
func NewLLMRegistry() LLMRegistry {
	return LLMRegistry{}
}

// IsSupported reports whether the supplied type resolves to a known LLM
// builder, accepting registered aliases.
func (registry LLMRegistry) IsSupported(providerType string) bool {
	_, ok := registry.Normalize(providerType)
	return ok
}

// Normalize collapses accepted aliases into the canonical key the
// persistence layer stores. Unknown inputs return ("", false).
func (LLMRegistry) Normalize(providerType string) (string, bool) {
	cleaned := strings.ToLower(strings.TrimSpace(providerType))
	if cleaned == "" {
		return "", false
	}
	if canonical, ok := llmCanonicalAliases[cleaned]; ok {
		cleaned = canonical
	}
	if _, ok := llmBuilders[cleaned]; ok {
		return cleaned, true
	}
	return "", false
}

// SupportedTypes returns the canonical LLM types in alphabetical order.
// Aliases are intentionally excluded so callers do not advertise them as
// distinct providers.
func (LLMRegistry) SupportedTypes() []string {
	canonical := map[string]struct{}{
		LLMProviderTypeOllama:     {},
		LLMProviderTypeOpenAI:     {},
		LLMProviderTypeAnthropic:  {},
		LLMProviderTypeAzure:      {},
		LLMProviderTypeOpenRouter: {},
	}
	out := make([]string, 0, len(canonical))
	for key := range canonical {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
