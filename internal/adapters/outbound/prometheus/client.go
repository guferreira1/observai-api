// Package prometheus implements a read-only Prometheus signal collector.
//
// The adapter exposes only the surface required by the analysis use case:
// instant queries against the public HTTP API. It never sends mutating
// requests and never accepts arbitrary user PromQL — queries are always
// rendered from server-controlled templates with sanitized service labels.
package prometheus

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/guferreira1/observai-api/internal/platform/observability"
)

// Client is a minimal Prometheus HTTP client tailored for instant queries.
type Client struct {
	baseURL    *url.URL
	httpClient *http.Client
	observer   observability.ProviderObserver
}

// ClientOptions configures Prometheus HTTP behavior.
type ClientOptions struct {
	BaseURL  string
	Timeout  time.Duration
	Observer observability.ProviderObserver
}

// NewClient validates the base URL and returns a configured Prometheus client.
func NewClient(opts ClientOptions) (*Client, error) {
	parsed, err := url.Parse(opts.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse prometheus base url: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("prometheus base url must include scheme and host")
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	observer := opts.Observer
	if observer == nil {
		observer = observability.NoopProviderObserver{}
	}

	return &Client{
		baseURL:    parsed,
		httpClient: &http.Client{Timeout: timeout},
		observer:   observer,
	}, nil
}

// Ping verifies that the Prometheus instance is reachable and ready to serve queries.
//
// It hits the public /-/healthy endpoint with the supplied context; the call is
// counted by the configured ProviderObserver as operation "ping".
func (client *Client) Ping(ctx context.Context) (err error) {
	startedAt := time.Now()
	defer func() {
		client.observer.Observe("prometheus", "ping", time.Since(startedAt), err)
	}()

	endpoint := *client.baseURL
	endpoint.Path = joinPath(endpoint.Path, "/-/healthy")

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return fmt.Errorf("build prometheus health request: %w", err)
	}

	response, err := client.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("call prometheus health: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("prometheus health returned status %d", response.StatusCode)
	}
	return nil
}

// InstantSample describes a single Prometheus instant query result.
type InstantSample struct {
	Labels map[string]string
	Value  float64
	Time   time.Time
}

type instantResponse struct {
	Status string `json:"status"`
	Error  string `json:"error"`
	Data   struct {
		ResultType string            `json:"resultType"`
		Result     []json.RawMessage `json:"result"`
	} `json:"data"`
}

// Query executes a PromQL instant query at evaluation time `at` (UTC).
// When at is the zero value the server's current time is used.
func (client *Client) Query(ctx context.Context, query string, at time.Time) (samples []InstantSample, err error) {
	startedAt := time.Now()
	defer func() {
		client.observer.Observe("prometheus", "query", time.Since(startedAt), err)
	}()

	if query == "" {
		return nil, fmt.Errorf("prometheus query is required")
	}

	endpoint := *client.baseURL
	endpoint.Path = joinPath(endpoint.Path, "/api/v1/query")
	values := url.Values{}
	values.Set("query", query)
	if !at.IsZero() {
		values.Set("time", strconv.FormatFloat(float64(at.UTC().UnixNano())/1e9, 'f', -1, 64))
	}
	endpoint.RawQuery = values.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build prometheus request: %w", err)
	}
	request.Header.Set("Accept", "application/json")

	response, err := client.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("call prometheus: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("read prometheus response: %w", err)
	}

	if response.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("prometheus returned status %d: %s", response.StatusCode, truncate(string(body), 200))
	}

	var parsed instantResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("decode prometheus response: %w", err)
	}
	if parsed.Status != "success" {
		return nil, fmt.Errorf("prometheus query failed: %s", parsed.Error)
	}
	if parsed.Data.ResultType != "vector" {
		return nil, fmt.Errorf("unsupported prometheus result type %q", parsed.Data.ResultType)
	}

	samples = make([]InstantSample, 0, len(parsed.Data.Result))
	for _, raw := range parsed.Data.Result {
		sample, decodeErr := decodeVectorSample(raw)
		if decodeErr != nil {
			return nil, decodeErr
		}
		samples = append(samples, sample)
	}

	return samples, nil
}

func decodeVectorSample(raw json.RawMessage) (InstantSample, error) {
	var entry struct {
		Metric map[string]string `json:"metric"`
		Value  []json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(raw, &entry); err != nil {
		return InstantSample{}, fmt.Errorf("decode prometheus sample: %w", err)
	}
	if len(entry.Value) != 2 {
		return InstantSample{}, fmt.Errorf("prometheus sample must have [time, value] pair")
	}

	var seconds float64
	if err := json.Unmarshal(entry.Value[0], &seconds); err != nil {
		return InstantSample{}, fmt.Errorf("decode prometheus sample time: %w", err)
	}

	var rawValue string
	if err := json.Unmarshal(entry.Value[1], &rawValue); err != nil {
		return InstantSample{}, fmt.Errorf("decode prometheus sample value: %w", err)
	}
	value, err := strconv.ParseFloat(rawValue, 64)
	if err != nil {
		return InstantSample{}, fmt.Errorf("parse prometheus sample value %q: %w", rawValue, err)
	}

	return InstantSample{
		Labels: entry.Metric,
		Value:  value,
		Time:   time.Unix(0, int64(seconds*1e9)).UTC(),
	}, nil
}

func joinPath(base, suffix string) string {
	if base == "" {
		return suffix
	}
	if base[len(base)-1] == '/' {
		base = base[:len(base)-1]
	}
	if suffix == "" {
		return base
	}
	if suffix[0] != '/' {
		return base + "/" + suffix
	}
	return base + suffix
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max] + "..."
}
