package main

import (
	inboundhttp "github.com/guferreira1/observai-api/internal/adapters/inbound/http"
	"github.com/guferreira1/observai-api/internal/platform/config"
)

// buildCapabilities assembles the non-sensitive runtime descriptor exposed by GET /v1/capabilities.
//
// It only echoes information that is safe to expose: deployment mode, provider names,
// configured LLM model, request limits and the build version. Secrets, URLs and
// tokens never appear here.
func buildCapabilities(cfg config.Config, runtime providers, version string) inboundhttp.CapabilitiesResponse {
	observability := make([]inboundhttp.CapabilityProvider, 0, len(runtime.observabilityList))
	for _, item := range runtime.observabilityList {
		observability = append(observability, inboundhttp.CapabilityProvider{
			Provider: item.name,
			Signals:  item.signals,
		})
	}

	return inboundhttp.CapabilitiesResponse{
		Mode:    string(cfg.Mode),
		Version: version,
		LLM: inboundhttp.CapabilityLLM{
			Provider: runtime.llmProvider,
			Model:    runtime.llmModel,
		},
		Observability: observability,
		Limits: inboundhttp.CapabilityLimits{
			HTTPRequestTimeoutMs: cfg.HTTPRequestTimeout.Milliseconds(),
			HTTPMaxBodyBytes:     cfg.HTTPMaxBodyBytes,
			RateLimitRPS:         cfg.HTTPRateLimit.RequestsPerSecond,
			RateLimitBurst:       cfg.HTTPRateLimit.Burst,
		},
	}
}

func providerNames(capabilities []inboundhttp.CapabilityProvider) []string {
	names := make([]string, 0, len(capabilities))
	for _, capability := range capabilities {
		names = append(names, capability.Provider)
	}
	return names
}
