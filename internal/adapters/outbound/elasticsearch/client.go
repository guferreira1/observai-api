// Package elasticsearch implements a logs signal collector backed by the
// Elasticsearch/OpenSearch _search API.
//
// The adapter targets the public _search endpoint with bounded aggregations
// so the analysis pipeline receives stable numeric scores instead of raw
// documents. Basic auth (username/password) is the only auth method
// supported initially; API-key auth can be layered later through the
// CredentialStore.
package elasticsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/guferreira1/observai-api/internal/platform/observability"
	"github.com/guferreira1/observai-api/internal/platform/retry"
)

// ClientOptions configures the Elasticsearch client.
type ClientOptions struct {
	BaseURL     string
	Username    string
	Password    string
	APIKey      string
	Timeout     time.Duration
	Observer    observability.ProviderObserver
	RetryPolicy retry.Policy
}

// Client is a minimal Elasticsearch / OpenSearch HTTP client tailored for
// log aggregations.
type Client struct {
	baseURL     *url.URL
	username    string
	password    string
	apiKey      string
	httpClient  *http.Client
	observer    observability.ProviderObserver
	retryPolicy retry.Policy
}

// NewClient validates the supplied options and returns a ready client.
func NewClient(opts ClientOptions) (*Client, error) {
	parsed, err := url.Parse(strings.TrimSpace(opts.BaseURL))
	if err != nil {
		return nil, fmt.Errorf("parse elasticsearch base url: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("elasticsearch base url must include scheme and host")
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	observer := opts.Observer
	if observer == nil {
		observer = observability.NoopProviderObserver{}
	}

	retryPolicy := opts.RetryPolicy
	if retryPolicy.MaxAttempts <= 0 {
		retryPolicy = retry.Default()
	}

	return &Client{
		baseURL:     parsed,
		username:    opts.Username,
		password:    opts.Password,
		apiKey:      opts.APIKey,
		httpClient:  &http.Client{Timeout: timeout},
		observer:    observer,
		retryPolicy: retryPolicy,
	}, nil
}

// AggregatedCount is the result of a per-service log volume aggregation.
type AggregatedCount struct {
	Service string
	Count   int64
}

// SearchOptions describes a logs aggregation query.
type SearchOptions struct {
	Index          string
	Service        string
	ServiceField   string
	MessagePattern string
	MessageField   string
	TimestampField string
	WindowStart    time.Time
	WindowEnd      time.Time
}

type searchResponse struct {
	Hits struct {
		Total struct {
			Value int64 `json:"value"`
		} `json:"total"`
	} `json:"hits"`
	Error *struct {
		Type   string `json:"type"`
		Reason string `json:"reason"`
	} `json:"error,omitempty"`
}

// Ping verifies reachability via the cluster root.
func (client *Client) Ping(ctx context.Context) (err error) {
	startedAt := time.Now()
	defer func() { client.observer.Observe("elasticsearch", "ping", time.Since(startedAt), err) }()

	endpoint := *client.baseURL
	endpoint.Path = joinPath(endpoint.Path, "/")

	request, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if reqErr != nil {
		return fmt.Errorf("build elasticsearch health request: %w", reqErr)
	}
	client.attachAuth(request)
	response, doErr := client.httpClient.Do(request)
	if doErr != nil {
		return fmt.Errorf("call elasticsearch health: %w", doErr)
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("elasticsearch health returned status %d", response.StatusCode)
	}
	return nil
}

// CountMatchingLogs returns the total number of log documents that match
// the supplied service, regex pattern and time window.
func (client *Client) CountMatchingLogs(ctx context.Context, opts SearchOptions) (count int64, err error) {
	startedAt := time.Now()
	defer func() { client.observer.Observe("elasticsearch", "count_logs", time.Since(startedAt), err) }()

	index := strings.TrimSpace(opts.Index)
	if index == "" {
		index = "_all"
	}

	timestampField := strings.TrimSpace(opts.TimestampField)
	if timestampField == "" {
		timestampField = "@timestamp"
	}
	serviceField := strings.TrimSpace(opts.ServiceField)
	if serviceField == "" {
		serviceField = "service.name"
	}
	messageField := strings.TrimSpace(opts.MessageField)
	if messageField == "" {
		messageField = "message"
	}

	must := []map[string]any{
		{
			"range": map[string]any{
				timestampField: map[string]any{
					"gte": opts.WindowStart.UTC().Format(time.RFC3339Nano),
					"lte": opts.WindowEnd.UTC().Format(time.RFC3339Nano),
				},
			},
		},
	}
	if strings.TrimSpace(opts.Service) != "" {
		must = append(must, map[string]any{
			"match_phrase": map[string]any{serviceField: opts.Service},
		})
	}
	if strings.TrimSpace(opts.MessagePattern) != "" {
		must = append(must, map[string]any{
			"regexp": map[string]any{messageField: map[string]any{"value": opts.MessagePattern}},
		})
	}

	payload := map[string]any{
		"track_total_hits": true,
		"size":             0,
		"query":            map[string]any{"bool": map[string]any{"must": must}},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("encode elasticsearch query: %w", err)
	}

	endpoint := *client.baseURL
	endpoint.Path = joinPath(endpoint.Path, "/"+url.PathEscape(index)+"/_search")

	err = retry.Do(ctx, client.retryPolicy, isRetryable, func(int) error {
		value, attemptErr := client.executeSearch(ctx, endpoint.String(), body)
		if attemptErr != nil {
			return attemptErr
		}
		count = value
		return nil
	})
	return count, err
}

func (client *Client) executeSearch(ctx context.Context, endpoint string, body []byte) (int64, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("build elasticsearch request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	client.attachAuth(request)

	response, err := client.httpClient.Do(request)
	if err != nil {
		return 0, &transientError{err: fmt.Errorf("call elasticsearch: %w", err)}
	}
	defer response.Body.Close()

	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return 0, &transientError{err: fmt.Errorf("read elasticsearch response: %w", err)}
	}

	if response.StatusCode >= http.StatusInternalServerError {
		return 0, &transientError{err: fmt.Errorf("elasticsearch status %d: %s", response.StatusCode, truncate(string(raw), 200))}
	}
	if response.StatusCode >= http.StatusBadRequest {
		return 0, fmt.Errorf("elasticsearch status %d: %s", response.StatusCode, truncate(string(raw), 200))
	}

	var parsed searchResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return 0, fmt.Errorf("decode elasticsearch response: %w", err)
	}
	if parsed.Error != nil {
		return 0, fmt.Errorf("elasticsearch error: %s", parsed.Error.Reason)
	}
	return parsed.Hits.Total.Value, nil
}

func (client *Client) attachAuth(request *http.Request) {
	if strings.TrimSpace(client.apiKey) != "" {
		request.Header.Set("Authorization", "ApiKey "+client.apiKey)
		return
	}
	if strings.TrimSpace(client.username) != "" {
		request.SetBasicAuth(client.username, client.password)
	}
}

type transientError struct{ err error }

func (e *transientError) Error() string { return e.err.Error() }
func (e *transientError) Unwrap() error { return e.err }

func isRetryable(err error) bool {
	var transient *transientError
	return errors.As(err, &transient)
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
