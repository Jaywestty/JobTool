-- +goose Up
-- +goose StatementBegin

-- The ingest pipeline reports kept (passed the tech and language gates) in
-- addition to the fetched/inserted/updated counters added with the table.
ALTER TABLE source_runs ADD COLUMN IF NOT EXISTS kept integer NOT NULL DEFAULT 0;

-- Scheduler lookups read the newest run per source to decide due-ness.
CREATE INDEX IF NOT EXISTS source_runs_source_started_at_desc_idx
    ON source_runs (source, started_at DESC);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS source_runs_source_started_at_desc_idx;
ALTER TABLE source_runs DROP COLUMN IF EXISTS kept;
-- +goose StatementEnd
