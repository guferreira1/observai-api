// Package credentials exposes a CredentialStore dispatcher that resolves
// opaque references into secret values using a scheme-based lookup.
package credentials

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/guferreira1/observai-api/internal/core/ports"
)

// ErrUnknownScheme indicates that no resolver is registered for the supplied scheme.
var ErrUnknownScheme = errors.New("unknown credential reference scheme")

// ErrEmptyReference indicates that the caller supplied an empty reference.
var ErrEmptyReference = errors.New("empty credential reference")

// Resolver resolves a single scheme.
type Resolver interface {
	Resolve(ctx context.Context, value string) (string, error)
}

// Dispatcher routes credential references to the resolver responsible for
// the reference scheme. Schemes are lowercase identifiers separated from
// the value by a single colon (e.g. "env:VAR", "file:/path").
//
// A bare value without a scheme is resolved as a literal so existing call
// sites that already hold the secret in plaintext continue to work during
// migration.
type Dispatcher struct {
	resolvers map[string]Resolver
}

// NewDispatcher creates a dispatcher with the default env, file and literal
// resolvers pre-registered. Additional schemes can be added through
// Register before any resolution happens.
func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		resolvers: map[string]Resolver{
			"env":     EnvResolver{},
			"file":    NewFileResolver(),
			"literal": LiteralResolver{},
		},
	}
}

// LiteralResolver returns the supplied value verbatim. It is the safe
// channel for in-memory plaintext credentials that may contain colons
// (e.g. Dynatrace API tokens) and therefore cannot ride the bare-value
// shortcut.
type LiteralResolver struct{}

// Resolve implements Resolver.
func (LiteralResolver) Resolve(_ context.Context, value string) (string, error) {
	return value, nil
}

// Register associates a resolver with a scheme. Subsequent registrations of
// the same scheme overwrite the previous resolver, which lets tests inject
// fakes.
func (dispatcher *Dispatcher) Register(scheme string, resolver Resolver) {
	if dispatcher.resolvers == nil {
		dispatcher.resolvers = map[string]Resolver{}
	}
	dispatcher.resolvers[strings.ToLower(strings.TrimSpace(scheme))] = resolver
}

// Resolve satisfies ports.CredentialStore.
func (dispatcher *Dispatcher) Resolve(ctx context.Context, reference string) (string, error) {
	trimmed := strings.TrimSpace(reference)
	if trimmed == "" {
		return "", ErrEmptyReference
	}

	scheme, value, found := strings.Cut(trimmed, ":")
	if !found {
		return trimmed, nil
	}

	resolver, ok := dispatcher.resolvers[strings.ToLower(strings.TrimSpace(scheme))]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrUnknownScheme, scheme)
	}
	resolved, err := resolver.Resolve(ctx, strings.TrimSpace(value))
	if err != nil {
		return "", fmt.Errorf("resolve credential %s: %w", scheme, err)
	}
	return resolved, nil
}

// Compile-time check that Dispatcher satisfies the core port.
var _ ports.CredentialStore = (*Dispatcher)(nil)
