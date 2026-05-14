package datadog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

func TestClientPingValidatesCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/validate" {
			t.Fatalf("unexpected path %q", request.URL.Path)
		}
		if request.Header.Get("DD-API-KEY") != "api" || request.Header.Get("DD-APPLICATION-KEY") != "app" {
			t.Fatalf("missing credentials: %q %q", request.Header.Get("DD-API-KEY"), request.Header.Get("DD-APPLICATION-KEY"))
		}
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := NewClient(ClientOptions{BaseURL: server.URL, Credentials: "api:app", Timeout: time.Second})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
}

func TestClientPingMapsUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client, _ := NewClient(ClientOptions{BaseURL: server.URL, Credentials: "x:y", Timeout: time.Second})
	if err := client.Ping(context.Background()); err != ErrUnauthorized {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestSignalCollectorEmitsEvidence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
            "series": [{
                "metric": "trace.servlet.request",
                "scope": "service:checkout",
                "tag_set": ["env:prod"],
                "unit": [{"name": "ms"}],
                "pointlist": [[1700000000000, 12.0]]
            }]
        }`))
	}))
	defer server.Close()

	client, _ := NewClient(ClientOptions{BaseURL: server.URL, Credentials: "api:app", Timeout: time.Second})
	collector := NewSignalCollector(client, SignalCollectorOptions{
		Templates: []MetricTemplate{{Name: "rt", Query: `avg:trace.servlet.request{service:{service}}`, Unit: "ms"}},
	})

	evidence, err := collector.Collect(context.Background(), domain.AnalysisRequest{
		AffectedServices: []string{"checkout"},
		Signals:          []domain.SignalType{domain.SignalMetrics},
	})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(evidence) != 1 || evidence[0].Service != "checkout" || evidence[0].Source != "datadog" {
		t.Fatalf("unexpected evidence: %+v", evidence)
	}
}
