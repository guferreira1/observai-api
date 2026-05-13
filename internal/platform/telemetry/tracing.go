package telemetry

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

// TracerOptions configures the OpenTelemetry tracer.
//
// Endpoint accepts plain host:port (e.g. otel-collector:4318) or a full URL
// (https://otel.example.com). When empty, NewTracer returns a no-op provider so
// the API can run without an OTLP collector available.
type TracerOptions struct {
	ServiceName    string
	ServiceVersion string
	Endpoint       string
	Insecure       bool
	SampleRatio    float64
	Timeout        time.Duration
}

// Tracer holds an OpenTelemetry tracer provider and a shutdown hook.
type Tracer struct {
	provider trace.TracerProvider
	shutdown func(context.Context) error
}

// NewTracer constructs and registers an OpenTelemetry tracer provider.
//
// When opts.Endpoint is empty the function installs a no-op provider and a
// shutdown hook that returns nil, so the rest of the application code can stay
// unchanged regardless of whether tracing is actually enabled.
func NewTracer(ctx context.Context, opts TracerOptions) (*Tracer, error) {
	endpoint := strings.TrimSpace(opts.Endpoint)
	if endpoint == "" {
		provider := tracenoop.NewTracerProvider()
		otel.SetTracerProvider(provider)
		otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		))
		return &Tracer{provider: provider, shutdown: func(context.Context) error { return nil }}, nil
	}

	exporter, err := newOTLPTraceExporter(ctx, endpoint, opts.Insecure, opts.Timeout)
	if err != nil {
		return nil, fmt.Errorf("init otlp trace exporter: %w", err)
	}

	res, err := resource.Merge(resource.Default(), resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(opts.ServiceName),
		semconv.ServiceVersion(opts.ServiceVersion),
	))
	if err != nil {
		return nil, fmt.Errorf("build otel resource: %w", err)
	}

	sampler := sdktrace.ParentBased(sdktrace.TraceIDRatioBased(clampRatio(opts.SampleRatio)))
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return &Tracer{provider: provider, shutdown: provider.Shutdown}, nil
}

// Shutdown drains pending spans and releases exporter resources.
func (tracer *Tracer) Shutdown(ctx context.Context) error {
	if tracer == nil || tracer.shutdown == nil {
		return nil
	}
	return tracer.shutdown(ctx)
}

// WrapHandler instruments an HTTP handler with OpenTelemetry server-side tracing.
func WrapHandler(operation string, handler http.Handler) http.Handler {
	return otelhttp.NewHandler(handler, operation)
}

func newOTLPTraceExporter(ctx context.Context, endpoint string, insecure bool, timeout time.Duration) (*otlptrace.Exporter, error) {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	host, urlPath, schemeFromURL, err := parseOTLPEndpoint(endpoint)
	if err != nil {
		return nil, err
	}

	options := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(host),
		otlptracehttp.WithTimeout(timeout),
	}
	if urlPath != "" {
		options = append(options, otlptracehttp.WithURLPath(urlPath))
	}

	useInsecure := insecure
	if schemeFromURL == "http" {
		useInsecure = true
	}
	if schemeFromURL == "https" {
		useInsecure = false
	}
	if useInsecure {
		options = append(options, otlptracehttp.WithInsecure())
	}

	return otlptracehttp.New(ctx, options...)
}

func parseOTLPEndpoint(raw string) (host string, urlPath string, scheme string, err error) {
	if !strings.Contains(raw, "://") {
		return raw, "", "", nil
	}
	parsed, parseErr := url.Parse(raw)
	if parseErr != nil {
		return "", "", "", fmt.Errorf("parse otlp endpoint %q: %w", raw, parseErr)
	}
	if parsed.Host == "" {
		return "", "", "", fmt.Errorf("otlp endpoint must include host: %s", raw)
	}
	return parsed.Host, parsed.Path, parsed.Scheme, nil
}

func clampRatio(ratio float64) float64 {
	if ratio < 0 {
		return 0
	}
	if ratio > 1 {
		return 1
	}
	return ratio
}
