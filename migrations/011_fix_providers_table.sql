-- Fix providers table structure to match what the code expects

-- First, rename workspace_id back to id if needed
ALTER TABLE providers 
  CHANGE COLUMN workspace_id id VARCHAR(255) NOT NULL FIRST;

-- Add missing columns
ALTER TABLE providers 
  ADD COLUMN IF NOT EXISTS display_name VARCHAR(255) AFTER id,
  ADD COLUMN IF NOT EXISTS domain VARCHAR(255) AFTER display_name,
  ADD COLUMN IF NOT EXISTS rate_limit_workspace_daily INT DEFAULT 2000 AFTER domain,
  ADD COLUMN IF NOT EXISTS rate_limit_per_user_daily INT DEFAULT 100 AFTER rate_limit_workspace_daily,
  ADD COLUMN IF NOT EXISTS rate_limit_custom_users JSON AFTER rate_limit_per_user_daily,
  ADD COLUMN IF NOT EXISTS provider_config JSON AFTER provider_type,
  ADD COLUMN IF NOT EXISTS service_account_json TEXT AFTER provider_config;

-- Update the primary key if needed
ALTER TABLE providers DROP PRIMARY KEY, ADD PRIMARY KEY (id);

-- Add indexes
ALTER TABLE providers 
  ADD INDEX IF NOT EXISTS idx_domain (domain),
  ADD INDEX IF NOT EXISTS idx_enabled (enabled),
  ADD INDEX IF NOT EXISTS idx_provider_type (provider_type);

-- Migrate data from existing providers to populate the new columns
-- For now, set defaults for missing data
UPDATE providers 
SET 
  display_name = COALESCE(display_name, id),
  domain = COALESCE(domain, CONCAT(id, '.com')),
  rate_limit_workspace_daily = COALESCE(rate_limit_workspace_daily, 2000),
  rate_limit_per_user_daily = COALESCE(rate_limit_per_user_daily, 100)
WHERE display_name IS NULL OR domain IS NULL;