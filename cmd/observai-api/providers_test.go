package main

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/guferreira1/observai-api/internal/adapters/outbound/factory"
	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/core/ports"
	"github.com/guferreira1/observai-api/internal/core/usecase"
	"github.com/guferreira1/observai-api/internal/platform/config"
)

func TestInitialRuntimeProviderConfigLoadsActiveDatabaseProviders(t *testing.T) {
	providerUseCase := usecase.NewProviderConfig(&stubProviderConfigRepository{
		active: []domain.ProviderConfig{
			{
				ID:       "prometheus",
				Type:     factory.ProviderTypePrometheus,
				Name:     "Prometheus",
				URL:      "http://prometheus:9090",
				Signals:  []string{"metrics"},
				IsActive: true,
			},
			{
				ID:       "jaeger",
				Type:     factory.ProviderTypeJaeger,
				Name:     "Jaeger",
				URL:      "http://jaeger:16686",
				Signals:  []string{"traces"},
				IsActive: true,
			},
		},
	}, nil, nil, factory.NewObservabilityRegistry(), nil)
	llmUseCase := usecase.NewLLMConfig(&stubLLMConfigRepository{
		active: &domain.LLMConfig{
			ID:       "openai",
			Type:     factory.LLMProviderTypeOpenAI,
			Name:     "ChatGPT",
			BaseURL:  "https://api.openai.com/v1",
			Model:    "gpt-4o-mini",
			IsActive: true,
		},
	}, nil, nil, factory.NewLLMRegistry(), nil)

	runtimeConfig, loaded := initialRuntimeProviderConfig(context.Background(), config.Config{}, discardLogger(), providerUseCase, llmUseCase)

	if !loaded {
		t.Fatalf("expected active database providers to be loaded")
	}
	if providerCount := len(runtimeConfig.Observability.Providers); providerCount != 2 {
		t.Fatalf("expected 2 observability providers, got %d", providerCount)
	}
	if runtimeConfig.Observability.Providers[0].Name != "Prometheus" {
		t.Fatalf("expected first provider to be Prometheus, got %q", runtimeConfig.Observability.Providers[0].Name)
	}
	if runtimeConfig.Observability.Providers[1].Type != factory.ProviderTypeJaeger {
		t.Fatalf("expected second provider to be jaeger, got %q", runtimeConfig.Observability.Providers[1].Type)
	}
	if providerCount := len(runtimeConfig.LLM.Providers); providerCount != 1 {
		t.Fatalf("expected 1 llm provider, got %d", providerCount)
	}
	if runtimeConfig.LLM.Active != "ChatGPT" {
		t.Fatalf("expected active llm to be ChatGPT, got %q", runtimeConfig.LLM.Active)
	}
	if runtimeConfig.LLM.Providers[0].Model != "gpt-4o-mini" {
		t.Fatalf("expected llm model to be gpt-4o-mini, got %q", runtimeConfig.LLM.Providers[0].Model)
	}
}

func TestInitialRuntimeProviderConfigKeepsBootstrapConfigWithoutActiveDatabaseProviders(t *testing.T) {
	bootstrapConfig := config.Config{
		Observability: config.ObservabilityConfig{
			Providers: []config.ObservabilityProviderConfig{
				{
					Type: "prometheus",
					Name: "bootstrap-prometheus",
					URL:  "http://localhost:9090",
				},
			},
		},
	}
	providerUseCase := usecase.NewProviderConfig(&stubProviderConfigRepository{}, nil, nil, factory.NewObservabilityRegistry(), nil)
	llmUseCase := usecase.NewLLMConfig(&stubLLMConfigRepository{}, nil, nil, factory.NewLLMRegistry(), nil)

	runtimeConfig, loaded := initialRuntimeProviderConfig(context.Background(), bootstrapConfig, discardLogger(), providerUseCase, llmUseCase)

	if loaded {
		t.Fatalf("expected no active database provider to be loaded")
	}
	if runtimeConfig.Observability.Providers[0].Name != "bootstrap-prometheus" {
		t.Fatalf("expected bootstrap provider to remain configured, got %q", runtimeConfig.Observability.Providers[0].Name)
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type stubProviderConfigRepository struct {
	active []domain.ProviderConfig
	err    error
}

func (repository *stubProviderConfigRepository) Create(context.Context, domain.ProviderConfig) error {
	return nil
}

func (repository *stubProviderConfigRepository) Find(context.Context, string) (domain.ProviderConfig, error) {
	return domain.ProviderConfig{}, domain.ErrProviderConfigNotFound
}

func (repository *stubProviderConfigRepository) List(context.Context, int, int) ([]domain.ProviderConfig, error) {
	return nil, nil
}

func (repository *stubProviderConfigRepository) ListActive(context.Context) ([]domain.ProviderConfig, error) {
	if repository.err != nil {
		return nil, repository.err
	}
	return append([]domain.ProviderConfig(nil), repository.active...), nil
}

func (repository *stubProviderConfigRepository) Count(context.Context) (int64, error) {
	return int64(len(repository.active)), nil
}

func (repository *stubProviderConfigRepository) Update(context.Context, domain.ProviderConfig) error {
	return nil
}

func (repository *stubProviderConfigRepository) SetActive(context.Context, string, bool, time.Time) error {
	return nil
}

func (repository *stubProviderConfigRepository) Delete(context.Context, string) error {
	return nil
}

type stubLLMConfigRepository struct {
	active *domain.LLMConfig
	err    error
}

func (repository *stubLLMConfigRepository) Create(context.Context, domain.LLMConfig) error {
	return nil
}

func (repository *stubLLMConfigRepository) Find(context.Context, string) (domain.LLMConfig, error) {
	return domain.LLMConfig{}, domain.ErrLLMConfigNotFound
}

func (repository *stubLLMConfigRepository) List(context.Context, int, int) ([]domain.LLMConfig, error) {
	return nil, nil
}

func (repository *stubLLMConfigRepository) FindActive(context.Context) (domain.LLMConfig, error) {
	if repository.err != nil {
		return domain.LLMConfig{}, repository.err
	}
	if repository.active == nil {
		return domain.LLMConfig{}, domain.ErrLLMConfigNotFound
	}
	return *repository.active, nil
}

func (repository *stubLLMConfigRepository) Count(context.Context) (int64, error) {
	if repository.active == nil {
		return 0, nil
	}
	return 1, nil
}

func (repository *stubLLMConfigRepository) Update(context.Context, domain.LLMConfig) error {
	return nil
}

func (repository *stubLLMConfigRepository) Activate(context.Context, string, time.Time) error {
	return nil
}

func (repository *stubLLMConfigRepository) Delete(context.Context, string) error {
	return nil
}

var (
	_ ports.ProviderConfigRepository = (*stubProviderConfigRepository)(nil)
	_ ports.LLMConfigRepository      = (*stubLLMConfigRepository)(nil)
)
