-- Recreate providers table with correct structure

-- Save existing data
CREATE TEMPORARY TABLE providers_backup AS SELECT * FROM providers;

-- Drop and recreate the table with correct structure
DROP TABLE providers;

CREATE TABLE providers (
    id VARCHAR(255) PRIMARY KEY,
    display_name VARCHAR(255) NOT NULL,
    domain VARCHAR(255) NOT NULL,
    rate_limit_workspace_daily INT DEFAULT 2000,
    rate_limit_per_user_daily INT DEFAULT 100,
    rate_limit_custom_users JSON,
    provider_type ENUM('gmail', 'mailgun', 'mandrill', 'sendgrid', 'ses') NOT NULL,
    provider_config JSON,
    service_account_json TEXT,
    enabled BOOLEAN DEFAULT true,
    priority INT DEFAULT 10,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_domain (domain),
    INDEX idx_enabled (enabled),
    INDEX idx_provider_type (provider_type),
    UNIQUE KEY uk_id_provider (id, provider_type)
);

-- Restore data from backup, using workspace_id as the new id
INSERT INTO providers (id, provider_type, enabled, priority, created_at, updated_at, display_name, domain)
SELECT 
    workspace_id as id,
    provider_type,
    enabled,
    priority,
    created_at,
    updated_at,
    COALESCE(workspace_id, 'default') as display_name,
    CONCAT(workspace_id, '.com') as domain
FROM providers_backup;

DROP TEMPORARY TABLE providers_backup;