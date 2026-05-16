package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/core/ports"
)

// Chat answers follow-up questions about an active analysis.
type Chat struct {
	repository ports.AnalysisRepository
	cache      ports.AnalysisContextCache
	cacheTTL   time.Duration
	history    ports.ChatHistoryRepository
	responder  ports.ChatResponder
	locker     ports.AnalysisLocker
	feedback   ports.ChatFeedbackRepository
	scope      chatScopePolicy
	now        func() time.Time
}

// NewChat creates a scoped analysis chat use case.
func NewChat(
	repository ports.AnalysisRepository,
	cache ports.AnalysisContextCache,
	cacheTTL time.Duration,
	history ports.ChatHistoryRepository,
	responder ports.ChatResponder,
) *Chat {
	if cache == nil {
		cache = noOpAnalysisContextCache{}
	}
	if history == nil {
		history = noOpChatHistoryRepository{}
	}

	return &Chat{
		repository: repository,
		cache:      cache,
		cacheTTL:   cacheTTL,
		history:    history,
		responder:  responder,
		locker:     noOpAnalysisLocker{},
		scope:      defaultChatScopePolicy(),
		now:        time.Now,
	}
}

// WithLocker configures the analysis-scoped locker used to serialize concurrent
// questions about the same analysis. When omitted, concurrent calls run freely.
func (useCase *Chat) WithLocker(locker ports.AnalysisLocker) *Chat {
	if locker == nil {
		locker = noOpAnalysisLocker{}
	}
	useCase.locker = locker
	return useCase
}

// WithFeedbackRepository attaches the repository used to persist user feedback.
func (useCase *Chat) WithFeedbackRepository(feedback ports.ChatFeedbackRepository) *Chat {
	useCase.feedback = feedback
	return useCase
}

// SubmitFeedback persists feedback for a previously delivered assistant message.
//
// The analysis must exist; the message must belong to the same analysis and be
// an assistant message. Repeated calls overwrite the previous feedback so
// frontends can toggle the user's choice without creating duplicates.
func (useCase *Chat) SubmitFeedback(ctx context.Context, feedback domain.ChatFeedback) error {
	feedback.AnalysisID = strings.TrimSpace(feedback.AnalysisID)
	feedback.MessageID = strings.TrimSpace(feedback.MessageID)
	if feedback.AnalysisID == "" {
		return fmt.Errorf("%w: analysis id is required", domain.ErrAnalysisNotFound)
	}
	if feedback.MessageID == "" {
		return fmt.Errorf("%w: message id is required", domain.ErrChatMessageNotFound)
	}

	if _, err := useCase.repository.Find(ctx, feedback.AnalysisID); err != nil {
		return fmt.Errorf("find analysis: %w", err)
	}

	if !useCase.messageBelongsToAnalysis(ctx, feedback.AnalysisID, feedback.MessageID) {
		return fmt.Errorf("%w: %s", domain.ErrChatMessageNotFound, feedback.MessageID)
	}

	if useCase.feedback == nil {
		return errors.New("chat feedback repository not configured")
	}

	if feedback.CreatedAt.IsZero() {
		feedback.CreatedAt = useCase.now().UTC()
	}
	if err := useCase.feedback.SaveFeedback(ctx, feedback); err != nil {
		return fmt.Errorf("save chat feedback: %w", err)
	}
	return nil
}

func (useCase *Chat) messageBelongsToAnalysis(ctx context.Context, analysisID string, messageID string) bool {
	messages, err := useCase.history.List(ctx, analysisID, domain.ChatHistoryFilter{Limit: maxChatHistoryLimit})
	if err != nil {
		return false
	}
	for _, message := range messages {
		if message.ID == messageID && message.Role == domain.ChatRoleAssistant {
			return true
		}
	}
	return false
}

// Ask answers a question about the active analysis or returns a scoped refusal.
//
// Concurrent calls for the same analysis identifier are serialized through the
// configured locker so chat exchanges remain in order.
func (useCase *Chat) Ask(ctx context.Context, question domain.ChatQuestion) (domain.ChatAnswer, error) {
	question.AnalysisID = strings.TrimSpace(question.AnalysisID)
	question.Question = strings.TrimSpace(question.Question)
	if question.AnalysisID == "" {
		return domain.ChatAnswer{}, fmt.Errorf("%w: analysis id is required", domain.ErrAnalysisNotFound)
	}
	if question.Question == "" {
		return domain.ChatAnswer{}, fmt.Errorf("%w: question is required", domain.ErrInvalidChatQuestion)
	}

	analysisContext, err := useCase.findAnalysisContext(ctx, question.AnalysisID)
	if err != nil {
		return domain.ChatAnswer{}, err
	}

	release, err := useCase.locker.Acquire(ctx, question.AnalysisID)
	if err != nil {
		return domain.ChatAnswer{}, fmt.Errorf("acquire analysis chat lock: %w", err)
	}
	defer release()

	var answer domain.ChatAnswer
	scopeDecision := useCase.scope.Evaluate(question.Question)
	if scopeDecision.AllowsActiveAnalysis() {
		answer, err = useCase.responder.Answer(ctx, analysisContext, question)
		if err != nil {
			return domain.ChatAnswer{}, fmt.Errorf("answer analysis question: %w", err)
		}
	} else {
		answer = newOutOfScopeChatAnswer(analysisContext.AnalysisID, question.Question)
	}

	err = useCase.saveExchange(ctx, question, answer)
	if err != nil {
		return domain.ChatAnswer{}, err
	}

	_ = useCase.cache.Save(ctx, analysisContext, useCase.cacheTTL)

	return answer, nil
}

func (useCase *Chat) saveExchange(ctx context.Context, question domain.ChatQuestion, answer domain.ChatAnswer) error {
	err := useCase.history.SaveExchange(ctx, domain.ChatMessage{
		AnalysisID: question.AnalysisID,
		Role:       domain.ChatRoleUser,
		Content:    question.Question,
		CreatedAt:  useCase.now().UTC(),
	}, domain.ChatMessage{
		AnalysisID: answer.AnalysisID,
		Role:       domain.ChatRoleAssistant,
		Content:    answer.Answer,
		Evidence:   answer.Evidence,
		Citations:  answer.Citations,
		CreatedAt:  useCase.now().UTC(),
	})
	if err != nil {
		return fmt.Errorf("save chat history: %w", err)
	}
	return nil
}

// Regenerate re-asks the question carried by the supplied user message
// against the current analysis context. The message must belong to the
// supplied analysis and have role=user; assistant messages are rejected
// with ErrChatMessageNotFound so callers cannot rerun the LLM with one
// of its own previous answers.
func (useCase *Chat) Regenerate(ctx context.Context, analysisID, messageID string) (domain.ChatAnswer, error) {
	cleanedAnalysis := strings.TrimSpace(analysisID)
	cleanedMessage := strings.TrimSpace(messageID)
	if cleanedAnalysis == "" || cleanedMessage == "" {
		return domain.ChatAnswer{}, fmt.Errorf("%w: analysis and message id are required", domain.ErrChatMessageNotFound)
	}
	message, err := useCase.history.FindMessage(ctx, cleanedMessage)
	if err != nil {
		return domain.ChatAnswer{}, err
	}
	if message.AnalysisID != cleanedAnalysis || message.Role != domain.ChatRoleUser {
		return domain.ChatAnswer{}, domain.ErrChatMessageNotFound
	}
	return useCase.Ask(ctx, domain.ChatQuestion{
		AnalysisID: cleanedAnalysis,
		Question:   message.Content,
	})
}

// ChatStreamEvent describes a single event emitted by AskStream.
//
// Frontends consume an ordered stream of events: `token` events carry partial
// or full text fragments, `evidence_cited` events carry citation references
// and a terminal `done` event marks completion. Implementations may also emit
// `error` events to communicate cancellation or upstream failures.
type ChatStreamEvent struct {
	Type     string
	Token    string
	Evidence []string
}

// ChatStreamSink receives streaming chat events. Implementations must be safe
// to call from the use case goroutine and should propagate cancellation back
// to the use case by returning an error.
type ChatStreamSink interface {
	Send(event ChatStreamEvent) error
}

// AskStream answers a scoped question and emits intermediate events to the supplied sink.
//
// The current implementation produces the full answer synchronously and then
// emits a single `token` event followed by `evidence_cited` and `done`. Real
// streaming LLM adapters can replace the synchronous Answer call with a
// chunked stream without changing the public event contract.
func (useCase *Chat) AskStream(ctx context.Context, question domain.ChatQuestion, sink ChatStreamSink) error {
	answer, err := useCase.Ask(ctx, question)
	if err != nil {
		return err
	}

	if err := sink.Send(ChatStreamEvent{Type: "token", Token: answer.Answer}); err != nil {
		return err
	}
	if len(answer.Evidence) > 0 {
		if err := sink.Send(ChatStreamEvent{Type: "evidence_cited", Evidence: answer.Evidence}); err != nil {
			return err
		}
	}
	return sink.Send(ChatStreamEvent{Type: "done"})
}

const defaultChatHistoryLimit = 50
const maxChatHistoryLimit = 200

// History returns persisted chat history for an analysis honoring the supplied filter.
func (useCase *Chat) History(ctx context.Context, analysisID string, filter domain.ChatHistoryFilter) ([]domain.ChatMessage, error) {
	analysisID = strings.TrimSpace(analysisID)
	if analysisID == "" {
		return nil, fmt.Errorf("%w: analysis id is required", domain.ErrAnalysisNotFound)
	}

	if _, err := useCase.repository.Find(ctx, analysisID); err != nil {
		return nil, fmt.Errorf("find analysis: %w", err)
	}

	filter = normalizeChatHistoryFilter(filter)
	messages, err := useCase.history.List(ctx, analysisID, filter)
	if err != nil {
		return nil, fmt.Errorf("list chat history: %w", err)
	}

	return messages, nil
}

func normalizeChatHistoryFilter(filter domain.ChatHistoryFilter) domain.ChatHistoryFilter {
	if filter.Limit <= 0 {
		filter.Limit = defaultChatHistoryLimit
	}
	if filter.Limit > maxChatHistoryLimit {
		filter.Limit = maxChatHistoryLimit
	}
	return filter
}

func (useCase *Chat) findAnalysisContext(ctx context.Context, analysisID string) (domain.AnalysisContext, error) {
	analysisContext, err := useCase.cache.Find(ctx, analysisID)
	if err == nil {
		return analysisContext, nil
	}

	analysis, err := useCase.repository.Find(ctx, analysisID)
	if err != nil {
		return domain.AnalysisContext{}, fmt.Errorf("find analysis: %w", err)
	}

	analysisContext = domain.NewAnalysisContext(analysis)
	_ = useCase.cache.Save(ctx, analysisContext, useCase.cacheTTL)

	return analysisContext, nil
}
