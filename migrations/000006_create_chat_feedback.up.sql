CREATE TABLE analysis_chat_feedback (
    analysis_id TEXT NOT NULL REFERENCES analyses (id) ON DELETE CASCADE,
    message_id BIGINT NOT NULL REFERENCES analysis_chat_messages (id) ON DELETE CASCADE,
    useful BOOLEAN NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (analysis_id, message_id)
);

CREATE INDEX analysis_chat_feedback_useful_idx ON analysis_chat_feedback (useful);
