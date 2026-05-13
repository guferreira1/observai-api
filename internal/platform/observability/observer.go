// Package observability defines small, dependency-free contracts for runtime
// instrumentation. Adapters depend on these contracts so the core and outbound
// packages stay free of metrics-library imports.
package observability

import "time"

// ProviderObserver records the latency and outcome of outbound provider calls.
type ProviderObserver interface {
	Observe(provider string, operation string, duration time.Duration, err error)
}

// NoopProviderObserver discards every observation. Useful for tests and local mode.
type NoopProviderObserver struct{}

// Observe satisfies ProviderObserver and does nothing.
func (NoopProviderObserver) Observe(string, string, time.Duration, error) {}
