-- Fixed migration: Handle existing provider_id columns and clean up workspace references

-- Step 1: Messages table - copy workspace_id to provider_id if needed, then drop workspace_id
UPDATE messages 
SET provider_id = workspace_id 
WHERE provider_id IS NULL AND workspace_id IS NOT NULL;

ALTER TABLE messages 
  DROP COLUMN IF EXISTS workspace_id,
  DROP INDEX IF EXISTS idx_workspace_id;

-- Step 2: Check and update recipients table
ALTER TABLE recipients
  CHANGE COLUMN workspace_id provider_id VARCHAR(255) NOT NULL;

-- Step 3: Check and update rate_limit_usage
ALTER TABLE rate_limit_usage
  CHANGE COLUMN IF EXISTS workspace_id provider_id VARCHAR(255);

-- Step 4: Rename workspace tables to provider tables (with existence checks)
-- First check if target tables don't exist
DROP TABLE IF EXISTS providers_temp;
DROP TABLE IF EXISTS provider_rate_limits_temp;
DROP TABLE IF EXISTS provider_user_rate_limits_temp;

-- Rename tables if they exist
RENAME TABLE workspace_providers TO providers_temp;
RENAME TABLE workspace_rate_limits TO provider_rate_limits_temp;
RENAME TABLE workspace_user_rate_limits TO provider_user_rate_limits_temp;

-- Now rename to final names
RENAME TABLE providers_temp TO providers;
RENAME TABLE provider_rate_limits_temp TO provider_rate_limits;
RENAME TABLE provider_user_rate_limits_temp TO provider_user_rate_limits;

-- Step 5: Update column names in renamed tables
ALTER TABLE provider_rate_limits
  CHANGE COLUMN workspace_id provider_id VARCHAR(255);

ALTER TABLE provider_user_rate_limits
  CHANGE COLUMN workspace_id provider_id VARCHAR(255);

-- Step 6: Update provider_header_rewrite_rules
ALTER TABLE provider_header_rewrite_rules
  CHANGE COLUMN IF EXISTS workspace_id provider_id VARCHAR(255);

-- Step 7: Handle pool tables
DROP TABLE IF EXISTS pool_providers_temp;
RENAME TABLE pool_workspaces TO pool_providers_temp;
RENAME TABLE pool_providers_temp TO pool_providers;

ALTER TABLE pool_providers
  CHANGE COLUMN workspace_id provider_id VARCHAR(255);

-- Step 8: Update provider_selections
ALTER TABLE provider_selections
  CHANGE COLUMN IF EXISTS workspace_id provider_id VARCHAR(255);

-- Step 9: Update indexes on recipients table
ALTER TABLE recipients
  DROP INDEX IF EXISTS idx_workspace_status,
  DROP INDEX IF EXISTS uk_email_workspace,
  ADD INDEX IF NOT EXISTS idx_provider_status (provider_id, status),
  ADD UNIQUE KEY IF NOT EXISTS uk_email_provider (email_address, provider_id);

-- Step 10: Update indexes on other tables
ALTER TABLE provider_rate_limits
  DROP INDEX IF EXISTS idx_workspace_id,
  ADD INDEX IF NOT EXISTS idx_provider_id (provider_id);

ALTER TABLE pool_providers
  DROP INDEX IF EXISTS idx_workspace_id,
  ADD INDEX IF NOT EXISTS idx_provider_id (provider_id);

-- Step 11: Drop the workspaces table if it exists (no longer needed)
DROP TABLE IF EXISTS workspaces;

-- Step 12: Drop views that reference workspace
DROP VIEW IF EXISTS workspace_pool_performance_view;
DROP VIEW IF EXISTS pool_workspace_health;

-- Step 13: Update pool_statistics to remove workspace references
ALTER TABLE pool_statistics
  DROP COLUMN IF EXISTS workspace_count;