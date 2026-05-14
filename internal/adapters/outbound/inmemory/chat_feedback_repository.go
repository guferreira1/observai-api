package inmemory

import (
	"context"
	"sync"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// ChatFeedbackRepository stores chat feedback in memory.
//
// Repeated submissions for the same (analysisID, messageID) overwrite the
// previous entry so frontends can toggle the user's choice.
type ChatFeedbackRepository struct {
	mu        sync.RWMutex
	feedbacks map[string]domain.ChatFeedback
}

// NewChatFeedbackRepository creates an in-memory chat feedback repository.
func NewChatFeedbackRepository() *ChatFeedbackRepository {
	return &ChatFeedbackRepository{feedbacks: make(map[string]domain.ChatFeedback)}
}

// SaveFeedback persists or replaces feedback for a single message.
func (repository *ChatFeedbackRepository) SaveFeedback(_ context.Context, feedback domain.ChatFeedback) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()

	repository.feedbacks[feedback.AnalysisID+"/"+feedback.MessageID] = feedback
	return nil
}

// Find returns the persisted feedback for a single message. Used by tests.
func (repository *ChatFeedbackRepository) Find(analysisID string, messageID string) (domain.ChatFeedback, bool) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()

	feedback, ok := repository.feedbacks[analysisID+"/"+messageID]
	return feedback, ok
}
