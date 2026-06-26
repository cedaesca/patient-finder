-- +goose Up
-- +goose StatementBegin
DROP INDEX IF EXISTS audit.idx_audit_events_team;
ALTER TABLE audit.events DROP COLUMN IF EXISTS team_id;
ALTER TABLE users DROP COLUMN IF EXISTS locale;
ALTER TABLE users DROP COLUMN IF EXISTS banned_at;
ALTER TABLE users DROP COLUMN IF EXISTS onboarded_at;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE users ADD COLUMN locale VARCHAR(10) DEFAULT 'es-ES' NOT NULL;
ALTER TABLE users ADD COLUMN banned_at TIMESTAMP WITH TIME ZONE DEFAULT null;
ALTER TABLE users ADD COLUMN onboarded_at TIMESTAMP WITH TIME ZONE DEFAULT null;
ALTER TABLE audit.events ADD COLUMN team_id UUID NOT NULL;
CREATE INDEX idx_audit_events_team ON audit.events(team_id);
-- +goose StatementEnd
