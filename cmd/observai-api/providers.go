package main

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/guferreira1/observai-api/internal/adapters/outbound/fake"
	"github.com/guferreira1/observai-api/internal/adapters/outbound/ollama"
	prometheusadapter "github.com/guferreira1/observai-api/internal/adapters/outbound/prometheus"
	"github.com/guferreira1/observai-api/internal/adapters/outbound/prompts"
	"github.com/guferreira1/observai-api/internal/core/ports"
	"github.com/guferreira1/observai-api/internal/platform/config"
	"github.com/guferreira1/observai-api/internal/platform/observability"
)

// providers groups the runtime providers used by the analysis and chat use cases.
type providers struct {
	collector  ports.SignalCollector
	generator  ports.AnalysisGenerator
	responder  ports.ChatResponder
	prometheus *prometheusadapter.Client
	ollama     *ollama.Client
}

// newProviders selects real or fake providers based on cfg.Mode.
//
// In ModeProd, missing configuration must already have been rejected by
// config.Validate(), so this function fails hard if construction errors occur.
// In ModeLocal/ModeDev, missing endpoints fall back to the deterministic fakes.
func newProviders(cfg config.Config, log *slog.Logger, observer observability.ProviderObserver) (providers, error) {
	loader := prompts.NewFileLoader(cfg.Prompts.Dir)

	collector, prometheusClient, err := buildSignalCollector(cfg, log, observer)
	if err != nil {
		return providers{}, err
	}

	generator, responder, ollamaClient, err := buildLLMProviders(cfg, loader, log, observer)
	if err != nil {
		return providers{}, err
	}

	return providers{
		collector:  collector,
		generator:  generator,
		responder:  responder,
		prometheus: prometheusClient,
		ollama:     ollamaClient,
	}, nil
}

func buildSignalCollector(cfg config.Config, log *slog.Logger, observer observability.ProviderObserver) (ports.SignalCollector, *prometheusadapter.Client, error) {
	prometheusURL := strings.TrimSpace(cfg.Prometheus.URL)
	if prometheusURL == "" {
		if cfg.Mode == config.ModeProd {
			return nil, nil, fmt.Errorf("prometheus url is required in mode=prod")
		}
		log.Warn("prometheus signal collector disabled; using deterministic fake")
		return fake.NewSignalCollector(), nil, nil
	}

	client, err := prometheusadapter.NewClient(prometheusadapter.ClientOptions{
		BaseURL:  prometheusURL,
		Timeout:  cfg.Prometheus.Timeout,
		Observer: observer,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("init prometheus client: %w", err)
	}

	log.Info("prometheus signal collector enabled", "url", prometheusURL)
	return prometheusadapter.NewSignalCollector(client, prometheusadapter.SignalCollectorOptions{}), client, nil
}

func buildLLMProviders(cfg config.Config, loader prompts.Loader, log *slog.Logger, observer observability.ProviderObserver) (ports.AnalysisGenerator, ports.ChatResponder, *ollama.Client, error) {
	ollamaURL := strings.TrimSpace(cfg.Ollama.URL)
	if ollamaURL == "" {
		if cfg.Mode == config.ModeProd {
			return nil, nil, nil, fmt.Errorf("ollama url is required in mode=prod")
		}
		log.Warn("ollama llm disabled; using deterministic fake analysis generator and chat responder")
		return fake.NewAnalysisGenerator(), fake.NewChatResponder(), nil, nil
	}

	client, err := ollama.NewClient(ollama.ClientOptions{
		BaseURL:  ollamaURL,
		Model:    cfg.Ollama.Model,
		Timeout:  cfg.Ollama.Timeout,
		Observer: observer,
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("init ollama client: %w", err)
	}

	log.Info("ollama llm enabled", "url", ollamaURL, "model", cfg.Ollama.Model)
	return ollama.NewAnalysisGenerator(client, loader), ollama.NewChatResponder(client, loader), client, nil
}
