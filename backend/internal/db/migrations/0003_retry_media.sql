-- Media library: uploaded assets (currently mp3 soundtracks) that can be
-- attached to a generation batch. The binary lives in MinIO under `object`.
CREATE TABLE IF NOT EXISTS media_assets (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kind             TEXT NOT NULL DEFAULT 'audio',
    title            TEXT NOT NULL DEFAULT '',
    filename         TEXT NOT NULL DEFAULT '',
    object           TEXT NOT NULL,
    content_type     TEXT NOT NULL DEFAULT '',
    size_bytes       BIGINT NOT NULL DEFAULT 0,
    duration_seconds DOUBLE PRECISION NOT NULL DEFAULT 0,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_media_assets_created ON media_assets (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_media_assets_kind    ON media_assets (kind);

-- Retry bookkeeping: transient failures (network blips, 5xx, dead process) no
-- longer end a task. `next_attempt_at` gates re-claiming until the backoff
-- window has passed.
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS attempts        INT NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS next_attempt_at TIMESTAMPTZ;

-- Audio options. generate_audio=false (the default) means the model returns a
-- silent clip and the soundtrack is muxed in from audio_object afterwards.
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS generate_audio BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS audio_asset_id UUID REFERENCES media_assets(id) ON DELETE SET NULL;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS audio_object   TEXT NOT NULL DEFAULT '';

-- Raw (silent) clip as downloaded from OpenRouter. Kept so the mixing stage can
-- be retried without paying for generation again.
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS source_video_object TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_tasks_claim ON tasks (status, next_attempt_at, created_at);
