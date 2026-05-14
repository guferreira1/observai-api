package newrelic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

func TestClientPingIssuesGraphQL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/graphql" {
			t.Fatalf("unexpected path %q", request.URL.Path)
		}
		if request.Header.Get("API-Key") != "nr-key" {
			t.Fatalf("missing api key header: %q", request.Header.Get("API-Key"))
		}
		body, _ := io.ReadAll(request.Body)
		if !strings.Contains(string(body), "actor") {
			t.Fatalf("expected actor query, got %s", body)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"data":{"actor":{"user":{"email":"x"}}}}`))
	}))
	defer server.Close()

	client, err := NewClient(ClientOptions{BaseURL: server.URL, APIKey: "nr-key", AccountID: "12345", Timeout: time.Second})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
}

func TestClientQueryNRQLParsesResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		var envelope map[string]string
		_ = json.Unmarshal(body, &envelope)
		if !strings.Contains(envelope["query"], "account(id: 99)") {
			t.Fatalf("expected account id in query, got %q", envelope["query"])
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"data":{"actor":{"account":{"nrql":{"results":[{"service.name":"checkout","average":42.0}]}}}}}`))
	}))
	defer server.Close()

	client, _ := NewClient(ClientOptions{BaseURL: server.URL, APIKey: "nr-key", AccountID: "99", Timeout: time.Second})
	samples, err := client.QueryNRQL(context.Background(), `SELECT average(duration) FROM Transaction`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(samples) != 1 || samples[0].Service != "checkout" || samples[0].Value != 42.0 {
		t.Fatalf("unexpected samples: %+v", samples)
	}
}

func TestSignalCollectorEmitsEvidence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"data":{"actor":{"account":{"nrql":{"results":[{"service.name":"checkout","average":12.0}]}}}}}`))
	}))
	defer server.Close()

	client, _ := NewClient(ClientOptions{BaseURL: server.URL, APIKey: "nr", AccountID: "1", Timeout: time.Second})
	collector := NewSignalCollector(client, SignalCollectorOptions{
		Templates: []NRQLTemplate{{Name: "p95", Query: `SELECT percentile(duration, 95) FROM Transaction WHERE service.name = '{service}'`, Unit: "ms"}},
	})

	evidence, err := collector.Collect(context.Background(), domain.AnalysisRequest{
		AffectedServices: []string{"checkout"},
		Signals:          []domain.SignalType{domain.SignalMetrics},
	})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(evidence) != 1 || evidence[0].Service != "checkout" || evidence[0].Source != "newrelic" {
		t.Fatalf("unexpected evidence: %+v", evidence)
	}
}

func TestClientQueryRequiresAccountID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := NewClient(ClientOptions{BaseURL: server.URL, APIKey: "nr", Timeout: time.Second})
	if _, err := client.QueryNRQL(context.Background(), `SELECT count(*) FROM Transaction`); err != ErrAccountMissing {
		t.Fatalf("expected ErrAccountMissing, got %v", err)
	}
}
