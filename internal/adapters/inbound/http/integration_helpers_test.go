//go:build integration

package http_test

import (
	"context"
	"io"
	"log/slog"
	stdhttp "net/http"
	"net/http/httptest"
	"testing"
	"time"

	inboundhttp "github.com/guferreira1/observai-api/internal/adapters/inbound/http"
	"github.com/guferreira1/observai-api/internal/adapters/outbound/fake"
	"github.com/guferreira1/observai-api/internal/adapters/outbound/postgres"
	"github.com/guferreira1/observai-api/internal/core/usecase"
	"github.com/stretchr/testify/require"
)

const integrationAnalysisContextCacheTTL = 6 * time.Hour

func buildIntegrationServer(ctx context.Context, t *testing.T, dsn string) *httptest.Server {
	t.Helper()

	repository, err := postgres.NewAnalysisRepository(ctx, dsn)
	require.NoError(t, err, "open postgres analysis repository")
	t.Cleanup(repository.Close)

	jobRepository := postgres.NewAnalysisJobRepository(repository.Pool())
	enqueuer := fake.NewSynchronousJobEnqueuer()
	analysisUseCase := usecase.NewAnalysis(
		fake.NewSignalCollector(),
		fake.NewAnalysisGenerator(),
		repository,
		fake.NewAnalysisContextCache(),
		integrationAnalysisContextCacheTTL,
		fake.NewIDGenerator("analysis"),
	).WithAsyncBackend(jobRepository, enqueuer)
	enqueuer.SetHandler(analysisUseCase.RunAnalysisJob)

	chatUseCase := usecase.NewChat(
		repository,
		fake.NewAnalysisContextCache(),
		integrationAnalysisContextCacheTTL,
		repository,
		fake.NewChatResponder(),
	).WithLocker(fake.NewAnalysisLocker())

	router := inboundhttp.NewRouter(analysisUseCase, chatUseCase, inboundhttp.RouterOptions{
		Logger:             slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout:     10 * time.Second,
		MaxRequestBodyByte: 1 << 20,
		Metrics: stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, _ *stdhttp.Request) {
			writer.WriteHeader(stdhttp.StatusOK)
		}),
	})

	return httptest.NewServer(router)
}
