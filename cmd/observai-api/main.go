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
	"github.com/guferreira1/observai-api/internal/platform/logger"
	"github.com/guferreira1/observai-api/internal/platform/server"
	"github.com/guferreira1/observai-api/internal/platform/telemetry"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}

	log := logger.New(cfg.Env)
	metrics := telemetry.NewHTTPMetrics()
	metricsHandler := promhttp.HandlerFor(metrics.Registry(), promhttp.HandlerOpts{})

	store := newAnalysisStore(cfg, log)
	defer store.close()

	cache := newAnalysisContextCache(cfg, log)
	defer cache.close()

	ids := fake.NewIDGenerator("analysis")

	analysisUseCase := usecase.NewAnalysis(
		fake.NewSignalCollector(),
		fake.NewAnalysisGenerator(),
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
		fake.NewChatResponder(),
	)

	router := inboundhttp.NewRouter(analysisUseCase, chatUseCase, metricsHandler)
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

		log.Info("server stopped")
	case err := <-serverErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server stopped with error", "error", err)
			os.Exit(1)
		}
	}
}
