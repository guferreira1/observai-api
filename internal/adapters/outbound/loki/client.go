// Package loki implements a read-only log signal collector backed by Loki's
// HTTP API.
//
// The adapter targets /loki/api/v1/query_range and renders LogQL queries
// from server-controlled templates with sanitized service labels. Arbitrary
// user-supplied LogQL is never forwarded so the analysis flow stays safe
// against query injection.
package loki

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/guferreira1/observai-api/internal/platform/observability"
	"github.com/guferreira1/observai-api/internal/platform/retry"
)

// ClientOptions configures the Loki HTTP client.
type ClientOptions struct {
	BaseURL     string
	Timeout     time.Duration
	Observer    observability.ProviderObserver
	RetryPolicy retry.Policy
}

// Client is a minimal Loki HTTP client.
type Client struct {
	baseURL     *url.URL
	httpClient  *http.Client
	observer    observability.ProviderObserver
	retryPolicy retry.Policy
}

// NewClient validates the base URL and returns a configured Loki client.
func NewClient(opts ClientOptions) (*Client, error) {
	parsed, err := url.Parse(opts.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse loki base url: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("loki base url must include scheme and host")
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
		httpClient:  &http.Client{Timeout: timeout},
		observer:    observer,
		retryPolicy: retryPolicy,
	}, nil
}

// Sample is a single aggregated LogQL series point returned by query_range.
//
// Loki returns a value array per stream; the collector reduces it to the
// most recent value so analysis evidence can carry a stable score number
// (typically a counter of matched events in the window).
type Sample struct {
	Labels map[string]string
	Value  float64
	Time   time.Time
}

type queryRangeResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Stream map[string]string `json:"metric"`
			Values [][2]string       `json:"values"`
		} `json:"result"`
	} `json:"data"`
	ErrorType string `json:"errorType"`
	Error     string `json:"error"`
}

// Ping verifies Loki readiness via /ready.
func (client *Client) Ping(ctx context.Context) (err error) {
	startedAt := time.Now()
	defer func() {
		client.observer.Observe("loki", "ping", time.Since(startedAt), err)
	}()

	endpoint := *client.baseURL
	endpoint.Path = joinPath(endpoint.Path, "/ready")

	request, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if reqErr != nil {
		return fmt.Errorf("build loki health request: %w", reqErr)
	}
	response, doErr := client.httpClient.Do(request)
	if doErr != nil {
		return fmt.Errorf("call loki health: %w", doErr)
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("loki health returned status %d", response.StatusCode)
	}
	return nil
}

// QueryRange evaluates a LogQL query across the supplied window and returns
// the matrix result as a list of Samples. Only matrix (aggregate) results
// are accepted so the analysis stage receives consistent numeric scores.
func (client *Client) QueryRange(ctx context.Context, query string, start time.Time, end time.Time, step time.Duration) (samples []Sample, err error) {
	startedAt := time.Now()
	defer func() {
		client.observer.Observe("loki", "query_range", time.Since(startedAt), err)
	}()

	if query == "" {
		return nil, fmt.Errorf("loki query is required")
	}
	if start.IsZero() || end.IsZero() || !end.After(start) {
		return nil, fmt.Errorf("loki query requires a non-empty time window")
	}
	if step <= 0 {
		step = 60 * time.Second
	}

	endpoint := *client.baseURL
	endpoint.Path = joinPath(endpoint.Path, "/loki/api/v1/query_range")
	values := url.Values{}
	values.Set("query", query)
	values.Set("start", strconv.FormatInt(start.UTC().UnixNano(), 10))
	values.Set("end", strconv.FormatInt(end.UTC().UnixNano(), 10))
	values.Set("step", strconv.FormatFloat(step.Seconds(), 'f', -1, 64))
	values.Set("direction", "backward")
	endpoint.RawQuery = values.Encode()

	err = retry.Do(ctx, client.retryPolicy, isRetryable, func(int) error {
		result, attemptErr := client.executeQuery(ctx, endpoint.String())
		if attemptErr != nil {
			return attemptErr
		}
		samples = result
		return nil
	})
	if err != nil {
		return nil, err
	}
	return samples, nil
}

func (client *Client) executeQuery(ctx context.Context, endpoint string) ([]Sample, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build loki request: %w", err)
	}
	request.Header.Set("Accept", "application/json")

	response, err := client.httpClient.Do(request)
	if err != nil {
		return nil, &transientError{err: fmt.Errorf("call loki: %w", err)}
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, &transientError{err: fmt.Errorf("read loki response: %w", err)}
	}

	if response.StatusCode >= http.StatusInternalServerError {
		return nil, &transientError{err: fmt.Errorf("loki status %d: %s", response.StatusCode, truncate(string(body), 200))}
	}
	if response.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("loki status %d: %s", response.StatusCode, truncate(string(body), 200))
	}

	var parsed queryRangeResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("decode loki response: %w", err)
	}
	if parsed.Status != "success" {
		return nil, fmt.Errorf("loki query failed: %s", parsed.Error)
	}
	if parsed.Data.ResultType != "matrix" {
		return nil, fmt.Errorf("unsupported loki result type %q", parsed.Data.ResultType)
	}

	samples := make([]Sample, 0, len(parsed.Data.Result))
	for _, entry := range parsed.Data.Result {
		if len(entry.Values) == 0 {
			continue
		}
		last := entry.Values[len(entry.Values)-1]
		seconds, secondsErr := strconv.ParseFloat(last[0], 64)
		if secondsErr != nil {
			return nil, fmt.Errorf("parse loki sample time %q: %w", last[0], secondsErr)
		}
		value, valueErr := strconv.ParseFloat(last[1], 64)
		if valueErr != nil {
			return nil, fmt.Errorf("parse loki sample value %q: %w", last[1], valueErr)
		}
		samples = append(samples, Sample{
			Labels: entry.Stream,
			Value:  value,
			Time:   time.Unix(0, int64(seconds*1e9)).UTC(),
		})
	}

	return samples, nil
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
