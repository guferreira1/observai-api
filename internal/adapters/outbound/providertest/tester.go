// Package providertest exposes a lightweight HTTP-only ProviderTester used
// by the admin "test connection" endpoints.
//
// The tester intentionally avoids constructing full adapters: it issues a
// single HTTP request to a known liveness path per provider type and
// classifies the outcome based on the response. Provider SDK behaviour
// (pagination, retries, response shaping) is unnecessary for verifying
// that the operator-supplied credentials and URL are reachable.
package providertest

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	inboundhttp "github.com/guferreira1/observai-api/internal/adapters/inbound/http"
	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/core/ports"
)

const defaultProbeTimeout = 5 * time.Second

type observabilityProbe struct {
	path string
	auth func(*http.Request, string)
}

type llmProbe struct {
	method string
	path   string
	auth   func(*http.Request, string)
}

var observabilityProbes = map[domain.ObservabilityProviderType]observabilityProbe{
	domain.ProviderTypePrometheus:    {path: "/api/v1/query?query=up", auth: noAuth},
	domain.ProviderTypeLoki:          {path: "/loki/api/v1/labels", auth: basicAuth},
	domain.ProviderTypeJaeger:        {path: "/api/services", auth: noAuth},
	domain.ProviderTypeElasticsearch: {path: "/", auth: basicAuth},
	domain.ProviderTypeOpenSearch:    {path: "/", auth: basicAuth},
	domain.ProviderTypeOTEL:          {path: "/", auth: bearerAuth},
	domain.ProviderTypeTempo:         {path: "/ready", auth: noAuth},
	domain.ProviderTypeDynatrace:     {path: "/api/v2/clusterversion", auth: dynatraceAuth},
	domain.ProviderTypeDatadog:       {path: "/api/v1/validate", auth: datadogAuth},
	domain.ProviderTypeNewRelic:      {path: "/graphql", auth: newRelicAuth},
}

var llmProbes = map[domain.LLMProviderType]llmProbe{
	domain.LLMProviderTypeOllama:     {method: http.MethodGet, path: "/api/tags", auth: noAuth},
	domain.LLMProviderTypeOpenAI:     {method: http.MethodGet, path: "/v1/models", auth: bearerAuth},
	domain.LLMProviderTypeAnthropic:  {method: http.MethodGet, path: "/v1/models", auth: anthropicAuth},
	domain.LLMProviderTypeAzure:      {method: http.MethodGet, path: "/openai/models?api-version=2024-02-01", auth: azureAuth},
	domain.LLMProviderTypeOpenRouter: {method: http.MethodGet, path: "/v1/models", auth: bearerAuth},
}

// Tester implements ports.ProviderTester over net/http.
type Tester struct {
	client *http.Client
}

// New returns a Tester with sensible defaults.
func New() *Tester {
	return &Tester{client: &http.Client{Timeout: defaultProbeTimeout}}
}

// TestObservability runs the per-type probe against an observability
// provider configuration.
func (tester *Tester) TestObservability(ctx context.Context, config domain.ProviderConfig, credentials string) ports.ProviderTestResult {
	probe, ok := observabilityProbes[config.Type]
	if !ok {
		return ports.ProviderTestResult{Error: "unsupported provider type"}
	}
	return tester.run(ctx, http.MethodGet, config.URL, probe.path, credentials, probe.auth, config.Timeout)
}

// TestLLM runs the per-type probe against an LLM provider configuration.
func (tester *Tester) TestLLM(ctx context.Context, config domain.LLMConfig, apiKey string) ports.ProviderTestResult {
	probe, ok := llmProbes[config.Type]
	if !ok {
		return ports.ProviderTestResult{Error: "unsupported llm provider type"}
	}
	return tester.run(ctx, probe.method, config.BaseURL, probe.path, apiKey, probe.auth, config.Timeout)
}

func (tester *Tester) run(ctx context.Context, method, baseURL, path, credentials string, auth func(*http.Request, string), timeout time.Duration) ports.ProviderTestResult {
	if strings.TrimSpace(baseURL) == "" {
		return ports.ProviderTestResult{Error: "base url is empty"}
	}
	probeTimeout := defaultProbeTimeout
	if timeout > 0 && timeout < defaultProbeTimeout {
		probeTimeout = timeout
	}
	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	target := strings.TrimRight(baseURL, "/") + path
	request, err := http.NewRequestWithContext(probeCtx, method, target, nil)
	if err != nil {
		return ports.ProviderTestResult{Error: inboundhttp.SanitizeExternalMessage(err.Error())}
	}
	if auth != nil {
		auth(request, credentials)
	}

	startedAt := time.Now()
	response, err := tester.client.Do(request)
	latencyMs := time.Since(startedAt).Milliseconds()
	if err != nil {
		return ports.ProviderTestResult{LatencyMs: latencyMs, Error: inboundhttp.SanitizeExternalMessage(err.Error())}
	}
	defer response.Body.Close()
	if response.StatusCode >= 500 {
		return ports.ProviderTestResult{LatencyMs: latencyMs, Error: fmt.Sprintf("upstream returned status %d", response.StatusCode)}
	}
	return ports.ProviderTestResult{Reached: true, LatencyMs: latencyMs}
}

func noAuth(*http.Request, string) {}

func bearerAuth(request *http.Request, credentials string) {
	if credentials != "" {
		request.Header.Set("Authorization", "Bearer "+credentials)
	}
}

func basicAuth(request *http.Request, credentials string) {
	if credentials == "" {
		return
	}
	parts := strings.SplitN(credentials, ":", 2)
	if len(parts) == 2 {
		request.SetBasicAuth(parts[0], parts[1])
	}
}

func dynatraceAuth(request *http.Request, credentials string) {
	if credentials != "" {
		request.Header.Set("Authorization", "Api-Token "+credentials)
	}
}

func datadogAuth(request *http.Request, credentials string) {
	if credentials == "" {
		return
	}
	parts := strings.SplitN(credentials, ":", 2)
	request.Header.Set("DD-API-KEY", parts[0])
	if len(parts) == 2 {
		request.Header.Set("DD-APPLICATION-KEY", parts[1])
	}
}

func newRelicAuth(request *http.Request, credentials string) {
	if credentials != "" {
		request.Header.Set("API-Key", credentials)
	}
}

func anthropicAuth(request *http.Request, credentials string) {
	if credentials != "" {
		request.Header.Set("x-api-key", credentials)
		request.Header.Set("anthropic-version", "2023-06-01")
	}
}

func azureAuth(request *http.Request, credentials string) {
	if credentials != "" {
		request.Header.Set("api-key", credentials)
	}
}
