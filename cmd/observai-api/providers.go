package main

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/guferreira1/observai-api/internal/adapters/outbound/null"
	"github.com/guferreira1/observai-api/internal/adapters/outbound/ollama"
	prometheusadapter "github.com/guferreira1/observai-api/internal/adapters/outbound/prometheus"
	"github.com/guferreira1/observai-api/internal/adapters/outbound/prompts"
	"github.com/guferreira1/observai-api/internal/core/ports"
	"github.com/guferreira1/observai-api/internal/platform/config"
	"github.com/guferreira1/observai-api/internal/platform/observability"
)

// providers groups the runtime providers used by the analysis and chat use cases.
type providers struct {
	collector         ports.SignalCollector
	generator         ports.AnalysisGenerator
	responder         ports.ChatResponder
	prometheus        *prometheusadapter.Client
	ollama            *ollama.Client
	llmProvider       string
	llmModel          string
	observabilityList []observabilityProvider
}

type observabilityProvider struct {
	name    string
	signals []string
}

// newProviders selects real or null providers based on cfg.Mode.
//
// In ModeProd, missing configuration must already have been rejected by
// config.Validate(), so this function fails hard if construction errors occur.
// In ModeLocal/ModeDev, missing endpoints fall back to the null adapters,
// which return domain.ErrProviderNotConfigured at request time instead of
// synthesizing observability data.
func newProviders(cfg config.Config, log *slog.Logger, observer observability.ProviderObserver) (providers, error) {
	loader := prompts.NewFileLoader(cfg.Prompts.Dir)

	collector, prometheusClient, observabilityList, err := buildSignalCollector(cfg, log, observer)
	if err != nil {
		return providers{}, err
	}

	generator, responder, ollamaClient, llmName, llmModel, err := buildLLMProviders(cfg, loader, log, observer)
	if err != nil {
		return providers{}, err
	}

	return providers{
		collector:         collector,
		generator:         generator,
		responder:         responder,
		prometheus:        prometheusClient,
		ollama:            ollamaClient,
		llmProvider:       llmName,
		llmModel:          llmModel,
		observabilityList: observabilityList,
	}, nil
}

func buildSignalCollector(cfg config.Config, log *slog.Logger, observer observability.ProviderObserver) (ports.SignalCollector, *prometheusadapter.Client, []observabilityProvider, error) {
	prometheusURL := strings.TrimSpace(cfg.Prometheus.URL)
	if prometheusURL == "" {
		if cfg.Mode == config.ModeProd {
			return nil, nil, nil, fmt.Errorf("prometheus url is required in mode=prod")
		}
		log.Warn("prometheus signal collector disabled; signal collection endpoint will return provider-not-configured")
		return null.NewSignalCollector(), nil, []observabilityProvider{{
			name:    "none",
			signals: []string{},
		}}, nil
	}

	client, err := prometheusadapter.NewClient(prometheusadapter.ClientOptions{
		BaseURL:  prometheusURL,
		Timeout:  cfg.Prometheus.Timeout,
		Observer: observer,
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("init prometheus client: %w", err)
	}

	log.Info("prometheus signal collector enabled", "url", prometheusURL)
	return prometheusadapter.NewSignalCollector(client, prometheusadapter.SignalCollectorOptions{}), client, []observabilityProvider{{
		name:    "prometheus",
		signals: []string{"metrics"},
	}}, nil
}

func buildLLMProviders(cfg config.Config, loader prompts.Loader, log *slog.Logger, observer observability.ProviderObserver) (ports.AnalysisGenerator, ports.ChatResponder, *ollama.Client, string, string, error) {
	ollamaURL := strings.TrimSpace(cfg.Ollama.URL)
	if ollamaURL == "" {
		if cfg.Mode == config.ModeProd {
			return nil, nil, nil, "", "", fmt.Errorf("ollama url is required in mode=prod")
		}
		log.Warn("ollama llm disabled; analysis and chat endpoints will return provider-not-configured")
		return null.NewAnalysisGenerator(), null.NewChatResponder(), nil, "none", "", nil
	}

	client, err := ollama.NewClient(ollama.ClientOptions{
		BaseURL:  ollamaURL,
		Model:    cfg.Ollama.Model,
		Timeout:  cfg.Ollama.Timeout,
		Observer: observer,
	})
	if err != nil {
		return nil, nil, nil, "", "", fmt.Errorf("init ollama client: %w", err)
	}

	log.Info("ollama llm enabled", "url", ollamaURL, "model", cfg.Ollama.Model)
	return ollama.NewAnalysisGenerator(client, loader), ollama.NewChatResponder(client, loader), client, "ollama", cfg.Ollama.Model, nil
}
