package factory

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/guferreira1/observai-api/internal/adapters/outbound/anthropic"
	"github.com/guferreira1/observai-api/internal/adapters/outbound/null"
	"github.com/guferreira1/observai-api/internal/adapters/outbound/ollama"
	"github.com/guferreira1/observai-api/internal/adapters/outbound/openai"
	"github.com/guferreira1/observai-api/internal/core/ports"
	"github.com/guferreira1/observai-api/internal/platform/config"
)

// LLMClients exposes typed clients built alongside the active LLM pair so
// health probes can call provider Ping methods.
type LLMClients struct {
	Ollama    *ollama.Client
	OpenAI    *openai.Client
	Anthropic *anthropic.Client
}

// LLMResult is the outcome of resolving the active LLM provider.
type LLMResult struct {
	Generator ports.AnalysisGenerator
	Responder ports.ChatResponder
	Provider  string
	Model     string
	Clients   LLMClients
}

type llmBuilder func(provider config.LLMProviderConfig, deps Dependencies, clients *LLMClients) (LLMResult, error)

var llmBuilders = map[string]llmBuilder{
	"ollama":            buildOllamaLLM,
	"openai":            buildOpenAILLM,
	"openai-compatible": buildOpenAILLM,
	"openai_compatible": buildOpenAILLM,
	"azure":             buildOpenAILLM,
	"openrouter":        buildOpenAILLM,
	"anthropic":         buildAnthropicLLM,
}

// BuildLLM resolves the active LLM provider and constructs its generator
// and chat responder. When no provider is configured the result is the
// null pair, which surfaces ErrProviderNotConfigured per call.
func BuildLLM(cfg config.Config, deps Dependencies) (LLMResult, error) {
	providers := cfg.LLM.Providers
	if len(providers) == 0 {
		deps.Logger.Warn("no llm provider configured; analysis and chat will return provider-not-configured")
		return LLMResult{
			Generator: null.NewAnalysisGenerator(),
			Responder: null.NewChatResponder(),
			Provider:  "none",
		}, nil
	}

	active, err := resolveActiveLLM(cfg.LLM)
	if err != nil {
		return LLMResult{}, err
	}

	builder, ok := llmBuilders[strings.ToLower(strings.TrimSpace(active.Type))]
	if !ok {
		return LLMResult{}, fmt.Errorf("unsupported llm provider type %q (supported: %s)", active.Type, supportedLLMTypes())
	}

	clients := LLMClients{}
	result, err := builder(active, deps, &clients)
	if err != nil {
		return LLMResult{}, fmt.Errorf("init llm provider %q: %w", llmProviderName(active), err)
	}
	result.Clients = clients
	deps.Logger.Info("llm provider enabled", "type", active.Type, "name", result.Provider, "model", result.Model)
	return result, nil
}

func resolveActiveLLM(cfg config.LLMConfig) (config.LLMProviderConfig, error) {
	active := strings.TrimSpace(cfg.Active)
	if active == "" {
		return cfg.Providers[0], nil
	}
	for _, provider := range cfg.Providers {
		if strings.EqualFold(strings.TrimSpace(provider.Name), active) {
			return provider, nil
		}
	}
	for _, provider := range cfg.Providers {
		if strings.EqualFold(strings.TrimSpace(provider.Type), active) {
			return provider, nil
		}
	}
	return config.LLMProviderConfig{}, fmt.Errorf("llm.active %q does not match any configured provider name or type", cfg.Active)
}

func buildOllamaLLM(provider config.LLMProviderConfig, deps Dependencies, clients *LLMClients) (LLMResult, error) {
	url := strings.TrimSpace(provider.URL)
	if url == "" {
		return LLMResult{}, fmt.Errorf("ollama provider requires a non-empty url")
	}

	client, err := ollama.NewClient(ollama.ClientOptions{
		BaseURL:  url,
		Model:    provider.Model,
		Timeout:  defaultTimeout(provider.Timeout, 30*time.Second),
		Observer: deps.Observer,
	})
	if err != nil {
		return LLMResult{}, err
	}

	clients.Ollama = client
	return LLMResult{
		Generator: ollama.NewAnalysisGenerator(client, deps.PromptLoader),
		Responder: ollama.NewChatResponder(client, deps.PromptLoader),
		Provider:  llmProviderName(provider),
		Model:     provider.Model,
	}, nil
}

func buildOpenAILLM(provider config.LLMProviderConfig, deps Dependencies, clients *LLMClients) (LLMResult, error) {
	baseURL := strings.TrimSpace(provider.BaseURL)
	if baseURL == "" {
		baseURL = strings.TrimSpace(provider.URL)
	}

	apiKey, authOptional, err := resolveOpenAIAPIKey(provider, deps)
	if err != nil {
		return LLMResult{}, err
	}

	client, err := openai.NewClient(openai.ClientOptions{
		BaseURL:          baseURL,
		APIKey:           apiKey,
		AllowEmptyAPIKey: authOptional,
		Model:            provider.Model,
		Timeout:          defaultTimeout(provider.Timeout, 60*time.Second),
		Observer:         deps.Observer,
	})
	if err != nil {
		return LLMResult{}, err
	}

	clients.OpenAI = client
	return LLMResult{
		Generator: openai.NewAnalysisGenerator(client, deps.PromptLoader),
		Responder: openai.NewChatResponder(client, deps.PromptLoader),
		Provider:  llmProviderName(provider),
		Model:     provider.Model,
	}, nil
}

func resolveOpenAIAPIKey(provider config.LLMProviderConfig, deps Dependencies) (string, bool, error) {
	apiKeyReference := credentialReference(provider.APIKeyEnv)
	authOptional := openAIAuthOptional(provider)
	if apiKeyReference == "" {
		if authOptional {
			return "", true, nil
		}
		return "", false, fmt.Errorf("openai-compatible provider requires api_key_env unless options.auth is optional")
	}

	apiKey, err := resolveCredential(context.Background(), deps.Credentials, apiKeyReference)
	if err != nil {
		return "", false, fmt.Errorf("resolve openai api key: %w", err)
	}
	return apiKey, authOptional, nil
}

func credentialReference(reference string) string {
	cleaned := strings.TrimSpace(reference)
	if cleaned == "" || strings.Contains(cleaned, ":") {
		return cleaned
	}
	return "env:" + cleaned
}

var openAIOptionalAuthOptions = map[string]string{
	"auth":          "optional",
	"authOptional":  "true",
	"auth_optional": "true",
}

func openAIAuthOptional(provider config.LLMProviderConfig) bool {
	for key, expected := range openAIOptionalAuthOptions {
		if strings.EqualFold(strings.TrimSpace(provider.Options[key]), expected) {
			return true
		}
	}
	return false
}

func buildAnthropicLLM(provider config.LLMProviderConfig, deps Dependencies, clients *LLMClients) (LLMResult, error) {
	apiKeyReference := credentialReference(provider.APIKeyEnv)
	if apiKeyReference == "" {
		return LLMResult{}, fmt.Errorf("anthropic provider requires api_key_env (e.g. env:OBSERVAI_ANTHROPIC_API_KEY)")
	}

	apiKey, err := resolveCredential(context.Background(), deps.Credentials, apiKeyReference)
	if err != nil {
		return LLMResult{}, fmt.Errorf("resolve anthropic api key: %w", err)
	}

	baseURL := strings.TrimSpace(provider.BaseURL)
	if baseURL == "" {
		baseURL = strings.TrimSpace(provider.URL)
	}

	client, err := anthropic.NewClient(anthropic.ClientOptions{
		BaseURL:  baseURL,
		APIKey:   apiKey,
		Model:    provider.Model,
		Timeout:  defaultTimeout(provider.Timeout, 60*time.Second),
		Observer: deps.Observer,
	})
	if err != nil {
		return LLMResult{}, err
	}

	clients.Anthropic = client
	return LLMResult{
		Generator: anthropic.NewAnalysisGenerator(client, deps.PromptLoader),
		Responder: anthropic.NewChatResponder(client, deps.PromptLoader),
		Provider:  llmProviderName(provider),
		Model:     provider.Model,
	}, nil
}

func supportedLLMTypes() string {
	types := make([]string, 0, len(llmBuilders))
	for providerType := range llmBuilders {
		types = append(types, providerType)
	}
	return strings.Join(types, ", ")
}

func llmProviderName(provider config.LLMProviderConfig) string {
	name := strings.TrimSpace(provider.Name)
	if name == "" {
		return strings.TrimSpace(provider.Type)
	}
	return name
}
