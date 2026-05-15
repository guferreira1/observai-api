package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/guferreira1/observai-api/internal/core/usecase"
	"github.com/guferreira1/observai-api/internal/platform/health"
)

// buildHealthProbes returns a probe per real dependency wired into the API.
//
// Adapters running in fake mode (no DSN/URL configured) are skipped so the
// readiness check stays accurate: it only fails when a configured real
// dependency is unavailable.
func buildHealthProbes(store analysisStore, cache analysisContextCache, deps providers, providerConfigs *usecase.ProviderConfig, llmConfigs *usecase.LLMConfig) []health.Probe {
	probes := make([]health.Probe, 0, 6)

	if store.postgres != nil {
		postgres := store.postgres
		probes = append(probes, health.ProbeFunc{
			ProbeName: "postgres",
			Fn: func(ctx context.Context) error {
				return postgres.Ping(ctx)
			},
		})
	}

	if cache.redis != nil {
		redis := cache.redis
		probes = append(probes, health.ProbeFunc{
			ProbeName: "redis",
			Fn: func(ctx context.Context) error {
				return redis.Ping(ctx)
			},
		})
	}

	if deps.prometheus != nil {
		prometheusClient := deps.prometheus
		probes = append(probes, health.ProbeFunc{
			ProbeName: "prometheus",
			Fn: func(ctx context.Context) error {
				return prometheusClient.Ping(ctx)
			},
		})
	}

	if deps.ollama != nil {
		ollamaClient := deps.ollama
		probes = append(probes, health.ProbeFunc{
			ProbeName: "ollama",
			Fn: func(ctx context.Context) error {
				return ollamaClient.Ping(ctx)
			},
		})
	}

	if providerConfigs != nil {
		probes = append(probes, health.ProbeFunc{
			ProbeName: "providers_active",
			Fn: func(ctx context.Context) error {
				return checkActiveProviders(ctx, providerConfigs)
			},
		})
	}

	if llmConfigs != nil {
		probes = append(probes, health.ProbeFunc{
			ProbeName: "llm_active",
			Fn: func(ctx context.Context) error {
				return checkActiveLLM(ctx, llmConfigs)
			},
		})
	}

	return probes
}

func checkActiveProviders(ctx context.Context, providerConfigs *usecase.ProviderConfig) error {
	configs, err := providerConfigs.ListActiveConfigs(ctx)
	if err != nil {
		return fmt.Errorf("list active providers: %w", err)
	}
	var failures []string
	for _, config := range configs {
		result, err := providerConfigs.Test(ctx, config.ID)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %s", config.Name, err.Error()))
			continue
		}
		if !result.Reached {
			failures = append(failures, fmt.Sprintf("%s: %s", config.Name, result.Error))
		}
	}
	if len(failures) > 0 {
		return errors.New("unreachable providers: " + joinStrings(failures, "; "))
	}
	return nil
}

func checkActiveLLM(ctx context.Context, llmConfigs *usecase.LLMConfig) error {
	config, err := llmConfigs.FindActiveConfig(ctx)
	if err != nil {
		return nil
	}
	result, err := llmConfigs.Test(ctx, config.ID)
	if err != nil {
		return fmt.Errorf("test llm %s: %w", config.Name, err)
	}
	if !result.Reached {
		return fmt.Errorf("llm %s unreachable: %s", config.Name, result.Error)
	}
	return nil
}

func joinStrings(values []string, separator string) string {
	if len(values) == 0 {
		return ""
	}
	out := values[0]
	for _, value := range values[1:] {
		out += separator + value
	}
	return out
}
