package dynatrace

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

func TestClientPingHitsClusterVersion(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		called = true
		if request.URL.Path != "/api/v2/clusterversion" {
			t.Fatalf("unexpected path %q", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Api-Token dt-token" {
			t.Fatalf("missing auth header: %q", request.Header.Get("Authorization"))
		}
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := NewClient(ClientOptions{BaseURL: server.URL, APIToken: "dt-token", Timeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
	if !called {
		t.Fatalf("server was not called")
	}
}

func TestClientQueryMetricParsesResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v2/metrics/query" {
			t.Fatalf("unexpected path %q", request.URL.Path)
		}
		if !strings.Contains(request.URL.RawQuery, "metricSelector=") {
			t.Fatalf("missing metricSelector: %s", request.URL.RawQuery)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
            "result": [{
                "metricId": "builtin:service.response.time:avg",
                "unit": "ms",
                "data": [{
                    "dimensionMap": {"dt.entity.service.name": "checkout"},
                    "timestamps": [1700000000000],
                    "values": [42.5]
                }]
            }]
        }`))
	}))
	defer server.Close()

	client, err := NewClient(ClientOptions{BaseURL: server.URL, APIToken: "x", Timeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	samples, err := client.QueryMetric(context.Background(), "builtin:service.response.time:avg", "1m", time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(samples) != 1 || samples[0].Service != "checkout" || samples[0].Value != 42.5 || samples[0].Unit != "ms" {
		t.Fatalf("unexpected samples: %+v", samples)
	}
}

func TestSignalCollectorEmitsEvidence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
            "result": [{
                "metricId": "builtin:service.response.time:avg",
                "unit": "ms",
                "data": [{
                    "dimensionMap": {"dt.entity.service.name": "checkout"},
                    "timestamps": [1700000000000],
                    "values": [12.0]
                }]
            }]
        }`))
	}))
	defer server.Close()

	client, _ := NewClient(ClientOptions{BaseURL: server.URL, APIToken: "x", Timeout: 2 * time.Second})
	collector := NewSignalCollector(client, SignalCollectorOptions{
		Templates: []MetricTemplate{{Name: "rt", Selector: `builtin:service.response.time:filter(eq("dt.entity.service.name","{service}")):avg`, Unit: "ms"}},
	})

	evidence, err := collector.Collect(context.Background(), domain.AnalysisRequest{
		AffectedServices: []string{"checkout"},
		Signals:          []domain.SignalType{domain.SignalMetrics},
	})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(evidence) != 1 {
		t.Fatalf("expected 1 evidence, got %d (%+v)", len(evidence), evidence)
	}
	if evidence[0].Service != "checkout" || evidence[0].Source != "dynatrace" || evidence[0].Score != 12.0 {
		t.Fatalf("unexpected evidence: %+v", evidence[0])
	}
}

func TestClientQueryMetricMapsUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client, _ := NewClient(ClientOptions{BaseURL: server.URL, APIToken: "wrong", Timeout: time.Second})
	_, err := client.QueryMetric(context.Background(), "builtin:service.response.time", "", time.Time{}, time.Time{})
	if err != ErrUnauthorized {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}
