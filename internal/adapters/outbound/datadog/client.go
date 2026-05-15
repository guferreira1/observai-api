// Package datadog implements a read-only Datadog signal collector.
//
// The adapter queries the Metrics v1 API and authenticates with the
// DD-API-KEY and DD-APPLICATION-KEY headers. Two-token operators supply
// the value as "<api_key>:<app_key>"; single-token operators may supply
// only the api_key.
package datadog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/guferreira1/observai-api/internal/platform/observability"
	"github.com/guferreira1/observai-api/internal/platform/retry"
)

// Client is a minimal Datadog HTTP client.
type Client struct {
	baseURL     *url.URL
	apiKey      string
	appKey      string
	httpClient  *http.Client
	observer    observability.ProviderObserver
	retryPolicy retry.Policy
}

// ClientOptions configures Datadog HTTP behavior.
//
// Credentials should be supplied as "<api_key>" or "<api_key>:<app_key>".
type ClientOptions struct {
	BaseURL     string
	Credentials string
	Timeout     time.Duration
	Observer    observability.ProviderObserver
	RetryPolicy retry.Policy
}

// ErrUnauthorized indicates the supplied credentials were rejected.
var ErrUnauthorized = errors.New("datadog credentials unauthorized")

// NewClient validates the base URL and returns a configured Datadog client.
func NewClient(opts ClientOptions) (*Client, error) {
	parsed, err := url.Parse(opts.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse datadog base url: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("datadog base url must include scheme and host")
	}
	apiKey, appKey := splitCredentials(opts.Credentials)
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
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
		apiKey:      apiKey,
		appKey:      appKey,
		httpClient:  &http.Client{Timeout: timeout},
		observer:    observer,
		retryPolicy: retryPolicy,
	}, nil
}

// Ping verifies the credentials are valid via /api/v1/validate.
func (client *Client) Ping(ctx context.Context) (err error) {
	startedAt := time.Now()
	defer func() { client.observer.Observe("datadog", "ping", time.Since(startedAt), err) }()

	request, err := client.newRequest(ctx, http.MethodGet, "/api/v1/validate", nil)
	if err != nil {
		return err
	}
	response, err := client.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("datadog ping: %w", err)
	}
	defer response.Body.Close()
	switch {
	case response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden:
		return ErrUnauthorized
	case response.StatusCode >= 500:
		return fmt.Errorf("datadog ping returned status %d", response.StatusCode)
	}
	return nil
}

// MetricSample is a single point returned by the Datadog metrics query API.
type MetricSample struct {
	MetricName string
	Service    string
	Value      float64
	Unit       string
	Observed   time.Time
}

// QueryMetric runs the supplied query string against /api/v1/query.
//
// The query follows the standard Datadog format, e.g.
// `avg:trace.servlet.request{service:checkout}`. The caller is expected to
// substitute the service name with proper escaping before invoking.
func (client *Client) QueryMetric(ctx context.Context, query string, from, to time.Time) (samples []MetricSample, err error) {
	startedAt := time.Now()
	defer func() { client.observer.Observe("datadog", "query_metric", time.Since(startedAt), err) }()

	if to.IsZero() {
		to = time.Now().UTC()
	}
	if from.IsZero() {
		from = to.Add(-15 * time.Minute)
	}
	values := url.Values{}
	values.Set("from", strconv.FormatInt(from.Unix(), 10))
	values.Set("to", strconv.FormatInt(to.Unix(), 10))
	values.Set("query", query)

	var body []byte
	err = retry.Do(ctx, client.retryPolicy, isRetryableDatadog, func(int) error {
		request, requestErr := client.newRequest(ctx, http.MethodGet, "/api/v1/query?"+values.Encode(), nil)
		if requestErr != nil {
			return requestErr
		}
		response, doErr := client.httpClient.Do(request)
		if doErr != nil {
			return &transientError{err: fmt.Errorf("datadog query: %w", doErr)}
		}
		defer response.Body.Close()
		payload, readErr := io.ReadAll(response.Body)
		if readErr != nil {
			return &transientError{err: fmt.Errorf("read datadog response: %w", readErr)}
		}
		switch {
		case response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden:
			return ErrUnauthorized
		case response.StatusCode >= 500:
			return &transientError{err: fmt.Errorf("datadog returned status %d", response.StatusCode)}
		case response.StatusCode >= 400:
			return fmt.Errorf("datadog returned status %d", response.StatusCode)
		}
		body = payload
		return nil
	})
	if err != nil {
		return nil, err
	}
	return parseMetricResponse(body)
}

func (client *Client) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	target := *client.baseURL
	target.Path = joinPath(target.Path, path)
	if questionIndex := strings.IndexByte(path, '?'); questionIndex >= 0 {
		target.Path = joinPath(client.baseURL.Path, path[:questionIndex])
		target.RawQuery = path[questionIndex+1:]
	}
	request, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return nil, fmt.Errorf("build datadog request: %w", err)
	}
	if client.apiKey != "" {
		request.Header.Set("DD-API-KEY", client.apiKey)
	}
	if client.appKey != "" {
		request.Header.Set("DD-APPLICATION-KEY", client.appKey)
	}
	request.Header.Set("Accept", "application/json")
	return request, nil
}

type metricResponse struct {
	Series []struct {
		Metric string   `json:"metric"`
		Scope  string   `json:"scope"`
		TagSet []string `json:"tag_set"`
		Unit   []*struct {
			Name string `json:"name"`
		} `json:"unit"`
		Pointlist [][]float64 `json:"pointlist"`
	} `json:"series"`
}

func parseMetricResponse(payload []byte) ([]MetricSample, error) {
	if len(payload) == 0 {
		return nil, nil
	}
	var decoded metricResponse
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil, fmt.Errorf("decode datadog metric response: %w", err)
	}
	var samples []MetricSample
	for _, series := range decoded.Series {
		service := serviceFromScopeAndTags(series.Scope, series.TagSet)
		unit := pickUnit(series.Unit)
		for _, point := range series.Pointlist {
			if len(point) < 2 {
				continue
			}
			samples = append(samples, MetricSample{
				MetricName: series.Metric,
				Service:    service,
				Value:      point[1],
				Unit:       unit,
				Observed:   time.UnixMilli(int64(point[0])).UTC(),
			})
		}
	}
	return samples, nil
}

func serviceFromScopeAndTags(scope string, tags []string) string {
	for _, candidate := range append([]string{scope}, tags...) {
		for _, prefix := range []string{"service:", "service.name:"} {
			if strings.HasPrefix(candidate, prefix) {
				return strings.TrimPrefix(candidate, prefix)
			}
		}
	}
	return ""
}

func pickUnit(values []*struct {
	Name string `json:"name"`
}) string {
	for _, unit := range values {
		if unit != nil && unit.Name != "" {
			return unit.Name
		}
	}
	return ""
}

func splitCredentials(value string) (string, string) {
	cleaned := strings.TrimSpace(value)
	if cleaned == "" {
		return "", ""
	}
	parts := strings.SplitN(cleaned, ":", 2)
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], parts[1]
}

func joinPath(left, right string) string {
	if left == "" || left == "/" {
		return right
	}
	if right == "" {
		return left
	}
	trimmedLeft := strings.TrimRight(left, "/")
	trimmedRight := strings.TrimLeft(right, "/")
	return trimmedLeft + "/" + trimmedRight
}

type transientError struct{ err error }

func (transient *transientError) Error() string     { return transient.err.Error() }
func (transient *transientError) Unwrap() error     { return transient.err }
func (transient *transientError) IsTransient() bool { return true }

func isRetryableDatadog(err error) bool {
	var transient *transientError
	return errors.As(err, &transient)
}
