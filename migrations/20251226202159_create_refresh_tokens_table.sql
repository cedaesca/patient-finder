-- +goose Up
-- +goose StatementBegin
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS refresh_tokens (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL,
  hash CHAR(64) NOT NULL UNIQUE,
  expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
  revoked_at TIMESTAMP WITH TIME ZONE NULL,
  replaced_by UUID NULL,

  CONSTRAINT fk_refresh_tokens_user
    FOREIGN KEY (user_id)
    REFERENCES users(id)
    ON DELETE CASCADE
);

CREATE INDEX idx_refresh_tokens_user_id
  ON refresh_tokens(user_id);

CREATE INDEX idx_refresh_tokens_hash
  ON refresh_tokens(hash);

CREATE INDEX idx_refresh_tokens_expires_at
  ON refresh_tokens(expires_at);
-- +goose StatementEnd


-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS refresh_tokens;
-- +goose StatementEnd
