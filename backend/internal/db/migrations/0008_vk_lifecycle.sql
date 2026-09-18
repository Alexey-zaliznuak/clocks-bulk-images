ALTER TABLE ad_campaigns DROP CONSTRAINT IF EXISTS ad_campaigns_lifecycle_check;
ALTER TABLE ad_campaigns ADD CONSTRAINT ad_campaigns_lifecycle_check
    CHECK (lifecycle IN ('draft','running','uploading','vk_plan','vk_groups','vk_ads','completed'));
