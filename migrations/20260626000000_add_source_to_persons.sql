-- +goose Up
-- +goose StatementBegin
ALTER TABLE persons
    ADD COLUMN source TEXT,
    ADD COLUMN source_id TEXT,
    ALTER COLUMN created_by DROP NOT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE persons
    DROP COLUMN IF EXISTS source,
    DROP COLUMN IF EXISTS source_id;
UPDATE persons SET created_by = '00000000-0000-0000-0000-000000000000' WHERE created_by IS NULL;
ALTER TABLE persons ALTER COLUMN created_by SET NOT NULL;
-- +goose StatementEnd
