package fake

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// AnalysisRepository stores analyses in memory for local execution and deterministic tests.
type AnalysisRepository struct {
	mu       sync.RWMutex
	analyses map[string]domain.AnalysisResult
	messages map[string][]domain.ChatMessage
	nextID   int64
}

// NewAnalysisRepository creates an in-memory analysis repository.
func NewAnalysisRepository() *AnalysisRepository {
	return &AnalysisRepository{
		analyses: make(map[string]domain.AnalysisResult),
		messages: make(map[string][]domain.ChatMessage),
	}
}

// Save stores an analysis result by identifier.
func (repository *AnalysisRepository) Save(_ context.Context, result domain.AnalysisResult) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()

	repository.analyses[result.ID] = result
	return nil
}

// Find returns an analysis result by identifier.
func (repository *AnalysisRepository) Find(_ context.Context, id string) (domain.AnalysisResult, error) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()

	result, ok := repository.analyses[id]
	if !ok {
		return domain.AnalysisResult{}, fmt.Errorf("%w: %s", domain.ErrAnalysisNotFound, id)
	}

	return result, nil
}

// SaveExchange stores a user question and assistant answer for an analysis.
func (repository *AnalysisRepository) SaveExchange(_ context.Context, question domain.ChatMessage, answer domain.ChatMessage) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()

	repository.nextID++
	question.ID = strconv.FormatInt(repository.nextID, 10)
	if question.CreatedAt.IsZero() {
		question.CreatedAt = time.Now().UTC()
	}

	repository.nextID++
	answer.ID = strconv.FormatInt(repository.nextID, 10)
	if answer.CreatedAt.IsZero() {
		answer.CreatedAt = time.Now().UTC()
	}

	repository.messages[question.AnalysisID] = append(repository.messages[question.AnalysisID], question, answer)

	return nil
}

// List returns persisted chat messages for an analysis.
func (repository *AnalysisRepository) List(_ context.Context, analysisID string) ([]domain.ChatMessage, error) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()

	messages := repository.messages[analysisID]
	copied := make([]domain.ChatMessage, len(messages))
	copy(copied, messages)

	return copied, nil
}
