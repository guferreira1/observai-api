package ports

// ObservabilityProviderRegistry is the source of truth for which
// observability provider types the running build supports.
//
// Adapters owning the collector and trace dispatcher maps implement the
// interface so the use case can validate operator input without
// importing adapter packages and without the core listing provider
// names directly.
type ObservabilityProviderRegistry interface {
	IsSupported(providerType string) bool
	SupportedTypes() []string
}

// LLMProviderRegistry mirrors ObservabilityProviderRegistry for LLM
// adapters.
//
// Normalize collapses accepted aliases (for example "openai-compatible"
// to "openai") into the canonical builder key the persistence layer
// will store.
type LLMProviderRegistry interface {
	IsSupported(providerType string) bool
	Normalize(providerType string) (canonical string, ok bool)
	SupportedTypes() []string
}
