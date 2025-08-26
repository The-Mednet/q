-- Compatible migration for MySQL - Handle workspace to provider renaming

-- Step 1: Messages table - copy data and clean up
UPDATE messages 
SET provider_id = workspace_id 
WHERE provider_id IS NULL AND workspace_id IS NOT NULL;

ALTER TABLE messages DROP COLUMN workspace_id;
ALTER TABLE messages DROP INDEX idx_workspace_id;

-- Step 2: Recipients table
ALTER TABLE recipients CHANGE COLUMN workspace_id provider_id VARCHAR(255) NOT NULL;
ALTER TABLE recipients DROP INDEX idx_workspace_status;
ALTER TABLE recipients DROP INDEX uk_email_workspace;
ALTER TABLE recipients ADD INDEX idx_provider_status (provider_id, status);
ALTER TABLE recipients ADD UNIQUE KEY uk_email_provider (email_address, provider_id);

-- Step 3: Check if these tables exist and rename them
RENAME TABLE workspace_providers TO providers;
RENAME TABLE workspace_rate_limits TO provider_rate_limits;
RENAME TABLE workspace_user_rate_limits TO provider_user_rate_limits;
RENAME TABLE pool_workspaces TO pool_providers;

-- Step 4: Update column names in renamed tables
ALTER TABLE provider_rate_limits CHANGE COLUMN workspace_id provider_id VARCHAR(255);
ALTER TABLE provider_user_rate_limits CHANGE COLUMN workspace_id provider_id VARCHAR(255);
ALTER TABLE pool_providers CHANGE COLUMN workspace_id provider_id VARCHAR(255);

-- Step 5: Update other tables that have workspace_id
-- Check if rate_limit_usage has workspace_id
SELECT COUNT(*) FROM information_schema.COLUMNS 
WHERE TABLE_SCHEMA = 'relay' 
AND TABLE_NAME = 'rate_limit_usage' 
AND COLUMN_NAME = 'workspace_id';
-- If it exists, update it (run manually if needed):
-- ALTER TABLE rate_limit_usage CHANGE COLUMN workspace_id provider_id VARCHAR(255);

-- Step 6: Update provider_selections if it has workspace_id
SELECT COUNT(*) FROM information_schema.COLUMNS 
WHERE TABLE_SCHEMA = 'relay' 
AND TABLE_NAME = 'provider_selections' 
AND COLUMN_NAME = 'workspace_id';
-- If it exists, update it (run manually if needed):
-- ALTER TABLE provider_selections CHANGE COLUMN workspace_id provider_id VARCHAR(255);

-- Step 7: Drop workspace-related tables and views
DROP TABLE IF EXISTS workspaces;
DROP VIEW IF EXISTS workspace_pool_performance_view;
DROP VIEW IF EXISTS pool_workspace_health;

-- Step 8: Clean up unused tables
DROP TABLE IF EXISTS gmail_provider_configs;
DROP TABLE IF EXISTS mailgun_provider_configs;
DROP TABLE IF EXISTS mandrill_provider_configs;
DROP TABLE IF EXISTS provider_credentials;
DROP TABLE IF EXISTS credential_audit_log;
DROP TABLE IF EXISTS pool_members;
DROP TABLE IF EXISTS pool_metrics;
DROP TABLE IF EXISTS recipient_lists;
DROP TABLE IF EXISTS recipient_list_members;