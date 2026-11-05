-- Apply explicitly, once, using your migration system.
CREATE TABLE sdk_outbox (
  id VARCHAR(36) PRIMARY KEY,
  payload BYTEA NOT NULL,
  attempts INTEGER NOT NULL DEFAULT 0,
  next_attempt TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  delivered_at TIMESTAMPTZ,
  dead_at TIMESTAMPTZ
);
CREATE INDEX sdk_outbox_pending ON sdk_outbox (next_attempt, created_at, id)
WHERE delivered_at IS NULL AND dead_at IS NULL;
