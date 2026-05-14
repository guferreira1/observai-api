CREATE TABLE webhook_subscriptions (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL DEFAULT '',
    url TEXT NOT NULL,
    secret TEXT NOT NULL,
    event TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    disabled_at TIMESTAMPTZ
);

CREATE INDEX webhook_subscriptions_event_idx ON webhook_subscriptions (event) WHERE disabled_at IS NULL;
