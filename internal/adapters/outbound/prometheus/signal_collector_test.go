package prometheus

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/platform/retry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignalCollectorReturnsEvidencePerSampleAndService(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, "/api/v1/query", request.URL.Path)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"status": "success",
			"data": {
				"resultType": "vector",
				"result": [
					{
						"metric": {"__name__": "up", "service": "checkout-service", "instance": "10.0.0.1:9100"},
						"value": [1715508000.000, "1"]
					}
				]
			}
		}`))
	}))
	defer server.Close()

	client, err := NewClient(ClientOptions{BaseURL: server.URL, Timeout: 2 * time.Second})
	require.NoError(t, err)

	collector := NewSignalCollector(client, SignalCollectorOptions{
		Templates: []MetricTemplate{
			{Name: "service_up", Expression: `up{service="{service}"}`, Unit: "ratio"},
		},
	})

	evidence, err := collector.Collect(context.Background(), domain.AnalysisRequest{
		Goal:             "investigate checkout latency",
		AffectedServices: []string{"checkout-service"},
		Signals:          []domain.SignalType{domain.SignalMetrics},
		TimeWindow: domain.TimeWindow{
			End: time.Date(2026, 5, 12, 11, 0, 0, 0, time.UTC),
		},
	})
	require.NoError(t, err)
	require.Len(t, evidence, 1)

	item := evidence[0]
	assert.Equal(t, domain.SignalMetrics, item.Signal)
	assert.Equal(t, "checkout-service", item.Service)
	assert.Equal(t, "service_up", item.Name)
	assert.Equal(t, "prometheus", item.Provider)
	assert.Equal(t, `up{service="checkout-service"}`, item.Query)
	assert.Equal(t, 1.0, item.Score)
	assert.Equal(t, "ratio", item.Unit)
	assert.Equal(t, "checkout-service", item.Attributes["service"])
	_, hasName := item.Attributes["__name__"]
	assert.False(t, hasName, "internal __name__ label must be filtered out")
}

func TestSignalCollectorSkipsWhenMetricsNotRequested(t *testing.T) {
	t.Parallel()

	client, err := NewClient(ClientOptions{BaseURL: "http://prometheus.invalid"})
	require.NoError(t, err)

	collector := NewSignalCollector(client, SignalCollectorOptions{})
	evidence, err := collector.Collect(context.Background(), domain.AnalysisRequest{
		Goal:             "investigate logs only",
		AffectedServices: []string{"checkout-service"},
		Signals:          []domain.SignalType{domain.SignalLogs},
	})
	require.NoError(t, err)
	assert.Empty(t, evidence)
}

func TestSignalCollectorSurfacesQueryError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = writer.Write([]byte(`{"status":"error","error":"parse error"}`))
	}))
	defer server.Close()

	client, err := NewClient(ClientOptions{BaseURL: server.URL})
	require.NoError(t, err)

	collector := NewSignalCollector(client, SignalCollectorOptions{
		Templates: []MetricTemplate{{Name: "broken", Expression: "rate(", Unit: ""}},
	})

	_, err = collector.Collect(context.Background(), domain.AnalysisRequest{
		AffectedServices: []string{"checkout-service"},
		Signals:          []domain.SignalType{domain.SignalMetrics},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "broken")
}

func TestClientQueryRetriesOn5xx(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			writer.WriteHeader(http.StatusBadGateway)
			_, _ = writer.Write([]byte("upstream down"))
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
	}))
	defer server.Close()

	client, err := NewClient(ClientOptions{
		BaseURL:     server.URL,
		Timeout:     2 * time.Second,
		RetryPolicy: retry.Policy{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond},
	})
	require.NoError(t, err)

	samples, err := client.Query(context.Background(), "up", time.Time{})
	require.NoError(t, err)
	assert.Empty(t, samples)
	assert.Equal(t, int32(2), calls.Load())
}

func TestClientQueryDoesNotRetryOnClientError(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = writer.Write([]byte("bad request"))
	}))
	defer server.Close()

	client, err := NewClient(ClientOptions{
		BaseURL:     server.URL,
		Timeout:     2 * time.Second,
		RetryPolicy: retry.Policy{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond},
	})
	require.NoError(t, err)

	_, err = client.Query(context.Background(), "up", time.Time{})
	require.Error(t, err)
	assert.Equal(t, int32(1), calls.Load())
}

func TestEscapeLabelEscapesQuotesAndBackslashes(t *testing.T) {
	t.Parallel()

	assert.Equal(t, `a\\b\"c`, escapeLabel(`a\b"c`))
}
