// Package factory wires concrete outbound adapters from the runtime
// configuration using a small dispatcher map keyed by provider type.
//
// The composition root (cmd/observai-api) calls Build* helpers exposed here
// instead of selecting providers with conditionals. New observability or
// LLM adapters register themselves by adding a builder to the dispatcher
// map; the use cases keep depending only on ports.
package factory

import (
	"context"
	"log/slog"
	"time"

	"github.com/guferreira1/observai-api/internal/adapters/outbound/prompts"
	"github.com/guferreira1/observai-api/internal/core/ports"
	"github.com/guferreira1/observai-api/internal/platform/observability"
)

// Dependencies holds collaborators shared by every builder.
//
// Logger is used for non-fatal warnings such as missing optional fields.
// Observer is forwarded to provider clients so they emit consistent
// latency / error metrics. PromptLoader is used by LLM builders.
// Credentials resolves opaque references (env:VAR, file:/path) into the
// actual secret value when a provider requires authentication.
type Dependencies struct {
	Logger       *slog.Logger
	Observer     observability.ProviderObserver
	PromptLoader prompts.Loader
	Credentials  ports.CredentialStore
}

// resolveCredential is a small helper that returns the literal value when
// no CredentialStore is wired (tests, local mode without secrets) and
// dispatches through the store otherwise.
func resolveCredential(ctx context.Context, store ports.CredentialStore, reference string) (string, error) {
	if store == nil {
		return reference, nil
	}
	return store.Resolve(ctx, reference)
}

// ProviderCapability is a non-sensitive description of a wired provider.
//
// The HTTP capabilities endpoint exposes this list so frontends can render
// which observability providers are reachable and which signals each one
// supports without ever touching credentials.
type ProviderCapability struct {
	Name    string
	Type    string
	Signals []string
}

// defaultTimeout returns the timeout to apply when a provider configuration
// did not specify one.
func defaultTimeout(value time.Duration, fallback time.Duration) time.Duration {
	if value <= 0 {
		return fallback
	}
	return value
}
