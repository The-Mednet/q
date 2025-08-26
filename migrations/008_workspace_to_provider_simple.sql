-- Simple migration: Rename workspace_id to provider_id in essential tables
-- Run this FIRST before the full cleanup

-- Step 1: Update messages table
ALTER TABLE messages 
  CHANGE COLUMN workspace_id provider_id VARCHAR(255);

-- Step 2: Update recipients table  
ALTER TABLE recipients
  CHANGE COLUMN workspace_id provider_id VARCHAR(255) NOT NULL;

-- Step 3: Update rate_limit_usage if it has workspace_id
ALTER TABLE rate_limit_usage
  CHANGE COLUMN workspace_id provider_id VARCHAR(255);

-- Step 4: Rename workspace tables to provider tables (preserving data)
RENAME TABLE workspace_providers TO providers;
RENAME TABLE workspace_rate_limits TO provider_rate_limits;
RENAME TABLE workspace_user_rate_limits TO provider_user_rate_limits;

-- Step 5: Update foreign key column names in renamed tables
ALTER TABLE provider_rate_limits
  CHANGE COLUMN workspace_id provider_id VARCHAR(255);

ALTER TABLE provider_user_rate_limits
  CHANGE COLUMN workspace_id provider_id VARCHAR(255);

-- Step 6: Update provider_header_rewrite_rules
ALTER TABLE provider_header_rewrite_rules
  CHANGE COLUMN workspace_id provider_id VARCHAR(255);

-- Step 7: For load balancing, rename pool_workspaces to pool_providers
RENAME TABLE pool_workspaces TO pool_providers;

ALTER TABLE pool_providers
  CHANGE COLUMN workspace_id provider_id VARCHAR(255);

-- Step 8: Update provider_selections
ALTER TABLE provider_selections
  CHANGE COLUMN workspace_id provider_id VARCHAR(255);

-- Step 9: Update indexes
ALTER TABLE messages
  DROP INDEX IF EXISTS idx_workspace_id,
  ADD INDEX idx_provider_id (provider_id);

ALTER TABLE recipients
  DROP INDEX IF EXISTS idx_workspace_status,
  DROP INDEX IF EXISTS uk_email_workspace,
  ADD INDEX idx_provider_status (provider_id, status),
  ADD UNIQUE KEY uk_email_provider (email_address, provider_id);

ALTER TABLE provider_rate_limits
  DROP INDEX IF EXISTS idx_workspace_id,
  ADD INDEX idx_provider_id (provider_id);

ALTER TABLE pool_providers
  DROP INDEX IF EXISTS idx_workspace_id,
  ADD INDEX idx_provider_id (provider_id);