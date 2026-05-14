// Package dynatrace implements a read-only Dynatrace signal collector.
//
// The adapter queries the Metrics v2 and Logs v2 APIs. Authentication uses
// the "Authorization: Api-Token" header so the operator can scope the
// token to read-only data ingest entitlements.
package dynatrace

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

// Client is a minimal Dynatrace HTTP client.
type Client struct {
	baseURL     *url.URL
	apiToken    string
	httpClient  *http.Client
	observer    observability.ProviderObserver
	retryPolicy retry.Policy
}

// ClientOptions configures Dynatrace HTTP behavior.
type ClientOptions struct {
	BaseURL     string
	APIToken    string
	Timeout     time.Duration
	Observer    observability.ProviderObserver
	RetryPolicy retry.Policy
}

// NewClient validates the base URL and returns a configured Dynatrace client.
func NewClient(opts ClientOptions) (*Client, error) {
	parsed, err := url.Parse(opts.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse dynatrace base url: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("dynatrace base url must include scheme and host")
	}
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
		apiToken:    opts.APIToken,
		httpClient:  &http.Client{Timeout: timeout},
		observer:    observer,
		retryPolicy: retryPolicy,
	}, nil
}

// Ping verifies the cluster is reachable by calling /api/v2/clusterversion.
func (client *Client) Ping(ctx context.Context) (err error) {
	startedAt := time.Now()
	defer func() { client.observer.Observe("dynatrace", "ping", time.Since(startedAt), err) }()

	request, err := client.newRequest(ctx, http.MethodGet, "/api/v2/clusterversion", nil)
	if err != nil {
		return err
	}
	response, err := client.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("dynatrace ping: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode >= 500 {
		return fmt.Errorf("dynatrace ping returned status %d", response.StatusCode)
	}
	return nil
}

// MetricSample is a single point returned by /api/v2/metrics/query.
type MetricSample struct {
	MetricID string
	Service  string
	Value    float64
	Unit     string
	Observed time.Time
}

// QueryMetric runs the supplied metric selector and resolution against the
// Dynatrace Metrics v2 API.
func (client *Client) QueryMetric(ctx context.Context, selector string, resolution string, from, to time.Time) (samples []MetricSample, err error) {
	startedAt := time.Now()
	defer func() { client.observer.Observe("dynatrace", "query_metric", time.Since(startedAt), err) }()

	query := url.Values{}
	query.Set("metricSelector", selector)
	if resolution != "" {
		query.Set("resolution", resolution)
	}
	if !from.IsZero() {
		query.Set("from", strconv.FormatInt(from.UnixMilli(), 10))
	}
	if !to.IsZero() {
		query.Set("to", strconv.FormatInt(to.UnixMilli(), 10))
	}

	var body []byte
	err = retry.Do(ctx, client.retryPolicy, isRetryableDynatrace, func(int) error {
		request, requestErr := client.newRequest(ctx, http.MethodGet, "/api/v2/metrics/query?"+query.Encode(), nil)
		if requestErr != nil {
			return requestErr
		}
		response, doErr := client.httpClient.Do(request)
		if doErr != nil {
			return &transientError{err: fmt.Errorf("dynatrace query metric: %w", doErr)}
		}
		defer response.Body.Close()
		payload, readErr := io.ReadAll(response.Body)
		if readErr != nil {
			return &transientError{err: fmt.Errorf("read dynatrace response: %w", readErr)}
		}
		if response.StatusCode >= 500 {
			return &transientError{err: fmt.Errorf("dynatrace returned status %d", response.StatusCode)}
		}
		if response.StatusCode == http.StatusUnauthorized {
			return ErrUnauthorized
		}
		if response.StatusCode >= 400 {
			return fmt.Errorf("dynatrace returned status %d", response.StatusCode)
		}
		body = payload
		return nil
	})
	if err != nil {
		return nil, err
	}
	return parseMetricResponse(body)
}

type transientError struct{ err error }

func (transient *transientError) Error() string     { return transient.err.Error() }
func (transient *transientError) Unwrap() error     { return transient.err }
func (transient *transientError) IsTransient() bool { return true }

func isRetryableDynatrace(err error) bool {
	var transient *transientError
	return errors.As(err, &transient)
}

func (client *Client) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	target := *client.baseURL
	target.Path = singleSlash(target.Path, path)
	if questionIndex := indexByte(path, '?'); questionIndex >= 0 {
		target.RawQuery = path[questionIndex+1:]
		target.Path = singleSlash(client.baseURL.Path, path[:questionIndex])
	}
	request, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return nil, fmt.Errorf("build dynatrace request: %w", err)
	}
	if client.apiToken != "" {
		request.Header.Set("Authorization", "Api-Token "+client.apiToken)
	}
	request.Header.Set("Accept", "application/json")
	return request, nil
}

type metricResponse struct {
	Result []struct {
		MetricID string `json:"metricId"`
		Data     []struct {
			Dimensions   []string          `json:"dimensions"`
			DimensionMap map[string]string `json:"dimensionMap"`
			Timestamps   []int64           `json:"timestamps"`
			Values       []float64         `json:"values"`
		} `json:"data"`
		Unit string `json:"unit"`
	} `json:"result"`
}

func parseMetricResponse(payload []byte) ([]MetricSample, error) {
	if len(payload) == 0 {
		return nil, nil
	}
	var decoded metricResponse
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil, fmt.Errorf("decode dynatrace metric response: %w", err)
	}
	var samples []MetricSample
	for _, result := range decoded.Result {
		for _, datum := range result.Data {
			service := pickService(datum.DimensionMap, datum.Dimensions)
			for index, value := range datum.Values {
				timestamp := time.Time{}
				if index < len(datum.Timestamps) {
					timestamp = time.UnixMilli(datum.Timestamps[index]).UTC()
				}
				samples = append(samples, MetricSample{
					MetricID: result.MetricID,
					Service:  service,
					Value:    value,
					Unit:     result.Unit,
					Observed: timestamp,
				})
			}
		}
	}
	return samples, nil
}

func pickService(dimensionMap map[string]string, dimensions []string) string {
	for _, key := range []string{"dt.entity.service.name", "dt.entity.service", "service.name", "service"} {
		if value, ok := dimensionMap[key]; ok && value != "" {
			return value
		}
	}
	if len(dimensions) > 0 {
		return dimensions[len(dimensions)-1]
	}
	return ""
}

func singleSlash(left, right string) string {
	if left == "" {
		return right
	}
	if right == "" {
		return left
	}
	if leftEndsWithSlash(left) && rightStartsWithSlash(right) {
		return left + right[1:]
	}
	if !leftEndsWithSlash(left) && !rightStartsWithSlash(right) {
		return left + "/" + right
	}
	return left + right
}

func leftEndsWithSlash(value string) bool    { return len(value) > 0 && value[len(value)-1] == '/' }
func rightStartsWithSlash(value string) bool { return len(value) > 0 && value[0] == '/' }

func indexByte(value string, target byte) int {
	for index := 0; index < len(value); index++ {
		if value[index] == target {
			return index
		}
	}
	return -1
}

// ErrUnauthorized indicates the supplied API token was rejected.
var ErrUnauthorized = errors.New("dynatrace api token unauthorized")
