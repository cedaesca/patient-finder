-- +goose Up
-- Insert new permissions
INSERT INTO permissions (slug, group_code, group_name, name, description) VALUES
  ('patients:read', 'patients', 'Pacientes', 'Leer pacientes', 'Permite consultar la lista de pacientes y ver detalles.'),
  ('patients:create', 'patients', 'Pacientes', 'Crear pacientes', 'Permite registrar nuevos pacientes.'),
  ('patients:update', 'patients', 'Pacientes', 'Actualizar pacientes', 'Permite modificar datos de pacientes existentes.'),
  ('patients:delete', 'patients', 'Pacientes', 'Eliminar pacientes', 'Permite eliminar pacientes del sistema.'),
  ('centers:read', 'centers', 'Centros', 'Leer centros', 'Permite consultar la lista de centros y ver detalles.'),
  ('centers:create', 'centers', 'Centros', 'Crear centros', 'Permite registrar nuevos centros médicos.'),
  ('centers:update', 'centers', 'Centros', 'Actualizar centros', 'Permite modificar datos de centros existentes.'),
  ('centers:delete', 'centers', 'Centros', 'Eliminar centros', 'Permite desactivar centros del sistema.'),
  ('users:read', 'users', 'Usuarios', 'Leer usuarios', 'Permite consultar la lista de usuarios del sistema.'),
  ('users:create', 'users', 'Usuarios', 'Crear usuarios', 'Permite crear nuevas cuentas de usuario.'),
  ('users:update', 'users', 'Usuarios', 'Actualizar usuarios', 'Permite modificar datos de usuarios existentes.'),
  ('users:delete', 'users', 'Usuarios', 'Eliminar usuarios', 'Permite desactivar usuarios del sistema.')
ON CONFLICT (slug) DO NOTHING;

-- Insert roles
INSERT INTO roles (name, display_name, is_global) VALUES
  ('admin', 'Administrador', true),
  ('supervisor', 'Supervisor', true),
  ('encargado', 'Encargado de Centro', false),
  ('digitador', 'Digitador', false),
  ('registrador', 'Registrador', false);

-- Assign permissions to admin (all)
INSERT INTO role_permissions (role_id, permission_slug)
SELECT r.id, p.slug
FROM roles r CROSS JOIN permissions p
WHERE r.name = 'admin'
ON CONFLICT DO NOTHING;

-- Assign permissions to supervisor (all except users:*)
INSERT INTO role_permissions (role_id, permission_slug)
SELECT r.id, p.slug
FROM roles r CROSS JOIN permissions p
WHERE r.name = 'supervisor' AND p.slug NOT LIKE 'users:%'
ON CONFLICT DO NOTHING;

-- Assign permissions to encargado (patients:*)
INSERT INTO role_permissions (role_id, permission_slug)
SELECT r.id, p.slug
FROM roles r CROSS JOIN permissions p
WHERE r.name = 'encargado' AND p.slug LIKE 'patients:%'
ON CONFLICT DO NOTHING;

-- Assign permissions to digitador (except patients:delete)
INSERT INTO role_permissions (role_id, permission_slug)
SELECT r.id, p.slug
FROM roles r CROSS JOIN permissions p
WHERE r.name = 'digitador' AND p.slug IN ('patients:read', 'patients:create', 'patients:update')
ON CONFLICT DO NOTHING;

-- Assign permissions to registrador (only patients:read, patients:create)
INSERT INTO role_permissions (role_id, permission_slug)
SELECT r.id, p.slug
FROM roles r CROSS JOIN permissions p
WHERE r.name = 'registrador' AND p.slug IN ('patients:read', 'patients:create')
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM role_permissions WHERE role_id IN (SELECT id FROM roles);
DELETE FROM roles;
DELETE FROM permissions WHERE slug LIKE 'patients:%' OR slug LIKE 'centers:%' OR slug LIKE 'users:%';
