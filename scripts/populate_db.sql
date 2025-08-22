-- Script to populate the database with standard providers
-- Run this with: mysql -h localhost -u relay -prelay relay < scripts/populate_db.sql

-- Clear existing data (optional - comment out if you want to keep existing data)
-- DELETE FROM workspaces;

-- Insert Gmail workspaces
INSERT INTO workspaces (
    id, display_name, domain, 
    rate_limit_workspace_daily, rate_limit_per_user_daily,
    rate_limit_custom_users,
    provider_type, provider_config,
    enabled, created_at, updated_at
) VALUES 
(
    'joinmednet',
    'JoinMednet Workspace',
    'joinmednet.org',
    500, 100,
    '{"vip@joinmednet.org": 5000, "bulk@joinmednet.org": 500}',
    'gmail',
    '{"service_account_file": "", "enabled": true, "default_sender": "brian@joinmednet.org", "enable_webhooks": false}',
    1,
    NOW(), NOW()
),
(
    'mednetmail',
    'MednetMail Workspace',
    'mednetmail.org',
    500, 100,
    '{"vip@joinmednet.org": 5000, "bulk@joinmednet.org": 500}',
    'gmail',
    '{"service_account_file": "", "enabled": true, "default_sender": "brian@mednetmail.org", "enable_webhooks": false}',
    1,
    NOW(), NOW()
),
(
    'mailgun-primary',
    'Mailgun',
    'mail.joinmednet.org',
    1000, 100,
    '{"brian@mail.joinmednet.org": 2000}',
    'mailgun',
    '{"api_key": "YOUR_MAILGUN_API_KEY_HERE", "domain": "mail.joinmednet.org", "base_url": "https://api.mailgun.net/v3", "region": "us", "enabled": true, "enable_webhooks": false, "tracking": {"opens": true, "clicks": true, "unsubscribe": true}}',
    1,
    NOW(), NOW()
),
(
    'mandrill-transactional',
    'Mandrill',
    'themednet.org',
    10000, 500,
    NULL,
    'mandrill',
    '{"api_key": "YOUR_MANDRILL_API_KEY_HERE", "base_url": "https://mandrillapp.com/api/1.0", "enabled": true, "enable_webhooks": false}',
    1,
    NOW(), NOW()
)
ON DUPLICATE KEY UPDATE
    display_name = VALUES(display_name),
    domain = VALUES(domain),
    rate_limit_workspace_daily = VALUES(rate_limit_workspace_daily),
    rate_limit_per_user_daily = VALUES(rate_limit_per_user_daily),
    updated_at = NOW();

-- Add additional domains for workspaces that support multiple domains
-- This would require a separate domains table if we want to properly support multiple domains per workspace
-- For now, we'll keep the primary domain in the main table

SELECT 'Database populated with standard providers' as status;
SELECT COUNT(*) as total_workspaces FROM workspaces;