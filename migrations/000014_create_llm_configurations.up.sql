CREATE TABLE llm_configurations (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL CHECK (type IN (
        'ollama', 'openai', 'anthropic', 'azure', 'openrouter'
    )),
    name TEXT NOT NULL,
    base_url TEXT NOT NULL,
    model TEXT NOT NULL,
    timeout_ms INTEGER NOT NULL DEFAULT 30000,
    api_key_ciphertext TEXT,
    options JSONB NOT NULL DEFAULT '{}'::JSONB,
    is_active BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX llm_configurations_name_idx ON llm_configurations (name);
CREATE UNIQUE INDEX llm_configurations_only_one_active_idx ON llm_configurations ((is_active)) WHERE is_active = true;
