package ports

// ProviderInventorySnapshot captures, at a point in time, how many
// observability and LLM providers the running instance has configured.
//
// W4 swaps the snapshot source from configuration to the
// provider_configurations / llm_configurations tables without changing the
// Setup use case contract.
type ProviderInventorySnapshot struct {
	ObservabilityProviders int
	LLMProviders           int
}

// ProviderInventory reports the current provider counts available to the
// instance. Implementations must be safe for concurrent use.
type ProviderInventory interface {
	Snapshot() ProviderInventorySnapshot
}

// ProviderInventoryFunc adapts a plain function to the ProviderInventory
// interface.
type ProviderInventoryFunc func() ProviderInventorySnapshot

// Snapshot returns the inventory snapshot reported by the underlying function.
func (fn ProviderInventoryFunc) Snapshot() ProviderInventorySnapshot { return fn() }
