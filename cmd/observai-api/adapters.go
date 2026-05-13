package main

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/guferreira1/observai-api/internal/adapters/outbound/fake"
	"github.com/guferreira1/observai-api/internal/adapters/outbound/postgres"
	redisadapter "github.com/guferreira1/observai-api/internal/adapters/outbound/redis"
	"github.com/guferreira1/observai-api/internal/core/ports"
	"github.com/guferreira1/observai-api/internal/platform/config"
	"github.com/guferreira1/observai-api/internal/platform/observability"
)

type analysisStore struct {
	repository  ports.AnalysisRepository
	chatHistory ports.ChatHistoryRepository
	close       func()
	postgres    *postgres.AnalysisRepository
}

type analysisContextCache struct {
	contexts ports.AnalysisContextCache
	close    func()
	redis    *redisadapter.AnalysisContextCache
}

func newAnalysisStore(cfg config.Config, log *slog.Logger, observer observability.ProviderObserver) analysisStore {
	fakeRepository := fake.NewAnalysisRepository()
	databaseDSN := strings.TrimSpace(cfg.DatabaseDSN)
	if databaseDSN == "" {
		log.Warn("postgres repository disabled; using in-memory analysis repository")
		return analysisStore{
			repository:  fakeRepository,
			chatHistory: fakeRepository,
			close:       func() {},
		}
	}

	databaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	postgresRepository, err := postgres.NewAnalysisRepository(databaseCtx, databaseDSN, postgres.RepositoryOptions{Observer: observer})
	cancel()
	if err != nil {
		log.Error("postgres repository initialization failed", "error", err)
		os.Exit(1)
	}

	log.Info("postgres repository enabled")
	return analysisStore{
		repository:  postgresRepository,
		chatHistory: postgresRepository,
		close:       postgresRepository.Close,
		postgres:    postgresRepository,
	}
}

func newAnalysisContextCache(cfg config.Config, log *slog.Logger, observer observability.ProviderObserver) analysisContextCache {
	redisURL := strings.TrimSpace(cfg.RedisURL)
	if redisURL == "" {
		log.Warn("redis analysis context cache disabled; using in-memory cache")
		return analysisContextCache{
			contexts: fake.NewAnalysisContextCache(),
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
