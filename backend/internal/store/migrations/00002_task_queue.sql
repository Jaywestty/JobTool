-- +goose Up
-- +goose StatementBegin

CREATE TABLE task_queue (
    id           bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind         text        NOT NULL,
    dedupe_key   text,
    payload      jsonb       NOT NULL DEFAULT '{}'::jsonb,
    status       text        NOT NULL DEFAULT 'pending',
    attempts     integer     NOT NULL DEFAULT 0,
    available_at timestamptz NOT NULL DEFAULT now(),
    locked_until timestamptz,
    last_error   text NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL DEFAULT now(),
    finished_at  timestamptz,
    CONSTRAINT task_queue_status_check CHECK (status IN ('pending', 'running', 'done', 'failed'))
);

-- The claim query only ever looks at pending, due work.
CREATE INDEX task_queue_claim_idx ON task_queue (available_at) WHERE status = 'pending';
-- A dedupe key makes re-enqueueing the same work a no-op while it is live.
CREATE UNIQUE INDEX task_queue_dedupe_idx ON task_queue (kind, dedupe_key)
    WHERE dedupe_key IS NOT NULL AND status IN ('pending', 'running');

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS task_queue;
-- +goose StatementEnd
