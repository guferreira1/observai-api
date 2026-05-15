package usecase

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/guferreira1/observai-api/internal/adapters/outbound/inmemory"
	"github.com/guferreira1/observai-api/internal/adapters/outbound/testfakes"
	"github.com/guferreira1/observai-api/internal/core/domain"
)

func TestChatConcurrencyParallelAnalysesIndependent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repository := inmemory.NewAnalysisRepository()
	for _, id := range []string{"analysis-a", "analysis-b", "analysis-c"} {
		if err := repository.Save(ctx, domain.AnalysisResult{
			ID:       id,
			Summary:  "incident " + id,
			Evidence: []domain.Evidence{{ID: "ev_" + id, Name: "p95"}},
		}); err != nil {
			t.Fatalf("seed analysis: %v", err)
		}
	}

	useCase := NewChat(repository, inmemory.NewAnalysisContextCache(), 6*time.Hour, repository, testfakes.NewChatResponder()).
		WithLocker(inmemory.NewAnalysisLocker())

	var wg sync.WaitGroup
	errs := make(chan error, 3)
	for _, analysisID := range []string{"analysis-a", "analysis-b", "analysis-c"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			if _, err := useCase.Ask(ctx, domain.ChatQuestion{AnalysisID: id, Question: "Which evidence supports this analysis?"}); err != nil {
				errs <- err
			}
		}(analysisID)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent Ask failed: %v", err)
	}
}

func TestChatConcurrencySameAnalysisSerializedByLocker(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repository := inmemory.NewAnalysisRepository()
	if err := repository.Save(ctx, domain.AnalysisResult{
		ID:       "analysis-1",
		Summary:  "single",
		Evidence: []domain.Evidence{{ID: "ev_1", Name: "p95"}},
	}); err != nil {
		t.Fatalf("seed analysis: %v", err)
	}

	useCase := NewChat(repository, inmemory.NewAnalysisContextCache(), 6*time.Hour, repository, testfakes.NewChatResponder()).
		WithLocker(inmemory.NewAnalysisLocker())

	var wg sync.WaitGroup
	const goroutines = 8
	errs := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := useCase.Ask(ctx, domain.ChatQuestion{AnalysisID: "analysis-1", Question: "Which evidence supports this analysis?"}); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("serialized Ask failed: %v", err)
	}
	messages, err := useCase.History(ctx, "analysis-1", domain.ChatHistoryFilter{})
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	// 2 messages per question (user + assistant) × 8 questions = 16
	if len(messages) != goroutines*2 {
		t.Fatalf("expected %d messages, got %d", goroutines*2, len(messages))
	}
}
