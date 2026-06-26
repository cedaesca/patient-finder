-- +goose Up
-- +goose StatementBegin
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS permissions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  slug VARCHAR(100) UNIQUE NOT NULL,
  group_code VARCHAR(100) NOT NULL DEFAULT '',
  group_name JSONB NOT NULL DEFAULT '{}'::jsonb,
  name JSONB NOT NULL DEFAULT '{}'::jsonb,
  description JSONB NOT NULL DEFAULT '{}'::jsonb,

  -- Audit
  created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

ALTER TABLE IF EXISTS permissions
  ADD COLUMN IF NOT EXISTS group_code VARCHAR(100) NOT NULL DEFAULT '';

ALTER TABLE IF EXISTS permissions
  ADD COLUMN IF NOT EXISTS group_name JSONB NOT NULL DEFAULT '{}'::jsonb;

CREATE INDEX IF NOT EXISTS idx_permissions_slug ON permissions(slug);
CREATE INDEX IF NOT EXISTS idx_permissions_group_code ON permissions(group_code);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_permissions_slug;
DROP INDEX IF EXISTS idx_permissions_group_code;

DROP TABLE IF EXISTS permissions;
-- +goose StatementEnd