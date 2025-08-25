-- Essential provider management tables for complete workspace configuration
-- This migration adds the missing tables needed to support all workspace.json settings

-- 1. Rate limits configuration
CREATE TABLE IF NOT EXISTS workspace_rate_limits (
    workspace_id VARCHAR(255) PRIMARY KEY,
    workspace_daily INT NOT NULL DEFAULT 500 COMMENT 'Daily limit for entire workspace',
    per_user_daily INT NOT NULL DEFAULT 100 COMMENT 'Default daily limit per user',
    burst_limit INT DEFAULT 50 COMMENT 'Maximum burst rate',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_rate_limits_workspace FOREIGN KEY (workspace_id) 
        REFERENCES workspaces(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='Workspace rate limiting configuration';

-- 2. Custom user rate limits
CREATE TABLE IF NOT EXISTS workspace_user_rate_limits (
    id INT AUTO_INCREMENT PRIMARY KEY,
    workspace_id VARCHAR(255) NOT NULL,
    email_address VARCHAR(255) NOT NULL,
    daily_limit INT NOT NULL COMMENT 'Custom daily limit for this user',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uk_workspace_user (workspace_id, email_address),
    INDEX idx_user_email (email_address),
    CONSTRAINT fk_user_limits_workspace FOREIGN KEY (workspace_id) 
        REFERENCES workspaces(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='Custom per-user rate limits';

-- 3. Provider configurations registry
CREATE TABLE IF NOT EXISTS workspace_providers (
    id INT AUTO_INCREMENT PRIMARY KEY,
    workspace_id VARCHAR(255) NOT NULL,
    provider_type ENUM('gmail', 'mailgun', 'mandrill', 'sendgrid', 'ses') NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    priority INT DEFAULT 10 COMMENT 'Provider priority for load balancing',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uk_workspace_provider (workspace_id, provider_type),
    INDEX idx_provider_type (provider_type),
    INDEX idx_enabled (enabled),
    CONSTRAINT fk_providers_workspace FOREIGN KEY (workspace_id) 
        REFERENCES workspaces(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='Provider configurations for workspaces';

-- 4. Gmail provider configuration
CREATE TABLE IF NOT EXISTS gmail_provider_configs (
    provider_id INT PRIMARY KEY,
    service_account_file VARCHAR(500) NULL COMMENT 'Path to service account JSON file',
    service_account_env VARCHAR(255) NULL COMMENT 'Environment variable containing service account',
    default_sender VARCHAR(255) NOT NULL COMMENT 'Default sender email address',
    impersonate_user VARCHAR(255) NULL COMMENT 'User to impersonate for domain-wide delegation',
    scopes JSON NULL COMMENT 'OAuth scopes required',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_gmail_provider FOREIGN KEY (provider_id) 
        REFERENCES workspace_providers(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='Gmail-specific provider configuration';

-- 5. Mailgun provider configuration
CREATE TABLE IF NOT EXISTS mailgun_provider_configs (
    provider_id INT PRIMARY KEY,
    api_key_env VARCHAR(255) NULL COMMENT 'Environment variable for API key',
    domain VARCHAR(255) NOT NULL COMMENT 'Mailgun sending domain',
    base_url VARCHAR(500) DEFAULT 'https://api.mailgun.net/v3' COMMENT 'Mailgun API base URL',
    region VARCHAR(50) DEFAULT 'us' COMMENT 'Mailgun region (us or eu)',
    track_opens BOOLEAN DEFAULT TRUE COMMENT 'Track email opens',
    track_clicks BOOLEAN DEFAULT TRUE COMMENT 'Track link clicks',
    track_unsubscribes BOOLEAN DEFAULT TRUE COMMENT 'Track unsubscribes',
    webhook_signing_key VARCHAR(255) NULL COMMENT 'Webhook signature verification key',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_mailgun_provider FOREIGN KEY (provider_id) 
        REFERENCES workspace_providers(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='Mailgun-specific provider configuration';

-- 6. Mandrill provider configuration
CREATE TABLE IF NOT EXISTS mandrill_provider_configs (
    provider_id INT PRIMARY KEY,
    api_key_env VARCHAR(255) NULL COMMENT 'Environment variable for API key',
    subaccount VARCHAR(255) NULL COMMENT 'Mandrill subaccount',
    default_from_name VARCHAR(255) NULL COMMENT 'Default sender name',
    default_from_email VARCHAR(255) NOT NULL COMMENT 'Default sender email',
    track_opens BOOLEAN DEFAULT TRUE COMMENT 'Track email opens',
    track_clicks BOOLEAN DEFAULT TRUE COMMENT 'Track link clicks',
    default_tags JSON NULL COMMENT 'Default tags for all emails',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_mandrill_provider FOREIGN KEY (provider_id) 
        REFERENCES workspace_providers(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='Mandrill-specific provider configuration';

-- 7. Header rewrite rules
CREATE TABLE IF NOT EXISTS provider_header_rewrite_rules (
    id INT AUTO_INCREMENT PRIMARY KEY,
    provider_id INT NOT NULL,
    header_name VARCHAR(255) NOT NULL COMMENT 'Header name to rewrite',
    action ENUM('remove', 'replace', 'add') NOT NULL DEFAULT 'remove' COMMENT 'Rewrite action',
    new_value TEXT NULL COMMENT 'New header value (for replace/add actions)',
    priority INT DEFAULT 10 COMMENT 'Rule priority (lower = higher priority)',
    enabled BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_provider_header (provider_id, header_name),
    INDEX idx_priority (priority),
    CONSTRAINT fk_header_rules_provider FOREIGN KEY (provider_id) 
        REFERENCES workspace_providers(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='Header rewrite rules for providers';

-- Insert sample data for existing workspaces
-- This will populate the new tables based on existing workspaces

-- Insert rate limits for existing workspaces
INSERT IGNORE INTO workspace_rate_limits (workspace_id, workspace_daily, per_user_daily)
SELECT id, 
    CASE 
        WHEN id = 'mailgun-primary' THEN 1000
        ELSE 500
    END as workspace_daily,
    100 as per_user_daily
FROM workspaces;

-- Insert custom user rate limits based on workspace.json
INSERT IGNORE INTO workspace_user_rate_limits (workspace_id, email_address, daily_limit)
VALUES 
    ('joinmednet', 'vip@joinmednet.org', 5000),
    ('joinmednet', 'bulk@joinmednet.org', 500),
    ('mednetmail', 'vip@joinmednet.org', 5000),
    ('mednetmail', 'bulk@joinmednet.org', 500),
    ('mailgun-primary', 'brian@mail.joinmednet.org', 2000);

-- Insert provider configurations for existing workspaces
INSERT IGNORE INTO workspace_providers (workspace_id, provider_type, enabled, priority)
SELECT 
    w.id as workspace_id,
    w.provider_type,
    TRUE as enabled,
    CASE 
        WHEN w.provider_type = 'gmail' THEN 10
        WHEN w.provider_type = 'mailgun' THEN 20
        WHEN w.provider_type = 'mandrill' THEN 30
        ELSE 40
    END as priority
FROM workspaces w
WHERE w.provider_type IS NOT NULL;

-- Insert Gmail configurations
INSERT IGNORE INTO gmail_provider_configs (provider_id, service_account_env, default_sender)
SELECT 
    wp.id,
    CASE 
        WHEN wp.workspace_id = 'joinmednet' THEN 'RELAY_JOINMEDNET_SERVICE_ACCOUNT'
        WHEN wp.workspace_id = 'mednetmail' THEN 'RELAY_MEDNETMAIL_SERVICE_ACCOUNT'
        ELSE NULL
    END as service_account_env,
    CASE 
        WHEN wp.workspace_id = 'joinmednet' THEN 'brian@joinmednet.org'
        WHEN wp.workspace_id = 'mednetmail' THEN 'brian@mednetmail.org'
        ELSE 'noreply@example.com'
    END as default_sender
FROM workspace_providers wp
WHERE wp.provider_type = 'gmail';

-- Insert Mailgun configuration
INSERT IGNORE INTO mailgun_provider_configs (provider_id, api_key_env, domain, base_url, region, track_opens, track_clicks, track_unsubscribes)
SELECT 
    wp.id,
    'MAILGUN_API_KEY' as api_key_env,
    'mail.joinmednet.org' as domain,
    'https://api.mailgun.net/v3' as base_url,
    'us' as region,
    TRUE as track_opens,
    TRUE as track_clicks,
    TRUE as track_unsubscribes
FROM workspace_providers wp
WHERE wp.provider_type = 'mailgun' AND wp.workspace_id = 'mailgun-primary';

-- Insert Mandrill configuration
INSERT IGNORE INTO mandrill_provider_configs (provider_id, api_key_env, default_from_email)
SELECT 
    wp.id,
    'MANDRILL_API_KEY' as api_key_env,
    'noreply@themednet.org' as default_from_email
FROM workspace_providers wp
WHERE wp.provider_type = 'mandrill';

-- Insert header rewrite rules for Gmail providers (List-Unsubscribe headers)
INSERT IGNORE INTO provider_header_rewrite_rules (provider_id, header_name, action, priority)
SELECT 
    wp.id,
    'List-Unsubscribe' as header_name,
    'remove' as action,
    10 as priority
FROM workspace_providers wp
WHERE wp.provider_type = 'gmail'
UNION ALL
SELECT 
    wp.id,
    'List-Unsubscribe-Post' as header_name,
    'remove' as action,
    20 as priority
FROM workspace_providers wp
WHERE wp.provider_type = 'gmail';

-- Insert header rewrite rules for Mailgun provider
INSERT IGNORE INTO provider_header_rewrite_rules (provider_id, header_name, action, new_value, priority)
SELECT 
    wp.id,
    'List-Unsubscribe' as header_name,
    'replace' as action,
    '<%unsubscribe_url%>' as new_value,
    10 as priority
FROM workspace_providers wp
WHERE wp.provider_type = 'mailgun'
UNION ALL
SELECT 
    wp.id,
    'List-Unsubscribe-Post' as header_name,
    'replace' as action,
    'List-Unsubscribe=One-Click' as new_value,
    20 as priority
FROM workspace_providers wp
WHERE wp.provider_type = 'mailgun';

-- Indexes are created automatically by the table definitions above

-- Verify the migration
SELECT 'Migration complete!' as status;
SELECT COUNT(*) as table_count FROM information_schema.tables 
WHERE table_schema = DATABASE() 
AND table_name IN ('workspace_rate_limits', 'workspace_user_rate_limits', 'workspace_providers', 
                   'gmail_provider_configs', 'mailgun_provider_configs', 'mandrill_provider_configs',
                   'provider_header_rewrite_rules');