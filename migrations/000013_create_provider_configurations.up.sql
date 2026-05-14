CREATE TABLE provider_configurations (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL CHECK (type IN (
        'prometheus', 'loki', 'jaeger', 'elasticsearch', 'opensearch',
        'otel', 'tempo', 'dynatrace', 'datadog', 'newrelic'
    )),
    name TEXT NOT NULL,
    url TEXT NOT NULL,
    timeout_ms INTEGER NOT NULL DEFAULT 10000,
    signals TEXT[] NOT NULL DEFAULT '{}',
    options JSONB NOT NULL DEFAULT '{}'::JSONB,
    credentials_ciphertext TEXT,
    is_active BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX provider_configurations_name_idx ON provider_configurations (name);
CREATE INDEX provider_configurations_active_idx ON provider_configurations (is_active) WHERE is_active = true;
