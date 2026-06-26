-- +goose Up
-- +goose StatementBegin
INSERT INTO permissions (slug, group_code, group_name, name, description) VALUES
  ('audit:read',
  'audit',
  '{"es": "Auditoría", "en": "Audit"}'::jsonb,
   '{"es": "Ver logs de auditoría", "en": "View audit logs"}'::jsonb,
   '{"es": "Permite consultar el registro inmutable de acciones.", "en": "Allows querying the immutable action log."}'::jsonb)
ON CONFLICT (slug) DO NOTHING;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM permissions WHERE slug = 'audit:read';
-- +goose StatementEnd
