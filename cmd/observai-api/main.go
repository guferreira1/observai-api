package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	inboundhttp "github.com/guferreira1/observai-api/internal/adapters/inbound/http"
	"github.com/guferreira1/observai-api/internal/adapters/outbound/credentials"
	"github.com/guferreira1/observai-api/internal/adapters/outbound/factory"
	"github.com/guferreira1/observai-api/internal/adapters/outbound/providertest"
	uuidadapter "github.com/guferreira1/observai-api/internal/adapters/outbound/uuid"
	"github.com/guferreira1/observai-api/internal/core/usecase"
	"github.com/guferreira1/observai-api/internal/platform/config"
	"github.com/guferreira1/observai-api/internal/platform/crypto"
	"github.com/guferreira1/observai-api/internal/platform/dbmigrate"
	"github.com/guferreira1/observai-api/internal/platform/health"
	"github.com/guferreira1/observai-api/internal/platform/logger"
	"github.com/guferreira1/observai-api/internal/platform/server"
	"github.com/guferreira1/observai-api/internal/platform/telemetry"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := run(); err != nil {
		_, writeErr := fmt.Fprintln(os.Stderr, err)
		if writeErr != nil {
			slog.Error("failed to print startup error", "error", writeErr)
		}
		os.Exit(1)
	}
}

func run() error {
	cfg, timeLocation, err := loadRuntimeConfig()
	if err != nil {
		return err
	}

	log := logger.New(cfg.Env)

	if err := runMigrationsOnStartup(cfg, log); err != nil {
		return err
	}

	metrics, providerMetrics, metricsHandler, tracer, err := bootstrapObservability(cfg, log)
	if err != nil {
		return reportAndReturn(log, "tracer initialization failed", err)
	}

	store, cache, queue := bootstrapStorage(cfg, log, providerMetrics)
	defer store.close()
	defer cache.close()
	defer queue.close()

	ids := uuidadapter.NewIDGenerator()

	providerConfigUseCase, llmConfigUseCase, err := buildProviderUseCases(cfg, log, store, ids)
	if err != nil {
		return reportAndReturn(log, "provider configuration initialization failed", err)
	}

	credentialStore := credentials.NewDispatcher()
	providers, registries, capabilities, loadedRuntimeProviderConfig, err := buildProviders(
		cfg,
		log,
		providerMetrics,
		credentialStore,
		providerConfigUseCase,
		llmConfigUseCase,
	)
	if err != nil {
		return reportAndReturn(log, "provider initialization failed", err)
	}

	analysisUseCase := buildAnalysisUseCase(cfg, store, cache, ids, queue, registries)
	chatUseCase := buildChatUseCase(cfg, store, cache, ids, queue, registries)
	traceUseCase := buildTraceUseCase(store, registries)
	apiKeyUseCase := buildAPIKeyUseCase(store, ids)

	jwtSigner, authUseCase, userUseCase, err := buildAuthUseCases(cfg, log, store, ids)
	if err != nil {
		return reportAndReturn(log, "user authentication initialization failed", err)
	}

	webhookUseCase := buildWebhookUseCase(store, ids)
	auditLogUseCase := buildAuditLogUseCase(store)
	retentionUseCase := buildRetentionUseCase(store)

	if webhookUseCase != nil {
		analysisUseCase.WithCompletionNotifier(usecase.NewWebhookNotifier(webhookUseCase, log))
	}

	configureProviderReload(
		cfg,
		log,
		providerMetrics,
		store,
		credentialStore,
		providerConfigUseCase,
		llmConfigUseCase,
		registries,
		capabilities,
		loadedRuntimeProviderConfig,
	)

	setupUseCase := buildSetupUseCase(cfg, store, userUseCase, auditLogUseCase)

	scheduler, schedulerErr := newSchedulerIfEnabled(cfg, log, retentionUseCase, webhookUseCase)
	if schedulerErr != nil {
		return reportAndReturn(log, "scheduler initialization failed", schedulerErr)
	}
	if scheduler != nil {
		if err := scheduler.Start(); err != nil {
			return reportAndReturn(log, "scheduler start failed", err)
		}
		defer scheduler.Stop()
	}

	checker := buildReadinessChecker(store, cache, providers, providerConfigUseCase, llmConfigUseCase)
	router := buildHTTPRouter(
		cfg,
		timeLocation,
		log,
		analysisUseCase,
		chatUseCase,
		store,
		capabilities,
		checker,
		apiKeyUseCase,
		metricsHandler,
		providerConfigUseCase,
		llmConfigUseCase,
		jwtSigner,
		authUseCase,
		userUseCase,
		setupUseCase,
		traceUseCase,
		webhookUseCase,
		auditLogUseCase,
		retentionUseCase,
	)
	handler := metrics.Middleware(telemetry.WrapHandler("observai-api", router))
	srv := server.New(cfg, handler)

	if err := runHTTPServer(cfg, log, srv, analysisUseCase, queue, tracer); err != nil {
		return reportAndReturn(log, "server stopped with error", err)
	}

	return nil
}

func reportAndReturn(log *slog.Logger, msg string, err error) error {
	log.Error(msg, "error", err)
	return err
}

func loadRuntimeConfig() (config.Config, *time.Location, error) {
	cfg, err := config.Load()
	if err != nil {
		return config.Config{}, nil, err
	}

	timeLocation, err := cfg.TimeLocation()
	if err != nil {
		return config.Config{}, nil, err
	}

	return cfg, timeLocation, nil
}

func runMigrationsOnStartup(cfg config.Config, log *slog.Logger) error {
	if !cfg.MigrateOnStart {
		return nil
	}

	if err := dbmigrate.Run(dbmigrate.Options{
		DatabaseDSN:   cfg.DatabaseDSN,
		MigrationsDir: cfg.MigrationsDir,
		Logger:        log,
	}); err != nil {
		log.Error("database migrations failed", "error", err)
		return err
	}

	return nil
}

func bootstrapObservability(cfg config.Config, log *slog.Logger) (*telemetry.HTTPMetrics, *telemetry.ProviderMetrics, http.Handler, *telemetry.Tracer, error) {
	metrics := telemetry.NewHTTPMetrics()
	providerMetrics := telemetry.NewProviderMetrics(metrics.Registry())
	_ = telemetry.NewQueueCacheMetrics(metrics.Registry())
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
		return nil, nil, nil, nil, err
	}

	if cfg.Tracing.Endpoint != "" {
		log.Info("otel tracing enabled", "endpoint", cfg.Tracing.Endpoint, "sampleRatio", cfg.Tracing.SampleRatio)
	}

	return metrics, providerMetrics, metricsHandler, tracer, nil
}

func bootstrapStorage(cfg config.Config, log *slog.Logger, providerMetrics *telemetry.ProviderMetrics) (analysisStore, analysisContextCache, analysisQueue) {
	return newAnalysisStore(cfg, log, providerMetrics), newAnalysisContextCache(cfg, log, providerMetrics), newAnalysisQueue(cfg, log, providerMetrics)
}

func buildProviderUseCases(cfg config.Config, log *slog.Logger, store analysisStore, ids *uuidadapter.IDGenerator) (*usecase.ProviderConfig, *usecase.LLMConfig, error) {
	var providerConfigUseCase *usecase.ProviderConfig
	var llmConfigUseCase *usecase.LLMConfig

	if store.providerConfigs != nil && store.llmConfigs != nil {
		cipher, cipherErr := buildEncryptionCipher(cfg, log)
		if cipherErr != nil {
			log.Error("encryption cipher initialization failed", "error", cipherErr)
			return nil, nil, cipherErr
		}

		tester := providertest.New()
		providerConfigUseCase = usecase.NewProviderConfig(store.providerConfigs, cipher, tester, factory.NewObservabilityRegistry(), ids)
		llmConfigUseCase = usecase.NewLLMConfig(store.llmConfigs, cipher, tester, factory.NewLLMRegistry(), ids)
	}

	return providerConfigUseCase, llmConfigUseCase, nil
}

func buildProviders(
	cfg config.Config,
	log *slog.Logger,
	providerMetrics *telemetry.ProviderMetrics,
	credentialStore *credentials.Dispatcher,
	providerConfigUseCase *usecase.ProviderConfig,
	llmConfigUseCase *usecase.LLMConfig,
) (providers, adapterRegistries, *capabilitiesStore, bool, error) {
	runtimeProviderCtx, runtimeProviderCancel := context.WithTimeout(context.Background(), 5*time.Second)
	runtimeProviderConfig, loadedRuntimeProviderConfig := initialRuntimeProviderConfig(runtimeProviderCtx, cfg, log, providerConfigUseCase, llmConfigUseCase)
	runtimeProviderCancel()

	providers, err := newProviders(runtimeProviderConfig, log, providerMetrics, credentialStore)
	if err != nil && loadedRuntimeProviderConfig {
		log.Warn("database-backed provider initialization failed; starting with bootstrap provider configuration", "error", err)
		loadedRuntimeProviderConfig = false
		providers, err = newProviders(cfg, log, providerMetrics, credentialStore)
	}
	if err != nil {
		return providers, adapterRegistries{}, nil, false, err
	}

	registries := newAdapterRegistries(providers)
	capabilities := newCapabilitiesStore(buildCapabilities(cfg, providers, version))

	return providers, registries, capabilities, loadedRuntimeProviderConfig, nil
}

func buildAnalysisUseCase(
	cfg config.Config,
	store analysisStore,
	cache analysisContextCache,
	ids *uuidadapter.IDGenerator,
	queue analysisQueue,
	registries adapterRegistries,
) *usecase.Analysis {
	return usecase.NewAnalysis(
		registries.collector,
		registries.generator,
		store.repository,
		cache.contexts,
		cfg.AnalysisContextCacheTTL,
		ids,
	).WithAsyncBackend(store.jobRepository, queue.enqueuer).WithRedaction(buildRedactionChain(cfg))
}

func buildChatUseCase(
	cfg config.Config,
	store analysisStore,
	cache analysisContextCache,
	ids *uuidadapter.IDGenerator,
	queue analysisQueue,
	registries adapterRegistries,
) *usecase.Chat {
	return usecase.NewChat(
		store.repository,
		cache.contexts,
		cfg.AnalysisContextCacheTTL,
		store.chatHistory,
		registries.responder,
	).WithLocker(queue.locker).WithFeedbackRepository(store.chatFeedback)
}

func buildTraceUseCase(store analysisStore, registries adapterRegistries) *usecase.Trace {
	return usecase.NewTrace(store.repository, registries.traces)
}

func buildAPIKeyUseCase(store analysisStore, ids *uuidadapter.IDGenerator) *usecase.APIKey {
	if store.apiKeys == nil {
		return nil
	}
	return usecase.NewAPIKey(store.apiKeys, ids)
}

func buildAuthUseCases(
	cfg config.Config,
	log *slog.Logger,
	store analysisStore,
	ids *uuidadapter.IDGenerator,
) (*crypto.JWTSigner, *usecase.Auth, *usecase.User, error) {
	var jwtSigner *crypto.JWTSigner
	var authUseCase *usecase.Auth
	var userUseCase *usecase.User

	if jwtSecret := cfg.JWT.Secret; jwtSecret != "" && store.users != nil && store.refreshTokens != nil {
		signer, err := crypto.NewJWTSigner([]byte(jwtSecret), cfg.JWT.Issuer)
		if err != nil {
			log.Error("jwt signer initialization failed", "error", err)
			return nil, nil, nil, err
		}
		jwtSigner = signer
		hasher := crypto.NewBcryptPasswordHasher(0)
		authUseCase = usecase.NewAuth(store.users, store.refreshTokens, jwtSigner, hasher, ids, usecase.AuthOptions{
			AccessTokenTTL:  cfg.JWT.AccessTokenTTL,
			RefreshTokenTTL: cfg.JWT.RefreshTokenTTL,
		})
		userUseCase = usecase.NewUser(store.users, store.refreshTokens, hasher, ids)
		return jwtSigner, authUseCase, userUseCase, nil
	}

	if cfg.Mode != config.ModeLocal {
		err := errors.New("user authentication requires database, jwt secret and user repository wiring")
		log.Error(err.Error())
		return nil, nil, nil, err
	}

	if jwtSecret := cfg.JWT.Secret; jwtSecret == "" {
		log.Warn("jwt secret not configured; user authentication endpoints disabled")
	}

	return nil, nil, nil, nil
}

func buildWebhookUseCase(store analysisStore, ids *uuidadapter.IDGenerator) *usecase.WebhookSubscriptions {
	if store.webhooks == nil {
		return nil
	}

	return usecase.NewWebhookSubscriptions(store.webhooks, store.webhookDeliveries, store.webhookDispatcher, ids)
}

func buildAuditLogUseCase(store analysisStore) *usecase.AuditLog {
	if store.auditLog == nil {
		return nil
	}

	return usecase.NewAuditLog(store.auditLog)
}

func buildRetentionUseCase(store analysisStore) *usecase.AnalysisRetention {
	if store.retention == nil {
		return nil
	}

	return usecase.NewAnalysisRetention(store.retention)
}

func configureProviderReload(
	cfg config.Config,
	log *slog.Logger,
	providerMetrics *telemetry.ProviderMetrics,
	store analysisStore,
	credentialStore *credentials.Dispatcher,
	providerConfigUseCase *usecase.ProviderConfig,
	llmConfigUseCase *usecase.LLMConfig,
	registries adapterRegistries,
	capabilities *capabilitiesStore,
	loadedRuntimeProviderConfig bool,
) {
	if store.providerConfigs == nil || store.llmConfigs == nil {
		return
	}

	reloadDeps := factoryDependencies(cfg, log, providerMetrics, credentialStore)
	reload := newAdapterReloader(cfg, log, reloadDeps, registries, capabilities, providerConfigUseCase, llmConfigUseCase)
	providerConfigUseCase.WithReloadHook(reload)
	llmConfigUseCase.WithReloadHook(reload)
	if !loadedRuntimeProviderConfig {
		reload(context.Background())
	}
}

func buildSetupUseCase(
	cfg config.Config,
	store analysisStore,
	userUseCase *usecase.User,
	auditLogUseCase *usecase.AuditLog,
) *usecase.Setup {
	if userUseCase == nil || store.users == nil {
		return nil
	}

	inventory := providerInventoryFromStores(store, cfg)
	setupUseCase := usecase.NewSetup(store.users, userUseCase, inventory)
	if auditLogUseCase != nil {
		setupUseCase.WithAuditLog(auditLogUseCase)
	}

	return setupUseCase
}

func buildReadinessChecker(
	store analysisStore,
	cache analysisContextCache,
	providers providers,
	providerConfigUseCase *usecase.ProviderConfig,
	llmConfigUseCase *usecase.LLMConfig,
) *health.Checker {
	return health.NewChecker(10*time.Second, buildHealthProbes(store, cache, providers, providerConfigUseCase, llmConfigUseCase)...)
}

func buildHTTPRouter(
	cfg config.Config,
	timeLocation *time.Location,
	log *slog.Logger,
	analysisUseCase *usecase.Analysis,
	chatUseCase *usecase.Chat,
	store analysisStore,
	capabilities *capabilitiesStore,
	checker *health.Checker,
	apiKeyUseCase *usecase.APIKey,
	metricsHandler http.Handler,
	providerConfigUseCase *usecase.ProviderConfig,
	llmConfigUseCase *usecase.LLMConfig,
	jwtSigner *crypto.JWTSigner,
	authUseCase *usecase.Auth,
	userUseCase *usecase.User,
	setupUseCase *usecase.Setup,
	traceUseCase *usecase.Trace,
	webhookUseCase *usecase.WebhookSubscriptions,
	auditLogUseCase *usecase.AuditLog,
	retentionUseCase *usecase.AnalysisRetention,
) http.Handler {
	return inboundhttp.NewRouter(analysisUseCase, chatUseCase, inboundhttp.RouterOptions{
		Logger:             log,
		RequestTimeout:     cfg.HTTPRequestTimeout,
		MaxRequestBodyByte: cfg.HTTPMaxBodyBytes,
		RateLimit: inboundhttp.RateLimitConfig{
			RequestsPerSecond: cfg.HTTPRateLimit.RequestsPerSecond,
			Burst:             cfg.HTTPRateLimit.Burst,
		},
		Auth: inboundhttp.AuthConfig{
			StaticKeys: cfg.HTTPAuth.Keys,
			AdminKeys:  cfg.HTTPAuth.AdminKeys,
			Skip:       cfg.HTTPAuth.Skip,
			APIKeys:    apiKeyUseCase,
			Signer:     jwtSigner,
			Users:      store.users,
		},
		Cookies: inboundhttp.CookieConfig{
			Domain: cfg.Cookies.Domain,
			Secure: cfg.Cookies.Secure,
		},
		TimeLocation:     timeLocation,
		Sessions:         authUseCase,
		Users:            userUseCase,
		Setup:            setupUseCase,
		ProviderConfigs:  providerConfigUseCase,
		LLMConfigs:       llmConfigUseCase,
		Metrics:          metricsHandler,
		ReadinessChecker: checker,
		Capabilities:     capabilities.Get(),
		CapabilitiesFunc: capabilities.Get,
		RetentionPolicy: inboundhttp.RetentionPolicyOptions{
			Days:     cfg.Scheduler.RetentionDays,
			Quantity: cfg.Scheduler.RetentionQuantity,
			Cron:     cfg.Scheduler.RetentionCron,
		},
		Provider:     capabilities.ProviderSummary(),
		ProviderFunc: capabilities.ProviderSummary,
		Trace:        traceUseCase,
		APIKeys:      apiKeyUseCase,
		Webhooks:     webhookUseCase,
		AuditLog:     auditLogUseCase,
		Retention:    retentionUseCase,
	})
}

func runHTTPServer(
	cfg config.Config,
	log *slog.Logger,
	srv *http.Server,
	analysisUseCase *usecase.Analysis,
	queue analysisQueue,
	tracer *telemetry.Tracer,
) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	workerCtx, workerCancel := context.WithCancel(context.Background())
	var workerWaitGroup sync.WaitGroup
	workerWaitGroup.Add(1)
	go func() {
		defer workerWaitGroup.Done()
		log.Info("analysis worker starting", "concurrency", cfg.Queue.Concurrency)
		queue.start(workerCtx, analysisUseCase.RunAnalysisJob, func(jobID string, runErr error) {
			log.Error("analysis worker job failed", "jobId", jobID, "error", runErr)
		})
		log.Info("analysis worker stopped")
	}()

	serverErrors := make(chan error, 1)
	go func() {
		log.Info("starting observai api", "address", srv.Addr)
		serverErrors <- srv.ListenAndServe()
	}()

	shutdown := func(reason string) {
		shutdownTimeout := cfg.HTTPShutdownTimeout
		if shutdownTimeout <= 0 {
			shutdownTimeout = 30 * time.Second
		}

		log.Info("shutdown sequence", "reason", reason, "timeout", shutdownTimeout)

		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Error("server shutdown failed", "error", err)
			_ = srv.Close()
		}

		workerCancel()
		workerWaitGroup.Wait()

		if err := tracer.Shutdown(shutdownCtx); err != nil {
			log.Error("tracer shutdown failed", "error", err)
		}
	}

	select {
	case <-ctx.Done():
		shutdown("signal")
		log.Info("server stopped")
	case err := <-serverErrors:
		shutdown("server error")
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}

	return nil
}
