-- Provider Management API Schema
-- This migration creates the correct database schema that matches the API expectations
-- Fixes schema inconsistencies identified in the codebase audit

USE relay;

-- Create workspaces table if it doesn't exist (normalized from previous migration)
CREATE TABLE IF NOT EXISTS workspaces (
    id VARCHAR(255) PRIMARY KEY,
    display_name VARCHAR(255) NOT NULL,
    domain VARCHAR(255) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    
    UNIQUE KEY uk_domain (domain),
    INDEX idx_display_name (display_name)
);

-- Workspace providers table (matches API expectations)
CREATE TABLE IF NOT EXISTS workspace_providers (
    id INT AUTO_INCREMENT PRIMARY KEY,
    workspace_id VARCHAR(255) NOT NULL,
    provider_type ENUM('gmail', 'mailgun', 'mandrill') NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    priority INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    
    INDEX idx_workspace_id (workspace_id),
    INDEX idx_provider_type (provider_type),
    INDEX idx_enabled (enabled),
    INDEX idx_priority (priority),
    
    -- Foreign key constraint with proper error handling
    CONSTRAINT fk_workspace_providers_workspace 
        FOREIGN KEY (workspace_id) REFERENCES workspaces(id) 
        ON DELETE CASCADE ON UPDATE CASCADE
);

-- Gmail provider configuration table
CREATE TABLE IF NOT EXISTS gmail_provider_configs (
    id INT AUTO_INCREMENT PRIMARY KEY,
    provider_id INT NOT NULL,
    service_account_file VARCHAR(500) NOT NULL,
    default_sender VARCHAR(320) NOT NULL,  -- RFC 5321 max email length
    delegated_user VARCHAR(320) NULL,
    scopes JSON NOT NULL DEFAULT '["https://www.googleapis.com/auth/gmail.send"]',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    
    INDEX idx_provider_id (provider_id),
    INDEX idx_default_sender (default_sender),
    
    -- Foreign key constraint
    CONSTRAINT fk_gmail_configs_provider 
        FOREIGN KEY (provider_id) REFERENCES workspace_providers(id) 
        ON DELETE CASCADE ON UPDATE CASCADE
);

-- Mailgun provider configuration table
CREATE TABLE IF NOT EXISTS mailgun_provider_configs (
    id INT AUTO_INCREMENT PRIMARY KEY,
    provider_id INT NOT NULL,
    api_key VARCHAR(500) NOT NULL,
    domain VARCHAR(255) NOT NULL,
    base_url VARCHAR(500) NOT NULL DEFAULT 'https://api.mailgun.net/v3',
    track_opens BOOLEAN NOT NULL DEFAULT FALSE,
    track_clicks BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    
    INDEX idx_provider_id (provider_id),
    INDEX idx_domain (domain),
    
    -- Foreign key constraint
    CONSTRAINT fk_mailgun_configs_provider 
        FOREIGN KEY (provider_id) REFERENCES workspace_providers(id) 
        ON DELETE CASCADE ON UPDATE CASCADE
);

-- Mandrill provider configuration table
CREATE TABLE IF NOT EXISTS mandrill_provider_configs (
    id INT AUTO_INCREMENT PRIMARY KEY,
    provider_id INT NOT NULL,
    api_key VARCHAR(500) NOT NULL,
    base_url VARCHAR(500) NOT NULL DEFAULT 'https://mandrillapp.com/api/1.0',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    
    INDEX idx_provider_id (provider_id),
    
    -- Foreign key constraint
    CONSTRAINT fk_mandrill_configs_provider 
        FOREIGN KEY (provider_id) REFERENCES workspace_providers(id) 
        ON DELETE CASCADE ON UPDATE CASCADE
);

-- Workspace rate limits table
CREATE TABLE IF NOT EXISTS workspace_rate_limits (
    workspace_id VARCHAR(255) PRIMARY KEY,
    daily INT NOT NULL DEFAULT 2000,
    hourly INT NOT NULL DEFAULT 200,
    per_user_daily INT NOT NULL DEFAULT 100,
    per_user_hourly INT NOT NULL DEFAULT 10,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    
    -- Foreign key constraint
    CONSTRAINT fk_workspace_rate_limits_workspace 
        FOREIGN KEY (workspace_id) REFERENCES workspaces(id) 
        ON DELETE CASCADE ON UPDATE CASCADE
);

-- Workspace user rate limits table
CREATE TABLE IF NOT EXISTS workspace_user_rate_limits (
    id INT AUTO_INCREMENT PRIMARY KEY,
    workspace_id VARCHAR(255) NOT NULL,
    user_email VARCHAR(320) NOT NULL,  -- RFC 5321 max email length
    daily INT NOT NULL DEFAULT 100,
    hourly INT NOT NULL DEFAULT 10,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    
    UNIQUE KEY uk_workspace_user (workspace_id, user_email),
    INDEX idx_workspace_id (workspace_id),
    INDEX idx_user_email (user_email),
    
    -- Foreign key constraint
    CONSTRAINT fk_workspace_user_rate_limits_workspace 
        FOREIGN KEY (workspace_id) REFERENCES workspaces(id) 
        ON DELETE CASCADE ON UPDATE CASCADE
);

-- Provider header rewrite rules table
CREATE TABLE IF NOT EXISTS provider_header_rewrite_rules (
    id INT AUTO_INCREMENT PRIMARY KEY,
    provider_id INT NOT NULL,
    header_name VARCHAR(255) NOT NULL,
    action ENUM('add', 'replace', 'remove') NOT NULL,
    value TEXT NULL,
    condition TEXT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    
    INDEX idx_provider_id (provider_id),
    INDEX idx_header_name (header_name),
    INDEX idx_enabled (enabled),
    
    -- Foreign key constraint
    CONSTRAINT fk_header_rewrite_rules_provider 
        FOREIGN KEY (provider_id) REFERENCES workspace_providers(id) 
        ON DELETE CASCADE ON UPDATE CASCADE
);

-- Add foreign key constraints to existing tables if they don't exist
-- Recipients table constraint
SET @constraint_exists = (
    SELECT COUNT(*) 
    FROM information_schema.TABLE_CONSTRAINTS 
    WHERE CONSTRAINT_SCHEMA = 'relay' 
    AND TABLE_NAME = 'recipients' 
    AND CONSTRAINT_NAME = 'fk_recipients_workspace'
);

SET @sql = IF(
    @constraint_exists = 0,
    'ALTER TABLE recipients ADD CONSTRAINT fk_recipients_workspace FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE ON UPDATE CASCADE',
    'SELECT "Foreign key constraint already exists for recipients.workspace_id"'
);

PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- Message recipients table constraint
SET @constraint_exists = (
    SELECT COUNT(*) 
    FROM information_schema.TABLE_CONSTRAINTS 
    WHERE CONSTRAINT_SCHEMA = 'relay' 
    AND TABLE_NAME = 'message_recipients' 
    AND CONSTRAINT_NAME = 'fk_message_recipients_workspace'
);

-- Add workspace_id to message_recipients if it doesn't exist
SET @column_exists = (
    SELECT COUNT(*) 
    FROM information_schema.COLUMNS 
    WHERE TABLE_SCHEMA = 'relay' 
    AND TABLE_NAME = 'message_recipients' 
    AND COLUMN_NAME = 'workspace_id'
);

SET @sql = IF(
    @column_exists = 0,
    'ALTER TABLE message_recipients ADD COLUMN workspace_id VARCHAR(255) NULL AFTER recipient_id, ADD INDEX idx_workspace_id (workspace_id)',
    'SELECT "workspace_id column already exists in message_recipients"'
);

PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- Add foreign key constraint if column exists and constraint doesn't
SET @constraint_exists = (
    SELECT COUNT(*) 
    FROM information_schema.TABLE_CONSTRAINTS 
    WHERE CONSTRAINT_SCHEMA = 'relay' 
    AND TABLE_NAME = 'message_recipients' 
    AND CONSTRAINT_NAME = 'fk_message_recipients_workspace'
);

SET @column_exists = (
    SELECT COUNT(*) 
    FROM information_schema.COLUMNS 
    WHERE TABLE_SCHEMA = 'relay' 
    AND TABLE_NAME = 'message_recipients' 
    AND COLUMN_NAME = 'workspace_id'
);

SET @sql = IF(
    @constraint_exists = 0 AND @column_exists > 0,
    'ALTER TABLE message_recipients ADD CONSTRAINT fk_message_recipients_workspace FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE SET NULL ON UPDATE CASCADE',
    'SELECT "Foreign key constraint handling completed for message_recipients.workspace_id"'
);

PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- Recipient lists table constraint
SET @constraint_exists = (
    SELECT COUNT(*) 
    FROM information_schema.TABLE_CONSTRAINTS 
    WHERE CONSTRAINT_SCHEMA = 'relay' 
    AND TABLE_NAME = 'recipient_lists' 
    AND CONSTRAINT_NAME = 'fk_recipient_lists_workspace'
);

SET @sql = IF(
    @constraint_exists = 0,
    'ALTER TABLE recipient_lists ADD CONSTRAINT fk_recipient_lists_workspace FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE ON UPDATE CASCADE',
    'SELECT "Foreign key constraint already exists for recipient_lists.workspace_id"'
);

PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- Messages table workspace constraint
SET @constraint_exists = (
    SELECT COUNT(*) 
    FROM information_schema.TABLE_CONSTRAINTS 
    WHERE CONSTRAINT_SCHEMA = 'relay' 
    AND TABLE_NAME = 'messages' 
    AND CONSTRAINT_NAME = 'fk_messages_workspace'
);

SET @sql = IF(
    @constraint_exists = 0,
    'ALTER TABLE messages ADD CONSTRAINT fk_messages_workspace FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE SET NULL ON UPDATE CASCADE',
    'SELECT "Foreign key constraint already exists for messages.workspace_id"'
);

PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;