package jaeger

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignalCollectorEmitsTraceEvidence(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, "/api/traces", request.URL.Path)
		assert.Equal(t, "observai-api", request.URL.Query().Get("service"))
		assert.Equal(t, "5", request.URL.Query().Get("limit"))
		_, _ = writer.Write([]byte(`{
			"data": [
				{
					"traceID": "trace-short",
					"spans": [
						{"traceID":"trace-short","spanID":"span-1","operationName":"GET /health","processID":"p1","startTime":1000000,"duration":1000}
					],
					"processes": {"p1":{"serviceName":"observai-api","tags":[]}}
				},
				{
					"traceID": "trace-long",
					"spans": [
						{"traceID":"trace-long","spanID":"span-root","operationName":"POST /v1/analyses","processID":"p1","startTime":2000000,"duration":8000},
						{"traceID":"trace-long","spanID":"span-child","operationName":"provider call","references":[{"refType":"CHILD_OF","traceID":"trace-long","spanID":"span-root"}],"processID":"p2","startTime":2001000,"duration":4000,"tags":[{"key":"error","type":"bool","value":true}]}
					],
					"processes": {
						"p1":{"serviceName":"observai-api","tags":[]},
						"p2":{"serviceName":"openai","tags":[]}
					}
				}
			]
		}`))
	}))
	defer server.Close()

	client, err := NewClient(ClientOptions{BaseURL: server.URL})
	require.NoError(t, err)

	collector := NewSignalCollector(client, SignalCollectorOptions{})
	evidence, err := collector.Collect(context.Background(), domain.AnalysisRequest{
		TimeWindow: domain.TimeWindow{
			Start: time.Unix(1, 0).UTC(),
			End:   time.Unix(3, 0).UTC(),
		},
		AffectedServices: []string{"observai-api"},
		Signals:          []domain.SignalType{domain.SignalTraces},
	})
	require.NoError(t, err)
	require.Len(t, evidence, 1)

	assert.Equal(t, domain.SignalTraces, evidence[0].Signal)
	assert.Equal(t, "jaeger", evidence[0].Provider)
	assert.Equal(t, "observai-api", evidence[0].Service)
	assert.Equal(t, "trace-long", evidence[0].Attributes["traceId"])
	assert.Equal(t, "2", evidence[0].Attributes["spanCount"])
	assert.Equal(t, "1", evidence[0].Attributes["errorSpanCount"])
	assert.Equal(t, "POST /v1/analyses", evidence[0].Attributes["rootOperation"])
}

func TestSignalCollectorSkipsWhenTracesAreNotRequested(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		t.Fatalf("unexpected request to %s", request.URL.Path)
	}))
	defer server.Close()

	client, err := NewClient(ClientOptions{BaseURL: server.URL})
	require.NoError(t, err)

	collector := NewSignalCollector(client, SignalCollectorOptions{})
	evidence, err := collector.Collect(context.Background(), domain.AnalysisRequest{
		AffectedServices: []string{"observai-api"},
		Signals:          []domain.SignalType{domain.SignalLogs},
	})
	require.NoError(t, err)
	assert.Empty(t, evidence)
}
