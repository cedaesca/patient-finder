-- +goose Up
-- +goose StatementBegin
CREATE SCHEMA IF NOT EXISTS audit;

CREATE TABLE audit.events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL,
    user_id UUID,
    action VARCHAR(10) NOT NULL,
    resource_type VARCHAR(50) NOT NULL,
    resource_id UUID,
    before_data JSONB,
    after_data JSONB,
    ip_address INET,
    user_agent TEXT,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_audit_events_team ON audit.events(team_id);
CREATE INDEX idx_audit_events_user ON audit.events(user_id);
CREATE INDEX idx_audit_events_resource ON audit.events(resource_type, resource_id);
CREATE INDEX idx_audit_events_created_at ON audit.events(created_at DESC);
CREATE INDEX idx_audit_events_action ON audit.events(action);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_audit_events_action;
DROP INDEX IF EXISTS idx_audit_events_created_at;
DROP INDEX IF EXISTS idx_audit_events_resource;
DROP INDEX IF EXISTS idx_audit_events_user;
DROP INDEX IF EXISTS idx_audit_events_team;

DROP TABLE IF EXISTS audit.events;
DROP SCHEMA IF EXISTS audit;
-- +goose StatementEnd
