CREATE TABLE IF NOT EXISTS ad_campaigns (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title                   TEXT NOT NULL DEFAULT '',
    lifecycle               TEXT NOT NULL DEFAULT 'draft'
                            CHECK (lifecycle IN ('draft','running')),
    name_text_template      TEXT NOT NULL,
    surname_text_template   TEXT NOT NULL,
    template_id             TEXT NOT NULL,
    image_settings          JSONB NOT NULL DEFAULT '{}'::jsonb,
    name_setting_key        TEXT NOT NULL DEFAULT 'name',
    video_model             TEXT NOT NULL,
    video_prompt            TEXT NOT NULL,
    video_duration          INT,
    video_resolution        TEXT NOT NULL DEFAULT '',
    video_aspect_ratio      TEXT NOT NULL DEFAULT '',
    generate_audio          BOOLEAN NOT NULL DEFAULT false,
    audio_asset_id          UUID REFERENCES media_assets(id) ON DELETE SET NULL,
    audio_object            TEXT NOT NULL DEFAULT '',
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at              TIMESTAMPTZ,
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ad_campaign_items (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    campaign_id         UUID NOT NULL REFERENCES ad_campaigns(id) ON DELETE CASCADE,
    kind                TEXT NOT NULL CHECK (kind IN ('name','surname')),
    value               TEXT NOT NULL,
    position            INT NOT NULL DEFAULT 0,
    status              TEXT NOT NULL DEFAULT 'draft',
    error               TEXT NOT NULL DEFAULT '',
    attempts            INT NOT NULL DEFAULT 0,
    next_attempt_at     TIMESTAMPTZ,
    locked_at           TIMESTAMPTZ,
    imanator_order_id   TEXT NOT NULL DEFAULT '',
    image_url           TEXT NOT NULL DEFAULT '',
    image_object        TEXT NOT NULL DEFAULT '',
    openrouter_job_id   TEXT NOT NULL DEFAULT '',
    source_video_object TEXT NOT NULL DEFAULT '',
    video_object        TEXT NOT NULL DEFAULT '',
    cost_usd            DOUBLE PRECISION NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (campaign_id, kind, value)
);

ALTER TABLE ad_campaign_items ADD COLUMN IF NOT EXISTS position INT NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_ad_campaigns_created
    ON ad_campaigns (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ad_campaign_items_campaign
    ON ad_campaign_items (campaign_id, created_at);
CREATE INDEX IF NOT EXISTS idx_ad_campaign_items_claim
    ON ad_campaign_items (status, next_attempt_at, created_at);
