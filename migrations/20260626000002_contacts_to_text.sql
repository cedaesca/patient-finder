-- +goose Up
ALTER TABLE persons ALTER COLUMN contacts TYPE TEXT USING contacts::text;
ALTER TABLE centers ALTER COLUMN contacts TYPE TEXT USING contacts::text;

-- +goose Down
ALTER TABLE persons ALTER COLUMN contacts TYPE JSONB USING contacts::jsonb;
ALTER TABLE centers ALTER COLUMN contacts TYPE JSONB USING contacts::jsonb;
