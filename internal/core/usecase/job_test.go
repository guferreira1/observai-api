package usecase

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/guferreira1/observai-api/internal/adapters/outbound/inmemory"
	"github.com/guferreira1/observai-api/internal/adapters/outbound/testfakes"
	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingEnqueuer struct {
	mu     sync.Mutex
	jobIDs []string
}

func (enqueuer *recordingEnqueuer) EnqueueAnalysis(_ context.Context, jobID string) error {
	enqueuer.mu.Lock()
	defer enqueuer.mu.Unlock()
	enqueuer.jobIDs = append(enqueuer.jobIDs, jobID)
	return nil
}

func TestAnalysisSubmitAndRun(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repository := inmemory.NewAnalysisRepository()
	jobs := inmemory.NewAnalysisJobRepository()
	enqueuer := &recordingEnqueuer{}

	useCase := NewAnalysis(
		testfakes.NewSignalCollector(),
		testfakes.NewAnalysisGenerator(),
		repository,
		inmemory.NewAnalysisContextCache(),
		6*time.Hour,
		testfakes.NewIDGenerator("analysis"),
	).WithAsyncBackend(jobs, enqueuer)

	submitted, err := useCase.SubmitAnalysis(ctx, domain.AnalysisRequest{
		Goal: "investigate checkout latency",
		TimeWindow: domain.TimeWindow{
			Start: time.Date(2026, 5, 12, 10, 0, 0, 0, time.UTC),
			End:   time.Date(2026, 5, 12, 11, 0, 0, 0, time.UTC),
		},
		AffectedServices: []string{"checkout-service"},
		Signals:          []domain.SignalType{domain.SignalLogs, domain.SignalMetrics, domain.SignalTraces},
	})
	require.NoError(t, err)
	assert.Equal(t, domain.JobStatusPending, submitted.Status)
	assert.Equal(t, []string{submitted.ID}, enqueuer.jobIDs)

	require.NoError(t, useCase.RunAnalysisJob(ctx, submitted.ID))

	finished, err := useCase.GetJob(ctx, submitted.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.JobStatusCompleted, finished.Status)
	assert.NotEmpty(t, finished.AnalysisID)
	require.NotNil(t, finished.FinishedAt)

	stored, err := repository.Find(ctx, finished.AnalysisID)
	require.NoError(t, err)
	assert.Equal(t, finished.AnalysisID, stored.ID)
}

func TestAnalysisSubmitRejectsInvalidRequest(t *testing.T) {
	t.Parallel()

	useCase := NewAnalysis(
		testfakes.NewSignalCollector(),
		testfakes.NewAnalysisGenerator(),
		inmemory.NewAnalysisRepository(),
		inmemory.NewAnalysisContextCache(),
		6*time.Hour,
		testfakes.NewIDGenerator("analysis"),
	).WithAsyncBackend(inmemory.NewAnalysisJobRepository(), &recordingEnqueuer{})

	_, err := useCase.SubmitAnalysis(context.Background(), domain.AnalysisRequest{})
	assert.True(t, errors.Is(err, domain.ErrInvalidAnalysisRequest))
}

type failingGenerator struct{}

func (failingGenerator) Generate(_ context.Context, _ domain.AnalysisRequest, _ []domain.Evidence) (domain.AnalysisResult, error) {
	return domain.AnalysisResult{}, errors.New("llm provider unavailable")
}

func TestAnalysisRunMarksFailedOnGeneratorError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	jobs := inmemory.NewAnalysisJobRepository()
	useCase := NewAnalysis(
		testfakes.NewSignalCollector(),
		failingGenerator{},
		inmemory.NewAnalysisRepository(),
		inmemory.NewAnalysisContextCache(),
		6*time.Hour,
		testfakes.NewIDGenerator("analysis"),
	).WithAsyncBackend(jobs, &recordingEnqueuer{})

	submitted, err := useCase.SubmitAnalysis(ctx, domain.AnalysisRequest{
		Goal: "investigate checkout latency",
		TimeWindow: domain.TimeWindow{
			Start: time.Date(2026, 5, 12, 10, 0, 0, 0, time.UTC),
			End:   time.Date(2026, 5, 12, 11, 0, 0, 0, time.UTC),
		},
		AffectedServices: []string{"checkout-service"},
		Signals:          []domain.SignalType{domain.SignalLogs},
	})
	require.NoError(t, err)

	runErr := useCase.RunAnalysisJob(ctx, submitted.ID)
	require.Error(t, runErr)

	job, err := useCase.GetJob(ctx, submitted.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.JobStatusFailed, job.Status)
	assert.Contains(t, job.ErrorMessage, "llm provider unavailable")
}

type orderedChatResponder struct {
	hits          int32
	concurrent    int32
	maxConcurrent int32
	delay         time.Duration
}

func (responder *orderedChatResponder) Answer(_ context.Context, _ domain.AnalysisContext, question domain.ChatQuestion) (domain.ChatAnswer, error) {
	current := atomic.AddInt32(&responder.concurrent, 1)
	for {
		previous := atomic.LoadInt32(&responder.maxConcurrent)
		if current <= previous {
			break
		}
		if atomic.CompareAndSwapInt32(&responder.maxConcurrent, previous, current) {
			break
		}
	}

	time.Sleep(responder.delay)
	atomic.AddInt32(&responder.hits, 1)
	atomic.AddInt32(&responder.concurrent, -1)

	return domain.ChatAnswer{AnalysisID: question.AnalysisID, Answer: question.Question}, nil
}

func TestChatSerializesConcurrentQuestionsForSameAnalysis(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repository := inmemory.NewAnalysisRepository()
	require.NoError(t, repository.Save(ctx, domain.AnalysisResult{
		ID:      "analysis-000001",
		Summary: "checkout-service latency increased",
	}))

	responder := &orderedChatResponder{delay: 20 * time.Millisecond}
	useCase := NewChat(repository, inmemory.NewAnalysisContextCache(), 6*time.Hour, repository, responder).
		WithLocker(inmemory.NewAnalysisLocker())

	const total = 8
	var waitGroup sync.WaitGroup
	waitGroup.Add(total)
	for index := 0; index < total; index++ {
		go func() {
			defer waitGroup.Done()
			_, err := useCase.Ask(ctx, domain.ChatQuestion{
				AnalysisID: "analysis-000001",
				Question:   "Which evidence supports this analysis?",
			})
			assert.NoError(t, err)
		}()
	}
	waitGroup.Wait()

	assert.Equal(t, int32(total), atomic.LoadInt32(&responder.hits))
	assert.Equal(t, int32(1), atomic.LoadInt32(&responder.maxConcurrent), "responder must observe at most one in-flight call per analysis")
}

func TestChatRunsDifferentAnalysesInParallel(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repository := inmemory.NewAnalysisRepository()
	require.NoError(t, repository.Save(ctx, domain.AnalysisResult{ID: "analysis-a"}))
	require.NoError(t, repository.Save(ctx, domain.AnalysisResult{ID: "analysis-b"}))

	responder := &orderedChatResponder{delay: 50 * time.Millisecond}
	useCase := NewChat(repository, inmemory.NewAnalysisContextCache(), 6*time.Hour, repository, responder).
		WithLocker(inmemory.NewAnalysisLocker())

	var waitGroup sync.WaitGroup
	waitGroup.Add(2)

	go func() {
		defer waitGroup.Done()
		_, err := useCase.Ask(ctx, domain.ChatQuestion{AnalysisID: "analysis-a", Question: "Which evidence supports this analysis?"})
		assert.NoError(t, err)
	}()
	go func() {
		defer waitGroup.Done()
		_, err := useCase.Ask(ctx, domain.ChatQuestion{AnalysisID: "analysis-b", Question: "Which evidence supports this analysis?"})
		assert.NoError(t, err)
	}()

	waitGroup.Wait()
	assert.Equal(t, int32(2), atomic.LoadInt32(&responder.maxConcurrent), "different analyses must run concurrently")
}
