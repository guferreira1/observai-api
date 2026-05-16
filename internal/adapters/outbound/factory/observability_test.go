package factory

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/platform/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildObservabilityWiresJaegerAsTraceSignalCollector(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, "/api/traces", request.URL.Path)
		assert.Equal(t, "observai-api", request.URL.Query().Get("service"))
		_, _ = writer.Write([]byte(`{
			"data": [
				{
					"traceID": "trace-1",
					"spans": [
						{"traceID":"trace-1","spanID":"span-1","operationName":"GET /v1/analyses","processID":"p1","startTime":1000000,"duration":3000}
					],
					"processes": {"p1":{"serviceName":"observai-api","tags":[]}}
				}
			]
		}`))
	}))
	defer server.Close()

	result, err := BuildObservability(config.Config{
		Observability: config.ObservabilityConfig{
			Providers: []config.ObservabilityProviderConfig{
				{Type: "jaeger", Name: "Jaeger", URL: server.URL, Signals: []string{"traces"}},
			},
		},
	}, Dependencies{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	require.NoError(t, err)
	require.NotNil(t, result.Clients.Jaeger)
	require.Len(t, result.Capabilities, 1)
	assert.Equal(t, "jaeger", result.Capabilities[0].Type)

	evidence, err := result.Collector.Collect(context.Background(), domain.AnalysisRequest{
		AffectedServices: []string{"observai-api"},
		Signals:          []domain.SignalType{domain.SignalTraces},
	})
	require.NoError(t, err)
	require.Len(t, evidence, 1)
	assert.Equal(t, "trace-1", evidence[0].Attributes["traceId"])
}
