-- Provider Management Schema
-- This migration adds support for database-backed provider configuration

USE relay;

-- Workspaces table to replace file-based workspace.json
CREATE TABLE IF NOT EXISTS workspaces (
    id VARCHAR(255) PRIMARY KEY,
    display_name VARCHAR(255) NOT NULL,
    domain VARCHAR(255) NOT NULL,
    rate_limit_workspace_daily INT NOT NULL DEFAULT 2000,
    rate_limit_per_user_daily INT NOT NULL DEFAULT 100,
    rate_limit_custom_users JSON,  -- Map of email -> limit
    provider_type ENUM('gmail', 'mailgun', 'mandrill') NOT NULL,
    provider_config JSON NOT NULL,  -- Provider-specific configuration
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    
    UNIQUE KEY uk_domain (domain),
    INDEX idx_enabled (enabled),
    INDEX idx_provider_type (provider_type)
);

-- Load balancing pools for routing emails
CREATE TABLE IF NOT EXISTS load_balancing_pools (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    algorithm ENUM('round_robin', 'weighted', 'least_connections', 'random') NOT NULL DEFAULT 'round_robin',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    
    UNIQUE KEY uk_name (name),
    INDEX idx_enabled (enabled)
);

-- Pool members (providers in a pool)
CREATE TABLE IF NOT EXISTS pool_members (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    pool_id VARCHAR(36) NOT NULL,
    workspace_id VARCHAR(255) NOT NULL,
    weight INT NOT NULL DEFAULT 1,  -- For weighted algorithm
    priority INT NOT NULL DEFAULT 0,  -- Lower priority = preferred
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    
    UNIQUE KEY uk_pool_workspace (pool_id, workspace_id),
    INDEX idx_pool_enabled (pool_id, enabled),
    
    FOREIGN KEY (pool_id) REFERENCES load_balancing_pools(id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
);

-- Pool statistics for monitoring
CREATE TABLE IF NOT EXISTS pool_statistics (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    pool_id VARCHAR(36) NOT NULL,
    workspace_id VARCHAR(255) NOT NULL,
    total_requests BIGINT NOT NULL DEFAULT 0,
    successful_requests BIGINT NOT NULL DEFAULT 0,
    failed_requests BIGINT NOT NULL DEFAULT 0,
    avg_response_time_ms INT NULL,
    last_used_at TIMESTAMP NULL,
    hour_bucket TIMESTAMP NOT NULL,  -- For hourly aggregation
    
    UNIQUE KEY uk_pool_workspace_hour (pool_id, workspace_id, hour_bucket),
    INDEX idx_pool_hour (pool_id, hour_bucket),
    INDEX idx_workspace_hour (workspace_id, hour_bucket),
    
    FOREIGN KEY (pool_id) REFERENCES load_balancing_pools(id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
);

-- Provider selection history for debugging
CREATE TABLE IF NOT EXISTS provider_selections (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    pool_id VARCHAR(36) NOT NULL,
    workspace_id VARCHAR(255) NOT NULL,
    message_id VARCHAR(36) NULL,
    algorithm_used VARCHAR(50) NOT NULL,
    success BOOLEAN NOT NULL,
    response_time_ms INT NULL,
    error_message TEXT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    
    INDEX idx_pool_created (pool_id, created_at),
    INDEX idx_workspace_created (workspace_id, created_at),
    INDEX idx_message_id (message_id),
    INDEX idx_created_at (created_at),
    
    FOREIGN KEY (pool_id) REFERENCES load_balancing_pools(id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
    FOREIGN KEY (message_id) REFERENCES messages(id) ON DELETE SET NULL
);

-- Provider health checks
CREATE TABLE IF NOT EXISTS provider_health (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    workspace_id VARCHAR(255) NOT NULL,
    healthy BOOLEAN NOT NULL,
    last_check_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    error_message TEXT NULL,
    response_time_ms INT NULL,
    consecutive_failures INT NOT NULL DEFAULT 0,
    
    UNIQUE KEY uk_workspace (workspace_id),
    INDEX idx_healthy (healthy),
    INDEX idx_last_check (last_check_at),
    
    FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
);

-- API keys for provider authentication (encrypted)
CREATE TABLE IF NOT EXISTS provider_credentials (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    workspace_id VARCHAR(255) NOT NULL,
    credential_type VARCHAR(50) NOT NULL,  -- 'api_key', 'service_account', etc.
    encrypted_value TEXT NOT NULL,  -- AES encrypted
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    
    UNIQUE KEY uk_workspace_type (workspace_id, credential_type),
    
    FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
);

-- Rate limit tracking per workspace
CREATE TABLE IF NOT EXISTS rate_limit_usage (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    workspace_id VARCHAR(255) NOT NULL,
    user_email VARCHAR(320) NULL,  -- NULL for workspace-level tracking
    date_bucket DATE NOT NULL,
    hour_bucket INT NOT NULL,  -- 0-23
    message_count INT NOT NULL DEFAULT 0,
    
    UNIQUE KEY uk_tracking (workspace_id, user_email, date_bucket, hour_bucket),
    INDEX idx_workspace_date (workspace_id, date_bucket),
    INDEX idx_user_date (user_email, date_bucket),
    
    FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
);

-- Migrate existing messages to reference workspaces
ALTER TABLE messages 
    ADD COLUMN provider_id VARCHAR(255) NULL AFTER workspace_id,
    ADD COLUMN pool_id VARCHAR(36) NULL AFTER provider_id,
    ADD INDEX idx_provider_id (provider_id),
    ADD INDEX idx_pool_id (pool_id);

-- Add provider tracking to messages
ALTER TABLE messages
    ADD COLUMN sent_at TIMESTAMP NULL AFTER processed_at,
    ADD COLUMN provider_response JSON NULL AFTER error;

-- Create default workspace entries from environment
-- These would be populated by the application on startup from workspace.json
INSERT IGNORE INTO workspaces (id, display_name, domain, provider_type, provider_config, enabled)
VALUES 
    ('default-gmail', 'Default Gmail', 'example.com', 'gmail', '{}', FALSE),
    ('default-mailgun', 'Default Mailgun', 'mail.example.com', 'mailgun', '{}', FALSE);

-- Create sample load balancing pool
INSERT IGNORE INTO load_balancing_pools (id, name, algorithm, enabled)
VALUES 
    ('default-pool', 'Default Pool', 'round_robin', FALSE);