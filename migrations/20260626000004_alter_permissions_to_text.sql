-- +goose Up
ALTER TABLE permissions
  ALTER COLUMN group_name TYPE VARCHAR(255) USING group_name->>'es',
  ALTER COLUMN name TYPE VARCHAR(255) USING name->>'es',
  ALTER COLUMN description TYPE TEXT USING description->>'es';

ALTER TABLE permissions
  ALTER COLUMN group_name SET DEFAULT '',
  ALTER COLUMN name SET DEFAULT '',
  ALTER COLUMN description SET DEFAULT '';

-- +goose Down
ALTER TABLE permissions
  ALTER COLUMN group_name TYPE JSONB USING to_jsonb(group_name),
  ALTER COLUMN name TYPE JSONB USING to_jsonb(name),
  ALTER COLUMN description TYPE JSONB USING to_jsonb(description);

ALTER TABLE permissions
  ALTER COLUMN group_name SET DEFAULT '{}'::jsonb,
  ALTER COLUMN name SET DEFAULT '{}'::jsonb,
  ALTER COLUMN description SET DEFAULT '{}'::jsonb;
