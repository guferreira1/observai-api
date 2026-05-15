package main

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"

	asynqadapter "github.com/guferreira1/observai-api/internal/adapters/outbound/asynq"
	"github.com/guferreira1/observai-api/internal/adapters/outbound/inmemory"
	"github.com/guferreira1/observai-api/internal/adapters/outbound/postgres"
	redisadapter "github.com/guferreira1/observai-api/internal/adapters/outbound/redis"
	"github.com/guferreira1/observai-api/internal/adapters/outbound/webhooks"
	"github.com/guferreira1/observai-api/internal/core/ports"
	"github.com/guferreira1/observai-api/internal/platform/config"
	"github.com/guferreira1/observai-api/internal/platform/observability"
	redisclient "github.com/redis/go-redis/v9"
)

type analysisStore struct {
	repository        ports.AnalysisRepository
	chatHistory       ports.ChatHistoryRepository
	jobRepository     ports.AnalysisJobRepository
	chatFeedback      ports.ChatFeedbackRepository
	apiKeys           ports.APIKeyRepository
	webhooks          ports.WebhookRepository
	webhookDispatcher ports.WebhookDispatcher
	auditLog          ports.AuditLogRepository
	retention         ports.AnalysisRetention
	users             ports.UserRepository
	refreshTokens     ports.RefreshTokenRepository
	providerConfigs   ports.ProviderConfigRepository
	llmConfigs        ports.LLMConfigRepository
	webhookDeliveries ports.WebhookDeliveryRepository
	close             func()
	postgres          *postgres.AnalysisRepository
}

type analysisContextCache struct {
	contexts ports.AnalysisContextCache
	close    func()
	redis    *redisadapter.AnalysisContextCache
}

type analysisQueue struct {
	enqueuer ports.JobEnqueuer
	locker   ports.AnalysisLocker
	start    func(ctx context.Context, handler func(context.Context, string) error, reportError func(jobID string, err error))
	close    func()
	redis    *redisLockerHandle
}

type redisLockerHandle struct {
	locker *redisadapter.AnalysisLocker
}

func newAnalysisStore(cfg config.Config, log *slog.Logger, observer observability.ProviderObserver) analysisStore {
	inMemoryRepository := inmemory.NewAnalysisRepository()
	inMemoryJobs := inmemory.NewAnalysisJobRepository()
	inMemoryFeedback := inmemory.NewChatFeedbackRepository()
	inMemoryUsers := inmemory.NewUserRepository()
	inMemoryRefresh := inmemory.NewRefreshTokenRepository()
	databaseDSN := strings.TrimSpace(cfg.DatabaseDSN)
	if databaseDSN == "" {
		log.Warn("postgres repository disabled; using in-memory analysis repository")
		return analysisStore{
			repository:    inMemoryRepository,
			chatHistory:   inMemoryRepository,
			jobRepository: inMemoryJobs,
			chatFeedback:  inMemoryFeedback,
			users:         inMemoryUsers,
			refreshTokens: inMemoryRefresh,
			close:         func() {},
		}
	}

	databaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	postgresRepository, err := postgres.NewAnalysisRepository(databaseCtx, databaseDSN, postgres.RepositoryOptions{Observer: observer})
	cancel()
	if err != nil {
		log.Error("postgres repository initialization failed", "error", err)
		os.Exit(1)
	}

	jobRepository := postgres.NewAnalysisJobRepository(postgresRepository.Pool(), postgres.RepositoryOptions{Observer: observer})
	chatFeedbackRepository := postgres.NewChatFeedbackRepository(postgresRepository.Pool(), postgres.RepositoryOptions{Observer: observer})
	apiKeyRepository := postgres.NewAPIKeyRepository(postgresRepository.Pool(), postgres.RepositoryOptions{Observer: observer})
	webhookRepository := postgres.NewWebhookRepository(postgresRepository.Pool(), postgres.RepositoryOptions{Observer: observer})
	auditLogRepository := postgres.NewAuditLogRepository(postgresRepository.Pool(), postgres.RepositoryOptions{Observer: observer})
	retentionRepository := postgres.NewAnalysisRetentionRepository(postgresRepository.Pool(), postgres.RepositoryOptions{Observer: observer})
	userRepository := postgres.NewUserRepository(postgresRepository.Pool(), postgres.RepositoryOptions{Observer: observer})
	refreshTokenRepository := postgres.NewRefreshTokenRepository(postgresRepository.Pool(), postgres.RepositoryOptions{Observer: observer})
	providerConfigRepository := postgres.NewProviderConfigRepository(postgresRepository.Pool(), postgres.RepositoryOptions{Observer: observer})
	llmConfigRepository := postgres.NewLLMConfigRepository(postgresRepository.Pool(), postgres.RepositoryOptions{Observer: observer})
	webhookDeliveryRepository := postgres.NewWebhookDeliveryRepository(postgresRepository.Pool(), postgres.RepositoryOptions{Observer: observer})
	webhookDispatcher := webhooks.NewDispatcher(webhooks.DispatcherOptions{Logger: log, Observer: observer, Deliveries: webhookDeliveryRepository})

	log.Info("postgres repository enabled")
	return analysisStore{
		repository:        postgresRepository,
		chatHistory:       postgresRepository,
		jobRepository:     jobRepository,
		chatFeedback:      chatFeedbackRepository,
		apiKeys:           apiKeyRepository,
		webhooks:          webhookRepository,
		webhookDispatcher: webhookDispatcher,
		auditLog:          auditLogRepository,
		retention:         retentionRepository,
		users:             userRepository,
		refreshTokens:     refreshTokenRepository,
		providerConfigs:   providerConfigRepository,
		llmConfigs:        llmConfigRepository,
		webhookDeliveries: webhookDeliveryRepository,
		close:             postgresRepository.Close,
		postgres:          postgresRepository,
	}
}

func newAnalysisContextCache(cfg config.Config, log *slog.Logger, observer observability.ProviderObserver) analysisContextCache {
	redisURL := strings.TrimSpace(cfg.RedisURL)
	if redisURL == "" {
		log.Warn("redis analysis context cache disabled; using in-memory cache")
		return analysisContextCache{
			contexts: inmemory.NewAnalysisContextCache(),
			close:    func() {},
		}
	}

	redisCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	redisCache, err := redisadapter.NewAnalysisContextCache(redisCtx, redisURL, redisadapter.CacheOptions{Observer: observer})
	cancel()
	if err != nil {
		log.Error("redis analysis context cache initialization failed", "error", err)
		os.Exit(1)
	}

	log.Info("redis analysis context cache enabled", "ttl", cfg.AnalysisContextCacheTTL.String())
	return analysisContextCache{
		contexts: redisCache,
		close: func() {
			if err := redisCache.Close(); err != nil {
				log.Error("redis analysis context cache close failed", "error", err)
			}
		},
		redis: redisCache,
	}
}

func newAnalysisQueue(cfg config.Config, log *slog.Logger, observer observability.ProviderObserver) analysisQueue {
	redisURL := strings.TrimSpace(cfg.RedisURL)
	if redisURL == "" {
		log.Warn("redis queue disabled; using in-memory analysis queue and locker")
		return buildInMemoryQueue(cfg)
	}

	redisCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	options, err := redisclient.ParseURL(redisURL)
	if err != nil {
		log.Error("parse redis url for queue failed", "error", err)
		os.Exit(1)
	}
	client := redisclient.NewClient(options)
	if err := client.Ping(redisCtx).Err(); err != nil {
		log.Error("ping redis for queue failed", "error", err)
		os.Exit(1)
	}

	locker, err := redisadapter.NewAnalysisLocker(redisCtx, redisURL, redisadapter.LockerOptions{
		TTL:      cfg.Queue.ChatLockTTL,
		Wait:     cfg.Queue.ChatLockWait,
		Observer: observer,
	})
	if err != nil {
		log.Error("redis analysis locker initialization failed", "error", err)
		os.Exit(1)
	}

	if strings.EqualFold(strings.TrimSpace(cfg.Queue.Backend), "asynq") {
		asynqEnqueuer, err := asynqadapter.NewAnalysisEnqueuer(asynqadapter.EnqueuerOptions{
			RedisURL: redisURL,
			Observer: observer,
		})
		if err != nil {
			log.Error("asynq enqueuer initialization failed", "error", err)
			os.Exit(1)
		}
		asynqWorker, err := asynqadapter.NewAnalysisWorker(asynqadapter.WorkerOptions{
			RedisURL:    redisURL,
			Concurrency: cfg.Queue.Concurrency,
			Logger:      log,
		})
		if err != nil {
			log.Error("asynq worker initialization failed", "error", err)
			os.Exit(1)
		}
		log.Info("asynq analysis queue enabled", "concurrency", cfg.Queue.Concurrency)
		return analysisQueue{
			enqueuer: asynqEnqueuer,
			locker:   locker,
			start:    asynqWorker.Start,
			close: func() {
				asynqWorker.Stop()
				if err := asynqEnqueuer.Close(); err != nil {
					log.Error("asynq enqueuer close failed", "error", err)
				}
				if err := locker.Close(); err != nil {
					log.Error("redis analysis locker close failed", "error", err)
				}
				if err := client.Close(); err != nil {
					log.Error("redis queue client close failed", "error", err)
				}
			},
			redis: &redisLockerHandle{locker: locker},
		}
	}

	enqueuer := redisadapter.NewJobEnqueuer(client, redisadapter.EnqueuerOptions{Observer: observer})
	worker := redisadapter.NewQueueWorker(client, redisadapter.WorkerOptions{
		Concurrency:    cfg.Queue.Concurrency,
		DequeueTimeout: cfg.Queue.DequeueTimeout,
		Observer:       observer,
	})

	log.Info("redis analysis queue enabled", "concurrency", cfg.Queue.Concurrency)
	return analysisQueue{
		enqueuer: enqueuer,
		locker:   locker,
		start:    worker.Start,
		close: func() {
			if err := locker.Close(); err != nil {
				log.Error("redis analysis locker close failed", "error", err)
			}
			if err := client.Close(); err != nil {
				log.Error("redis queue client close failed", "error", err)
			}
		},
		redis: &redisLockerHandle{locker: locker},
	}
}

func buildInMemoryQueue(cfg config.Config) analysisQueue {
	queue := inmemory.NewAnalysisQueue(cfg.Queue.Concurrency * 8)
	enqueuer := inmemory.NewJobEnqueuer(queue)
	worker := inmemory.NewQueueWorker(queue, cfg.Queue.Concurrency)
	locker := inmemory.NewAnalysisLocker()
	return analysisQueue{
		enqueuer: enqueuer,
		locker:   locker,
		start:    worker.Start,
		close:    queue.Close,
	}
}
