package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	inboundhttp "github.com/guferreira1/observai-api/internal/adapters/inbound/http"
	"github.com/guferreira1/observai-api/internal/adapters/outbound/fake"
	"github.com/guferreira1/observai-api/internal/core/usecase"
	"github.com/guferreira1/observai-api/internal/platform/config"
	"github.com/guferreira1/observai-api/internal/platform/health"
	"github.com/guferreira1/observai-api/internal/platform/logger"
	"github.com/guferreira1/observai-api/internal/platform/server"
	"github.com/guferreira1/observai-api/internal/platform/telemetry"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	cfg, err := config.Load()
	if err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}

	log := logger.New(cfg.Env)
	metrics := telemetry.NewHTTPMetrics()
	providerMetrics := telemetry.NewProviderMetrics(metrics.Registry())
	metricsHandler := promhttp.HandlerFor(metrics.Registry(), promhttp.HandlerOpts{})

	tracerCtx, tracerCancel := context.WithTimeout(context.Background(), 10*time.Second)
	tracer, err := telemetry.NewTracer(tracerCtx, telemetry.TracerOptions{
		ServiceName:    cfg.Tracing.ServiceName,
		ServiceVersion: version,
		Endpoint:       cfg.Tracing.Endpoint,
		Insecure:       cfg.Tracing.Insecure,
		SampleRatio:    cfg.Tracing.SampleRatio,
		Timeout:        cfg.Tracing.Timeout,
	})
	tracerCancel()
	if err != nil {
		log.Error("tracer initialization failed", "error", err)
		os.Exit(1)
	}
	if cfg.Tracing.Endpoint != "" {
		log.Info("otel tracing enabled", "endpoint", cfg.Tracing.Endpoint, "sampleRatio", cfg.Tracing.SampleRatio)
	}

	store := newAnalysisStore(cfg, log, providerMetrics)
	defer store.close()

	cache := newAnalysisContextCache(cfg, log, providerMetrics)
	defer cache.close()

	ids := fake.NewIDGenerator("analysis")

	providers, err := newProviders(cfg, log, providerMetrics)
	if err != nil {
		log.Error("provider initialization failed", "error", err)
		os.Exit(1)
	}

	analysisUseCase := usecase.NewAnalysis(
		providers.collector,
		providers.generator,
		store.repository,
		cache.contexts,
		cfg.AnalysisContextCacheTTL,
		ids,
	)
	chatUseCase := usecase.NewChat(
		store.repository,
		cache.contexts,
		cfg.AnalysisContextCacheTTL,
		store.chatHistory,
		providers.responder,
	)

	checker := health.NewChecker(2*time.Second, buildHealthProbes(store, cache, providers)...)

	router := inboundhttp.NewRouter(analysisUseCase, chatUseCase, inboundhttp.RouterOptions{
		Logger:             log,
		RequestTimeout:     cfg.HTTPRequestTimeout,
		MaxRequestBodyByte: cfg.HTTPMaxBodyBytes,
		RateLimit: inboundhttp.RateLimitConfig{
			RequestsPerSecond: cfg.HTTPRateLimit.RequestsPerSecond,
			Burst:             cfg.HTTPRateLimit.Burst,
		},
		Metrics:   metricsHandler,
		Liveness:  health.LivenessHandler(),
		Readiness: health.ReadinessHandler(checker),
	})
	handler := metrics.Middleware(telemetry.WrapHandler("observai-api", router))
	srv := server.New(cfg, handler)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serverErrors := make(chan error, 1)
	go func() {
		log.Info("starting observai api", "address", srv.Addr)
		serverErrors <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Error("server shutdown failed", "error", err)
			os.Exit(1)
		}

		if err := tracer.Shutdown(shutdownCtx); err != nil {
			log.Error("tracer shutdown failed", "error", err)
		}

		log.Info("server stopped")
	case err := <-serverErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server stopped with error", "error", err)
			os.Exit(1)
		}
	}
}
