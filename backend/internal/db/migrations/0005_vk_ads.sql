ALTER TABLE ad_campaigns DROP CONSTRAINT IF EXISTS ad_campaigns_lifecycle_check;
ALTER TABLE ad_campaigns ADD CONSTRAINT ad_campaigns_lifecycle_check
    CHECK (lifecycle IN ('draft','running','uploading','vk_plan','vk_groups','vk_ads','completed'));

ALTER TABLE ad_campaigns ADD COLUMN IF NOT EXISTS vk_settings JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE ad_campaigns ADD COLUMN IF NOT EXISTS vk_ad_plan_id TEXT NOT NULL DEFAULT '';
ALTER TABLE ad_campaigns ADD COLUMN IF NOT EXISTS vk_upload_error TEXT NOT NULL DEFAULT '';

ALTER TABLE ad_campaign_items ADD COLUMN IF NOT EXISTS audience_id BIGINT NOT NULL DEFAULT 0;
ALTER TABLE ad_campaign_items ADD COLUMN IF NOT EXISTS audience_name TEXT NOT NULL DEFAULT '';
ALTER TABLE ad_campaign_items ADD COLUMN IF NOT EXISTS vk_ad_group_id TEXT NOT NULL DEFAULT '';
