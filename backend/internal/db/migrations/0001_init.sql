CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE IF NOT EXISTS batches (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title       TEXT NOT NULL DEFAULT '',
    template_id TEXT NOT NULL,
    video_model TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS tasks (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    batch_id            UUID NOT NULL REFERENCES batches(id) ON DELETE CASCADE,

    first_name          TEXT NOT NULL,
    last_name           TEXT NOT NULL,

    -- config snapshot for this task
    template_id         TEXT NOT NULL,
    image_settings      JSONB NOT NULL DEFAULT '{}'::jsonb,
    video_model         TEXT NOT NULL,
    video_prompt        TEXT NOT NULL,
    video_duration      INT,
    video_resolution    TEXT,
    video_aspect_ratio  TEXT,

    -- state machine
    status              TEXT NOT NULL DEFAULT 'queued',
    error               TEXT NOT NULL DEFAULT '',

    -- external references / results
    imanator_order_id   TEXT NOT NULL DEFAULT '',
    image_url           TEXT NOT NULL DEFAULT '',
    openrouter_job_id   TEXT NOT NULL DEFAULT '',
    video_object        TEXT NOT NULL DEFAULT '',

    -- worker lease (for SKIP LOCKED style claiming across restarts)
    locked_at           TIMESTAMPTZ,

    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_tasks_batch     ON tasks (batch_id);
CREATE INDEX IF NOT EXISTS idx_tasks_status    ON tasks (status);
CREATE INDEX IF NOT EXISTS idx_tasks_created   ON tasks (created_at DESC);
