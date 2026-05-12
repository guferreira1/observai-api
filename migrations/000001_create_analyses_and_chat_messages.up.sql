CREATE TABLE analyses (
    id TEXT PRIMARY KEY,
    summary TEXT NOT NULL,
    severity TEXT NOT NULL,
    confidence TEXT NOT NULL,
    affected_services TEXT[] NOT NULL DEFAULT '{}',
    evidence JSONB NOT NULL DEFAULT '[]'::jsonb,
    detected_anomalies TEXT[] NOT NULL DEFAULT '{}',
    possible_root_causes JSONB NOT NULL DEFAULT '[]'::jsonb,
    recommended_actions JSONB NOT NULL DEFAULT '[]'::jsonb,
    code_level_insights TEXT[] NOT NULL DEFAULT '{}',
    missing_evidence TEXT[] NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX analyses_created_at_idx ON analyses (created_at DESC);
CREATE INDEX analyses_severity_idx ON analyses (severity);

CREATE TABLE analysis_chat_messages (
    id BIGSERIAL PRIMARY KEY,
    analysis_id TEXT NOT NULL REFERENCES analyses (id) ON DELETE CASCADE,
    role TEXT NOT NULL,
    content TEXT NOT NULL,
    evidence JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT analysis_chat_messages_role_check CHECK (role IN ('user', 'assistant'))
);

CREATE INDEX analysis_chat_messages_analysis_created_at_idx ON analysis_chat_messages (analysis_id, created_at ASC);
