CREATE TABLE webhook_deliveries (
    id TEXT PRIMARY KEY,
    subscription_id TEXT NOT NULL REFERENCES webhook_subscriptions (id) ON DELETE CASCADE,
    event TEXT NOT NULL,
    payload JSONB NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending', 'delivered', 'failed')) DEFAULT 'pending',
    attempt INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    response_status INTEGER,
    next_attempt_at TIMESTAMPTZ,
    delivered_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX webhook_deliveries_subscription_idx ON webhook_deliveries (subscription_id);
CREATE INDEX webhook_deliveries_status_idx ON webhook_deliveries (status, next_attempt_at) WHERE status = 'pending';
CREATE INDEX webhook_deliveries_created_idx ON webhook_deliveries (created_at DESC);
