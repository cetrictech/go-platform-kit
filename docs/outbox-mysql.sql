-- Apply explicitly, once, using your migration system. Requires MySQL 8+ for SKIP LOCKED.
CREATE TABLE sdk_outbox (
  id VARCHAR(36) PRIMARY KEY,
  payload LONGBLOB NOT NULL,
  attempts INTEGER NOT NULL DEFAULT 0,
  next_attempt DATETIME(6) NOT NULL,
  created_at DATETIME(6) NOT NULL,
  delivered_at DATETIME(6),
  dead_at DATETIME(6),
  INDEX sdk_outbox_pending (delivered_at, dead_at, next_attempt, created_at, id)
) ENGINE=InnoDB;
