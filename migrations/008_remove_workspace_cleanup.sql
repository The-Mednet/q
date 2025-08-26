-- Migration: Remove all workspace references and clean up unused tables
-- WARNING: Backup your database before running this migration!

-- Step 1: Update messages table - rename workspace_id to provider_id
ALTER TABLE messages 
  CHANGE COLUMN workspace_id provider_id VARCHAR(255),
  DROP INDEX idx_workspace_id,
  ADD INDEX idx_provider_id (provider_id);

-- Step 2: Drop all workspace-related tables that are no longer needed
DROP TABLE IF EXISTS workspaces;
DROP TABLE IF EXISTS workspace_providers;
DROP TABLE IF EXISTS workspace_rate_limits;
DROP TABLE IF EXISTS workspace_user_rate_limits;
DROP TABLE IF EXISTS pool_workspaces;
DROP TABLE IF EXISTS workspace_pool_performance_view;

-- Step 3: Update recipients table - rename workspace_id to provider_id
ALTER TABLE recipients
  CHANGE COLUMN workspace_id provider_id VARCHAR(255) NOT NULL,
  DROP INDEX uk_email_workspace,
  DROP INDEX idx_workspace_status,
  ADD UNIQUE KEY uk_email_provider (email_address, provider_id),
  ADD INDEX idx_provider_status (provider_id, status);

-- Step 4: Drop unused/legacy tables
DROP TABLE IF EXISTS gmail_provider_configs;
DROP TABLE IF EXISTS mailgun_provider_configs;
DROP TABLE IF EXISTS mandrill_provider_configs;
DROP TABLE IF EXISTS provider_credentials;
DROP TABLE IF EXISTS credential_audit_log;
DROP TABLE IF EXISTS load_balancing_selections;
DROP TABLE IF EXISTS pool_members;
DROP TABLE IF EXISTS pool_metrics;
DROP TABLE IF EXISTS pool_workspace_health;
DROP TABLE IF EXISTS pool_performance_view;
DROP TABLE IF EXISTS pool_status_view;
DROP TABLE IF EXISTS recipient_lists;
DROP TABLE IF EXISTS recipient_list_members;

-- Step 5: Create new provider configuration tables
CREATE TABLE IF NOT EXISTS providers (
    id VARCHAR(36) PRIMARY KEY,
    type ENUM('gmail', 'mailgun', 'mandrill') NOT NULL,
    display_name VARCHAR(255) NOT NULL,
    domain VARCHAR(255) NOT NULL,
    enabled BOOLEAN DEFAULT true,
    priority INT DEFAULT 100,
    config JSON,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_domain (domain),
    INDEX idx_enabled (enabled),
    INDEX idx_type (type)
);

-- Step 6: Create provider rate limits table
CREATE TABLE IF NOT EXISTS provider_rate_limits (
    id VARCHAR(36) PRIMARY KEY,
    provider_id VARCHAR(36) NOT NULL,
    daily_limit INT NOT NULL DEFAULT 2000,
    hourly_limit INT NOT NULL DEFAULT 100,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (provider_id) REFERENCES providers(id) ON DELETE CASCADE,
    UNIQUE KEY uk_provider_id (provider_id)
);

-- Step 7: Create provider user rate limits table
CREATE TABLE IF NOT EXISTS provider_user_rate_limits (
    id VARCHAR(36) PRIMARY KEY,
    provider_id VARCHAR(36) NOT NULL,
    user_email VARCHAR(255) NOT NULL,
    daily INT NOT NULL DEFAULT 100,
    hourly INT NOT NULL DEFAULT 10,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (provider_id) REFERENCES providers(id) ON DELETE CASCADE,
    UNIQUE KEY uk_provider_user (provider_id, user_email),
    INDEX idx_user_email (user_email)
);

-- Step 8: Update provider_header_rewrite_rules if it exists
ALTER TABLE provider_header_rewrite_rules
  ADD COLUMN IF NOT EXISTS provider_id VARCHAR(36),
  ADD FOREIGN KEY (provider_id) REFERENCES providers(id) ON DELETE CASCADE;

-- Step 9: Update load balancing pools to reference providers
CREATE TABLE IF NOT EXISTS pool_providers (
    id VARCHAR(36) PRIMARY KEY,
    pool_id VARCHAR(36) NOT NULL,
    provider_id VARCHAR(36) NOT NULL,
    weight INT DEFAULT 1,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (pool_id) REFERENCES load_balancing_pools(id) ON DELETE CASCADE,
    FOREIGN KEY (provider_id) REFERENCES providers(id) ON DELETE CASCADE,
    UNIQUE KEY uk_pool_provider (pool_id, provider_id)
);

-- Step 10: Update provider_selections table
ALTER TABLE provider_selections
  ADD COLUMN IF NOT EXISTS provider_id VARCHAR(36),
  ADD INDEX idx_provider_id (provider_id);

-- Step 11: Update rate_limit_usage table
ALTER TABLE rate_limit_usage
  ADD COLUMN IF NOT EXISTS provider_id VARCHAR(36),
  ADD INDEX idx_provider_id (provider_id);

-- Step 12: Clean up pool_statistics table
ALTER TABLE pool_statistics
  DROP COLUMN IF EXISTS workspace_count;

-- Add notification_id column to messages if it doesn't exist
ALTER TABLE messages
  ADD COLUMN IF NOT EXISTS notification_id VARCHAR(255) AFTER campaign_id,
  ADD INDEX idx_notification_id (notification_id);

-- Add invitation columns from X-MC-Metadata parsing
ALTER TABLE messages
  ADD COLUMN IF NOT EXISTS invitation_id VARCHAR(255),
  ADD COLUMN IF NOT EXISTS email_type VARCHAR(255),
  ADD COLUMN IF NOT EXISTS invitation_dispatch_id VARCHAR(255),
  ADD INDEX idx_invitation_id (invitation_id),
  ADD INDEX idx_email_type (email_type);

-- Final cleanup: Set any NULL provider_id values to a default or remove them
-- UPDATE messages SET provider_id = 'default' WHERE provider_id IS NULL;
-- DELETE FROM messages WHERE provider_id IS NULL;

-- Note: After running this migration, you'll need to:
-- 1. Migrate any existing workspace data to the new providers table
-- 2. Update your application code to use provider_id instead of workspace_id
-- 3. Update any API endpoints that reference workspaces