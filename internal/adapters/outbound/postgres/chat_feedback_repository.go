package postgres

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/guferreira1/observai-api/internal/adapters/outbound/postgres/sqlc"
	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/platform/observability"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ChatFeedbackRepository persists chat feedback in PostgreSQL.
type ChatFeedbackRepository struct {
	pool     *pgxpool.Pool
	queries  *sqlc.Queries
	observer observability.ProviderObserver
}

// NewChatFeedbackRepository creates a PostgreSQL chat feedback repository.
//
// The provided pool must already be initialized. Sharing the pool with the
// existing AnalysisRepository keeps connection usage predictable.
func NewChatFeedbackRepository(pool *pgxpool.Pool, opts ...RepositoryOptions) *ChatFeedbackRepository {
	observer := observability.ProviderObserver(observability.NoopProviderObserver{})
	if len(opts) > 0 && opts[0].Observer != nil {
		observer = opts[0].Observer
	}
	return &ChatFeedbackRepository{
		pool:     pool,
		queries:  sqlc.New(pool),
		observer: observer,
	}
}

func (repository *ChatFeedbackRepository) observe(operation string, startedAt time.Time, err error) {
	repository.observer.Observe("postgres", operation, time.Since(startedAt), err)
}

// SaveFeedback upserts feedback for a single chat message.
//
// The domain ChatFeedback uses string identifiers; the underlying schema
// references analysis_chat_messages.id which is BIGSERIAL, so MessageID is
// parsed as int64 at the adapter boundary.
func (repository *ChatFeedbackRepository) SaveFeedback(ctx context.Context, feedback domain.ChatFeedback) (err error) {
	startedAt := time.Now()
	defer func() { repository.observe("upsert_chat_feedback", startedAt, err) }()

	messageID, err := strconv.ParseInt(feedback.MessageID, 10, 64)
	if err != nil {
		return fmt.Errorf("parse chat feedback message id %q: %w", feedback.MessageID, err)
	}

	createdAt := feedback.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	params := sqlc.UpsertChatFeedbackParams{
		AnalysisID: feedback.AnalysisID,
		MessageID:  messageID,
		Useful:     feedback.Useful,
		Reason:     feedback.Reason,
		CreatedAt:  pgtype.Timestamptz{Time: createdAt, Valid: true},
	}
	if err := repository.queries.UpsertChatFeedback(ctx, params); err != nil {
		return fmt.Errorf("upsert chat feedback: %w", err)
	}
	return nil
}
