// Package otel implements a trace provider compatible with OTLP-JSON
// backends such as Grafana Tempo, Honeycomb (refinery proxy) and any
// service that exposes OpenTelemetry's HTTP/JSON tracing surface.
//
// The adapter targets GET /api/traces/{traceID} returning the standard
// OTLP-JSON encoding (resource_spans -> scope_spans -> spans).
package otel

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/platform/observability"
	"github.com/guferreira1/observai-api/internal/platform/retry"
)

// ClientOptions configures the OTLP-JSON trace client.
type ClientOptions struct {
	BaseURL     string
	Timeout     time.Duration
	Observer    observability.ProviderObserver
	RetryPolicy retry.Policy
}

// Client talks to an OTLP-JSON trace backend.
type Client struct {
	baseURL     *url.URL
	httpClient  *http.Client
	observer    observability.ProviderObserver
	retryPolicy retry.Policy
}

// NewClient validates the options and returns a ready-to-use client.
func NewClient(opts ClientOptions) (*Client, error) {
	parsed, err := url.Parse(strings.TrimSpace(opts.BaseURL))
	if err != nil {
		return nil, fmt.Errorf("parse otel trace base url: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("otel trace base url must include scheme and host")
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
		httpClient:  &http.Client{Timeout: timeout},
		observer:    observer,
		retryPolicy: retryPolicy,
	}, nil
}

// TraceProvider implements ports.TraceProvider for OTLP-JSON backends.
type TraceProvider struct {
	client *Client
}

// NewTraceProvider builds a TraceProvider over the supplied client.
func NewTraceProvider(client *Client) *TraceProvider {
	return &TraceProvider{client: client}
}

// FetchSpans retrieves the trace identified by traceID and converts the
// OTLP-JSON payload into provider-agnostic domain.Span values.
func (provider *TraceProvider) FetchSpans(ctx context.Context, traceID string) ([]domain.Span, error) {
	cleaned := strings.TrimSpace(traceID)
	if cleaned == "" {
		return nil, fmt.Errorf("otel trace fetch requires a trace id")
	}

	endpoint := *provider.client.baseURL
	endpoint.Path = joinPath(endpoint.Path, "/api/traces/"+url.PathEscape(cleaned))

	var raw []byte
	err := retry.Do(ctx, provider.client.retryPolicy, isRetryable, func(int) error {
		body, attemptErr := provider.client.fetch(ctx, endpoint.String())
		if attemptErr != nil {
			return attemptErr
		}
		raw = body
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("fetch otel trace: %w", err)
	}

	return decodeOTLPTrace(raw)
}

func (client *Client) fetch(ctx context.Context, endpoint string) ([]byte, error) {
	startedAt := time.Now()
	var resultErr error
	defer func() { client.observer.Observe("otel", "fetch_trace", time.Since(startedAt), resultErr) }()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		resultErr = err
		return nil, fmt.Errorf("build otel trace request: %w", err)
	}
	request.Header.Set("Accept", "application/json")

	response, err := client.httpClient.Do(request)
	if err != nil {
		resultErr = err
		return nil, &transientError{err: fmt.Errorf("call otel backend: %w", err)}
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		resultErr = err
		return nil, &transientError{err: fmt.Errorf("read otel response: %w", err)}
	}

	if response.StatusCode == http.StatusNotFound {
		resultErr = fmt.Errorf("trace not found")
		return nil, resultErr
	}
	if response.StatusCode >= http.StatusInternalServerError {
		err := fmt.Errorf("otel backend status %d: %s", response.StatusCode, truncate(string(body), 200))
		resultErr = err
		return nil, &transientError{err: err}
	}
	if response.StatusCode >= http.StatusBadRequest {
		err := fmt.Errorf("otel backend status %d: %s", response.StatusCode, truncate(string(body), 200))
		resultErr = err
		return nil, err
	}
	return body, nil
}

// Ping checks reachability via /api/echo, which Tempo exposes for health probes.
func (client *Client) Ping(ctx context.Context) (err error) {
	startedAt := time.Now()
	defer func() { client.observer.Observe("otel", "ping", time.Since(startedAt), err) }()

	endpoint := *client.baseURL
	endpoint.Path = joinPath(endpoint.Path, "/api/echo")

	request, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if reqErr != nil {
		return fmt.Errorf("build otel health request: %w", reqErr)
	}
	response, doErr := client.httpClient.Do(request)
	if doErr != nil {
		return fmt.Errorf("call otel health: %w", doErr)
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("otel health returned status %d", response.StatusCode)
	}
	return nil
}

type otlpResponse struct {
	Batches []struct {
		Resource struct {
			Attributes []otlpKeyValue `json:"attributes"`
		} `json:"resource"`
		ScopeSpans []struct {
			Spans []otlpSpan `json:"spans"`
		} `json:"scopeSpans"`
	} `json:"batches"`
	ResourceSpans []struct {
		Resource struct {
			Attributes []otlpKeyValue `json:"attributes"`
		} `json:"resource"`
		ScopeSpans []struct {
			Spans []otlpSpan `json:"spans"`
		} `json:"scopeSpans"`
	} `json:"resourceSpans"`
}

type otlpSpan struct {
	TraceID           string         `json:"traceId"`
	SpanID            string         `json:"spanId"`
	ParentSpanID      string         `json:"parentSpanId"`
	Name              string         `json:"name"`
	StartTimeUnixNano string         `json:"startTimeUnixNano"`
	EndTimeUnixNano   string         `json:"endTimeUnixNano"`
	Status            otlpStatus     `json:"status"`
	Attributes        []otlpKeyValue `json:"attributes"`
}

type otlpStatus struct {
	Code int `json:"code"`
}

type otlpKeyValue struct {
	Key   string `json:"key"`
	Value struct {
		StringValue string      `json:"stringValue,omitempty"`
		IntValue    json.Number `json:"intValue,omitempty"`
		DoubleValue float64     `json:"doubleValue,omitempty"`
		BoolValue   bool        `json:"boolValue,omitempty"`
	} `json:"value"`
}

func decodeOTLPTrace(body []byte) ([]domain.Span, error) {
	var parsed otlpResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("decode otel trace: %w", err)
	}

	type resourceGroup struct {
		service string
		spans   []otlpSpan
	}
	groups := make([]resourceGroup, 0)
	collect := func(attributes []otlpKeyValue, scopeSpans []otlpSpan) {
		groups = append(groups, resourceGroup{
			service: extractServiceName(attributes),
			spans:   scopeSpans,
		})
	}
	for _, batch := range parsed.Batches {
		for _, scope := range batch.ScopeSpans {
			collect(batch.Resource.Attributes, scope.Spans)
		}
	}
	for _, batch := range parsed.ResourceSpans {
		for _, scope := range batch.ScopeSpans {
			collect(batch.Resource.Attributes, scope.Spans)
		}
	}

	durationsByID := map[string]int64{}
	for _, group := range groups {
		for _, span := range group.spans {
			start, _ := strconv.ParseInt(span.StartTimeUnixNano, 10, 64)
			end, _ := strconv.ParseInt(span.EndTimeUnixNano, 10, 64)
			durationsByID[span.SpanID] = end - start
		}
	}
	childDurations := map[string]int64{}
	for _, group := range groups {
		for _, span := range group.spans {
			if span.ParentSpanID == "" {
				continue
			}
			childDurations[span.ParentSpanID] += durationsByID[span.SpanID]
		}
	}

	out := make([]domain.Span, 0)
	for _, group := range groups {
		for _, raw := range group.spans {
			startNanos, _ := strconv.ParseInt(raw.StartTimeUnixNano, 10, 64)
			durationNanos := durationsByID[raw.SpanID]
			durationMs := float64(durationNanos) / 1_000_000.0
			selfMs := durationMs
			if child := childDurations[raw.SpanID]; child > 0 {
				selfMs = (float64(durationNanos) - float64(child)) / 1_000_000.0
				if selfMs < 0 {
					selfMs = 0
				}
			}
			out = append(out, domain.Span{
				TraceID:      decodeHexID(raw.TraceID),
				SpanID:       decodeHexID(raw.SpanID),
				ParentSpanID: decodeHexID(raw.ParentSpanID),
				Service:      group.service,
				Operation:    raw.Name,
				StartTime:    time.Unix(0, startNanos).UTC(),
				DurationMs:   durationMs,
				SelfTimeMs:   selfMs,
				Status:       statusFromOTLP(raw.Status.Code),
				Attributes:   attributesToMap(raw.Attributes),
			})
		}
	}
	return out, nil
}

func extractServiceName(attributes []otlpKeyValue) string {
	for _, attribute := range attributes {
		if attribute.Key == "service.name" {
			return attribute.Value.StringValue
		}
	}
	return ""
}

func attributesToMap(attributes []otlpKeyValue) map[string]string {
	if len(attributes) == 0 {
		return nil
	}
	out := make(map[string]string, len(attributes))
	for _, attribute := range attributes {
		switch {
		case attribute.Value.StringValue != "":
			out[attribute.Key] = attribute.Value.StringValue
		case attribute.Value.IntValue != "":
			out[attribute.Key] = attribute.Value.IntValue.String()
		case attribute.Value.DoubleValue != 0:
			out[attribute.Key] = strconv.FormatFloat(attribute.Value.DoubleValue, 'f', -1, 64)
		case attribute.Value.BoolValue:
			out[attribute.Key] = "true"
		}
	}
	return out
}

func statusFromOTLP(code int) domain.SpanStatus {
	switch code {
	case 2:
		return domain.SpanStatusError
	case 1:
		return domain.SpanStatusOk
	default:
		return domain.SpanStatusUnset
	}
}

func decodeHexID(value string) string {
	if value == "" {
		return ""
	}
	if _, err := hex.DecodeString(value); err == nil {
		return value
	}
	return value
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
