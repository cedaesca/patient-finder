-- +goose Up
CREATE UNIQUE INDEX IF NOT EXISTS idx_persons_source_dedup
ON persons (source, source_id)
WHERE source IS NOT NULL AND source_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_persons_source_dedup;
