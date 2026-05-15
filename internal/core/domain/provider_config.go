package domain

import (
	"errors"
	"strings"
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

// ObservabilityProviderType enumerates the observability provider adapters
// the API can resolve at runtime.
type ObservabilityProviderType string

const (
	ProviderTypePrometheus    ObservabilityProviderType = "prometheus"
	ProviderTypeLoki          ObservabilityProviderType = "loki"
	ProviderTypeJaeger        ObservabilityProviderType = "jaeger"
	ProviderTypeElasticsearch ObservabilityProviderType = "elasticsearch"
	ProviderTypeOpenSearch    ObservabilityProviderType = "opensearch"
	ProviderTypeOTEL          ObservabilityProviderType = "otel"
	ProviderTypeTempo         ObservabilityProviderType = "tempo"
	ProviderTypeDynatrace     ObservabilityProviderType = "dynatrace"
	ProviderTypeDatadog       ObservabilityProviderType = "datadog"
	ProviderTypeNewRelic      ObservabilityProviderType = "newrelic"
)

// IsValidObservabilityProviderType reports whether the supplied type is a
// known observability adapter.
func IsValidObservabilityProviderType(value ObservabilityProviderType) bool {
	switch value {
	case ProviderTypePrometheus, ProviderTypeLoki, ProviderTypeJaeger,
		ProviderTypeElasticsearch, ProviderTypeOpenSearch, ProviderTypeOTEL,
		ProviderTypeTempo, ProviderTypeDynatrace, ProviderTypeDatadog,
		ProviderTypeNewRelic:
		return true
	}
	return false
}

// LLMProviderType enumerates the LLM provider adapters the API can resolve.
type LLMProviderType string

const (
	LLMProviderTypeOllama     LLMProviderType = "ollama"
	LLMProviderTypeOpenAI     LLMProviderType = "openai"
	LLMProviderTypeAnthropic  LLMProviderType = "anthropic"
	LLMProviderTypeAzure      LLMProviderType = "azure"
	LLMProviderTypeOpenRouter LLMProviderType = "openrouter"
)

// IsValidLLMProviderType reports whether the supplied type is a known LLM
// adapter.
func IsValidLLMProviderType(value LLMProviderType) bool {
	_, ok := NormalizeLLMProviderType(value)
	return ok
}

// NormalizeLLMProviderType converts accepted aliases to the canonical LLM
// provider type persisted by the core.
func NormalizeLLMProviderType(value LLMProviderType) (LLMProviderType, bool) {
	switch strings.ToLower(strings.TrimSpace(string(value))) {
	case string(LLMProviderTypeOllama):
		return LLMProviderTypeOllama, true
	case string(LLMProviderTypeOpenAI), "openai-compatible", "openai_compatible":
		return LLMProviderTypeOpenAI, true
	case string(LLMProviderTypeAnthropic):
		return LLMProviderTypeAnthropic, true
	case string(LLMProviderTypeAzure):
		return LLMProviderTypeAzure, true
	case string(LLMProviderTypeOpenRouter):
		return LLMProviderTypeOpenRouter, true
	}
	return "", false
}

// ProviderConfig is the persisted definition of an observability provider
// adapter.
//
// CredentialsCiphertext stores the encrypted secret material (e.g. API
// token, basic-auth password) using the platform cipher. It is opaque to
// callers; the repository decrypts it just before constructing the
// adapter and never returns the plaintext through the HTTP boundary.
type ProviderConfig struct {
	ID                    string
	Type                  ObservabilityProviderType
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
type LLMConfig struct {
	ID           string
	Type         LLMProviderType
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
