package domain

import (
	"errors"
	"time"
)

// ErrProviderConfigNotFound indicates that the requested provider
// configuration does not exist in persistence.
var ErrProviderConfigNotFound = errors.New("provider configuration not found")

// ErrLLMConfigNotFound indicates that the requested LLM configuration does
// not exist in persistence.
var ErrLLMConfigNotFound = errors.New("llm configuration not found")

// ErrInvalidProviderConfig indicates that provider input failed validation.
var ErrInvalidProviderConfig = errors.New("invalid provider configuration")

// ErrInvalidLLMConfig indicates that LLM input failed validation.
var ErrInvalidLLMConfig = errors.New("invalid llm configuration")

// ErrProviderConfigConflict indicates a unique-constraint violation while
// persisting a provider or LLM configuration.
var ErrProviderConfigConflict = errors.New("provider configuration name already in use")

// ProviderConfig is the persisted definition of an observability provider
// adapter.
//
// Type is an opaque identifier whose accepted values are decided by the
// adapter registry; the core does not enumerate provider names. The use
// case validates Type against ports.ObservabilityProviderRegistry before
// persisting.
//
// CredentialsCiphertext stores the encrypted secret material (e.g. API
// token, basic-auth password) using the platform cipher. It is opaque to
// callers; the repository decrypts it just before constructing the
// adapter and never returns the plaintext through the HTTP boundary.
type ProviderConfig struct {
	ID                    string
	Type                  string
	Name                  string
	URL                   string
	Timeout               time.Duration
	Signals               []string
	Options               map[string]string
	CredentialsCiphertext string
	IsActive              bool
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// LLMConfig is the persisted definition of an LLM provider adapter.
//
// Type is the canonical identifier returned by
// ports.LLMProviderRegistry.Normalize; aliases are collapsed before
// persistence so a stored configuration always uses one stable name.
type LLMConfig struct {
	ID           string
	Type         string
	Name         string
	BaseURL      string
	Model        string
	Timeout      time.Duration
	APIKeyCipher string
	Options      map[string]string
	IsActive     bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
