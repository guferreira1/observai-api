package main

import (
	"context"

	"github.com/guferreira1/observai-api/internal/platform/health"
)

// buildHealthProbes returns a probe per real dependency wired into the API.
//
// Adapters running in fake mode (no DSN/URL configured) are skipped so the
// readiness check stays accurate: it only fails when a configured real
// dependency is unavailable.
func buildHealthProbes(store analysisStore, cache analysisContextCache, deps providers) []health.Probe {
	probes := make([]health.Probe, 0, 4)

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

	return probes
}
