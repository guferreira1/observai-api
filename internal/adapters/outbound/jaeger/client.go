// Package jaeger implements a read-only Jaeger v1 HTTP trace provider.
//
// The adapter targets the legacy /api/traces/{traceID} endpoint exposed by
// the Jaeger query service. The Jaeger v2 backend keeps the same path
// shape, so this code works against both. OTLP-native backends are
// covered by the sibling otel adapter.
package jaeger

import (
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

// ClientOptions configures the Jaeger HTTP client.
type ClientOptions struct {
	BaseURL     string
	Timeout     time.Duration
	Observer    observability.ProviderObserver
	RetryPolicy retry.Policy
}

// Client talks to the Jaeger query HTTP API.
type Client struct {
	baseURL     *url.URL
	httpClient  *http.Client
	observer    observability.ProviderObserver
	retryPolicy retry.Policy
}

// NewClient validates the supplied options and returns a ready-to-use client.
func NewClient(opts ClientOptions) (*Client, error) {
	parsed, err := url.Parse(strings.TrimSpace(opts.BaseURL))
	if err != nil {
		return nil, fmt.Errorf("parse jaeger base url: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("jaeger base url must include scheme and host")
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

// Span is the normalized trace span returned by Jaeger.
type Span struct {
	TraceID       string
	SpanID        string
	ParentSpanID  string
	ProcessID     string
	OperationName string
	StartTimeUS   int64
	DurationUS    int64
	Tags          map[string]string
}

// Trace is the response payload Jaeger returns under data[0].
type Trace struct {
	TraceID   string
	Spans     []Span
	Processes map[string]Process
}

// Process is a Jaeger process (service entry).
type Process struct {
	ServiceName string
	Tags        map[string]string
}

type tracesResponse struct {
	Data []struct {
		TraceID   string                `json:"traceID"`
		Spans     []rawSpan             `json:"spans"`
		Processes map[string]rawProcess `json:"processes"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"msg"`
	} `json:"errors"`
}

type rawSpan struct {
	TraceID       string         `json:"traceID"`
	SpanID        string         `json:"spanID"`
	OperationName string         `json:"operationName"`
	References    []rawReference `json:"references"`
	StartTime     int64          `json:"startTime"`
	Duration      int64          `json:"duration"`
	Tags          []rawKeyValue  `json:"tags"`
	ProcessID     string         `json:"processID"`
}

type rawReference struct {
	RefType string `json:"refType"`
	TraceID string `json:"traceID"`
	SpanID  string `json:"spanID"`
}

type rawKeyValue struct {
	Key   string      `json:"key"`
	Type  string      `json:"type"`
	Value interface{} `json:"value"`
}

type rawProcess struct {
	ServiceName string        `json:"serviceName"`
	Tags        []rawKeyValue `json:"tags"`
}

// Ping verifies Jaeger query reachability via the services endpoint.
func (client *Client) Ping(ctx context.Context) (err error) {
	startedAt := time.Now()
	defer func() { client.observer.Observe("jaeger", "ping", time.Since(startedAt), err) }()

	endpoint := *client.baseURL
	endpoint.Path = joinPath(endpoint.Path, "/api/services")

	request, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if reqErr != nil {
		return fmt.Errorf("build jaeger health request: %w", reqErr)
	}
	response, doErr := client.httpClient.Do(request)
	if doErr != nil {
		return fmt.Errorf("call jaeger health: %w", doErr)
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("jaeger health returned status %d", response.StatusCode)
	}
	return nil
}

// FetchTrace returns the trace identified by traceID.
func (client *Client) FetchTrace(ctx context.Context, traceID string) (trace Trace, err error) {
	startedAt := time.Now()
	defer func() { client.observer.Observe("jaeger", "fetch_trace", time.Since(startedAt), err) }()

	cleaned := strings.TrimSpace(traceID)
	if cleaned == "" {
		return Trace{}, fmt.Errorf("jaeger fetch requires a trace id")
	}

	endpoint := *client.baseURL
	endpoint.Path = joinPath(endpoint.Path, "/api/traces/"+url.PathEscape(cleaned))

	err = retry.Do(ctx, client.retryPolicy, isRetryable, func(int) error {
		fetched, attemptErr := client.executeFetch(ctx, endpoint.String())
		if attemptErr != nil {
			return attemptErr
		}
		trace = fetched
		return nil
	})
	return trace, err
}

func (client *Client) executeFetch(ctx context.Context, endpoint string) (Trace, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Trace{}, fmt.Errorf("build jaeger request: %w", err)
	}
	request.Header.Set("Accept", "application/json")

	response, err := client.httpClient.Do(request)
	if err != nil {
		return Trace{}, &transientError{err: fmt.Errorf("call jaeger: %w", err)}
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return Trace{}, &transientError{err: fmt.Errorf("read jaeger response: %w", err)}
	}

	if response.StatusCode == http.StatusNotFound {
		return Trace{}, fmt.Errorf("jaeger trace not found")
	}
	if response.StatusCode >= http.StatusInternalServerError {
		return Trace{}, &transientError{err: fmt.Errorf("jaeger status %d: %s", response.StatusCode, truncate(string(body), 200))}
	}
	if response.StatusCode >= http.StatusBadRequest {
		return Trace{}, fmt.Errorf("jaeger status %d: %s", response.StatusCode, truncate(string(body), 200))
	}

	var parsed tracesResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return Trace{}, fmt.Errorf("decode jaeger response: %w", err)
	}
	if len(parsed.Errors) > 0 {
		return Trace{}, fmt.Errorf("jaeger error: %s", parsed.Errors[0].Message)
	}
	if len(parsed.Data) == 0 {
		return Trace{}, fmt.Errorf("jaeger returned no traces")
	}

	entry := parsed.Data[0]
	trace := Trace{
		TraceID:   entry.TraceID,
		Spans:     make([]Span, 0, len(entry.Spans)),
		Processes: make(map[string]Process, len(entry.Processes)),
	}
	for processID, process := range entry.Processes {
		trace.Processes[processID] = Process{
			ServiceName: process.ServiceName,
			Tags:        keyValueMap(process.Tags),
		}
	}
	for _, raw := range entry.Spans {
		trace.Spans = append(trace.Spans, Span{
			TraceID:       raw.TraceID,
			SpanID:        raw.SpanID,
			ParentSpanID:  parentSpanID(raw.References),
			ProcessID:     raw.ProcessID,
			OperationName: raw.OperationName,
			StartTimeUS:   raw.StartTime,
			DurationUS:    raw.Duration,
			Tags:          keyValueMap(raw.Tags),
		})
	}
	return trace, nil
}

func parentSpanID(references []rawReference) string {
	for _, reference := range references {
		if strings.EqualFold(reference.RefType, "CHILD_OF") {
			return reference.SpanID
		}
	}
	return ""
}

func keyValueMap(pairs []rawKeyValue) map[string]string {
	if len(pairs) == 0 {
		return nil
	}
	out := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		if pair.Value == nil {
			continue
		}
		out[pair.Key] = fmt.Sprintf("%v", pair.Value)
	}
	return out
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
