-- Fix providers table to match expected structure

-- Drop the auto-increment id and make workspace_id the primary key
ALTER TABLE providers DROP PRIMARY KEY;
ALTER TABLE providers DROP COLUMN id;
ALTER TABLE providers CHANGE COLUMN workspace_id id VARCHAR(255) NOT NULL;
ALTER TABLE providers ADD PRIMARY KEY (id);

-- Add missing columns
ALTER TABLE providers 
  ADD COLUMN display_name VARCHAR(255) AFTER id,
  ADD COLUMN domain VARCHAR(255) AFTER display_name,
  ADD COLUMN rate_limit_workspace_daily INT DEFAULT 2000 AFTER domain,
  ADD COLUMN rate_limit_per_user_daily INT DEFAULT 100 AFTER rate_limit_workspace_daily,
  ADD COLUMN rate_limit_custom_users JSON AFTER rate_limit_per_user_daily,
  ADD COLUMN provider_config JSON AFTER provider_type,
  ADD COLUMN service_account_json TEXT AFTER provider_config;

-- Drop the unique constraint that references workspace_id
ALTER TABLE providers DROP INDEX uk_workspace_provider;

-- Add new unique constraint for id and provider_type
ALTER TABLE providers ADD UNIQUE KEY uk_id_provider (id, provider_type);

-- Add domain index
ALTER TABLE providers ADD INDEX idx_domain (domain);

-- Set default values for new columns
UPDATE providers 
SET 
  display_name = COALESCE(display_name, id),
  domain = COALESCE(domain, CONCAT(id, '.com')),
  rate_limit_workspace_daily = 2000,
  rate_limit_per_user_daily = 100;