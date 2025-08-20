-- Migration script for recipient tracking system
-- This script migrates existing message data to the new recipient tracking tables

-- First, create the new tables (these should already exist from schema.sql)
-- This is included here for reference and safety

-- Recipients tracking table
CREATE TABLE IF NOT EXISTS recipients (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    email_address VARCHAR(320) NOT NULL,                        -- RFC 5321 max email length
    workspace_id VARCHAR(255) NOT NULL,
    user_id VARCHAR(255) NULL,                                   -- Can be null for guest recipients
    campaign_id VARCHAR(255) NULL,                               -- Can be null for non-campaign emails
    first_name VARCHAR(100) NULL,
    last_name VARCHAR(100) NULL,
    status ENUM('ACTIVE', 'INACTIVE', 'BOUNCED', 'UNSUBSCRIBED') NOT NULL DEFAULT 'ACTIVE',
    opt_in_date TIMESTAMP NULL,
    opt_out_date TIMESTAMP NULL,
    bounce_count INT NOT NULL DEFAULT 0,
    last_bounce_date TIMESTAMP NULL,
    bounce_type ENUM('SOFT', 'HARD') NULL,
    metadata JSON,                                               -- Additional recipient data
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    
    -- Composite unique key to prevent duplicates within workspace
    UNIQUE KEY uk_email_workspace (email_address, workspace_id),
    
    -- Performance indexes
    INDEX idx_workspace_status (workspace_id, status),
    INDEX idx_campaign_id (campaign_id),
    INDEX idx_user_id (user_id),
    INDEX idx_status_created (status, created_at),
    INDEX idx_email_status (email_address, status),
    INDEX idx_bounce_tracking (status, bounce_count, last_bounce_date)
);

-- Message recipients junction table (tracks which recipients received which messages)
CREATE TABLE IF NOT EXISTS message_recipients (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    message_id VARCHAR(36) NOT NULL,
    recipient_id BIGINT NOT NULL,
    recipient_type ENUM('TO', 'CC', 'BCC') NOT NULL,
    delivery_status ENUM('PENDING', 'SENT', 'BOUNCED', 'FAILED', 'DEFERRED') NOT NULL DEFAULT 'PENDING',
    sent_at TIMESTAMP NULL,
    bounce_reason TEXT NULL,
    opens INT NOT NULL DEFAULT 0,
    clicks INT NOT NULL DEFAULT 0,
    last_open_at TIMESTAMP NULL,
    last_click_at TIMESTAMP NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    
    -- Composite unique key to prevent duplicate recipient records per message
    UNIQUE KEY uk_message_recipient (message_id, recipient_id),
    
    -- Performance indexes
    INDEX idx_message_id (message_id),
    INDEX idx_recipient_id (recipient_id),
    INDEX idx_delivery_status (delivery_status),
    INDEX idx_sent_at (sent_at),
    INDEX idx_engagement (opens, clicks, last_open_at),
    INDEX idx_message_status_type (message_id, delivery_status, recipient_type),
    
    -- Foreign keys
    FOREIGN KEY (message_id) REFERENCES messages(id) ON DELETE CASCADE,
    FOREIGN KEY (recipient_id) REFERENCES recipients(id) ON DELETE CASCADE
);

-- Recipient engagement events table (detailed tracking of opens, clicks, etc.)
CREATE TABLE IF NOT EXISTS recipient_events (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    message_recipient_id BIGINT NOT NULL,
    event_type ENUM('OPEN', 'CLICK', 'UNSUBSCRIBE', 'COMPLAINT', 'BOUNCE') NOT NULL,
    event_data JSON,                                             -- URL clicked, user agent, etc.
    ip_address VARCHAR(45) NULL,                                 -- IPv4 or IPv6
    user_agent TEXT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    
    -- Performance indexes
    INDEX idx_message_recipient (message_recipient_id),
    INDEX idx_event_type_created (event_type, created_at),
    INDEX idx_created_at (created_at),
    
    -- Foreign key
    FOREIGN KEY (message_recipient_id) REFERENCES message_recipients(id) ON DELETE CASCADE
);

-- Recipient lists for campaign management
CREATE TABLE IF NOT EXISTS recipient_lists (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    name VARCHAR(255) NOT NULL,
    description TEXT NULL,
    workspace_id VARCHAR(255) NOT NULL,
    user_id VARCHAR(255) NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    
    INDEX idx_workspace_active (workspace_id, is_active),
    INDEX idx_user_id (user_id)
);

-- Junction table for recipient list membership
CREATE TABLE IF NOT EXISTS recipient_list_members (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    list_id BIGINT NOT NULL,
    recipient_id BIGINT NOT NULL,
    added_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    added_by VARCHAR(255) NULL,
    
    UNIQUE KEY uk_list_recipient (list_id, recipient_id),
    INDEX idx_list_id (list_id),
    INDEX idx_recipient_id (recipient_id),
    
    FOREIGN KEY (list_id) REFERENCES recipient_lists(id) ON DELETE CASCADE,
    FOREIGN KEY (recipient_id) REFERENCES recipients(id) ON DELETE CASCADE
);

-- Now migrate existing data

-- Step 1: Extract unique recipients from existing messages
-- This will create recipient records for all emails that have been sent to
INSERT IGNORE INTO recipients (email_address, workspace_id, user_id, campaign_id, status, created_at, updated_at)
SELECT DISTINCT 
    JSON_UNQUOTE(JSON_EXTRACT(recipient_email.value, '$')) as email_address,
    m.workspace_id,
    m.user_id,
    m.campaign_id,
    CASE 
        WHEN m.status = 'sent' THEN 'ACTIVE'
        WHEN m.status = 'failed' AND m.error LIKE '%bounce%' THEN 'BOUNCED'
        WHEN m.status = 'failed' AND m.error LIKE '%invalid%' THEN 'BOUNCED'
        ELSE 'ACTIVE'
    END as status,
    m.queued_at as created_at,
    CURRENT_TIMESTAMP as updated_at
FROM messages m
CROSS JOIN JSON_TABLE(
    CONCAT('[', 
        COALESCE(m.to_emails, '[]'), 
        ',', COALESCE(m.cc_emails, '[]'), 
        ',', COALESCE(m.bcc_emails, '[]'),
    ']'), 
    '$[*]' COLUMNS (value JSON PATH '$')
) as recipient_email
WHERE JSON_UNQUOTE(JSON_EXTRACT(recipient_email.value, '$')) IS NOT NULL
  AND JSON_UNQUOTE(JSON_EXTRACT(recipient_email.value, '$')) != ''
  AND JSON_UNQUOTE(JSON_EXTRACT(recipient_email.value, '$')) REGEXP '^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\\.[A-Za-z]{2,}$';

-- Step 2: Create message recipient records for TO recipients
INSERT IGNORE INTO message_recipients (message_id, recipient_id, recipient_type, delivery_status, sent_at, created_at, updated_at)
SELECT 
    m.id as message_id,
    r.id as recipient_id,
    'TO' as recipient_type,
    CASE 
        WHEN m.status = 'sent' THEN 'SENT'
        WHEN m.status = 'failed' AND m.error LIKE '%bounce%' THEN 'BOUNCED'
        WHEN m.status = 'failed' THEN 'FAILED'
        WHEN m.status = 'auth_error' THEN 'DEFERRED'
        ELSE 'PENDING'
    END as delivery_status,
    CASE WHEN m.status = 'sent' THEN m.processed_at ELSE NULL END as sent_at,
    m.queued_at as created_at,
    CURRENT_TIMESTAMP as updated_at
FROM messages m
CROSS JOIN JSON_TABLE(
    COALESCE(m.to_emails, '[]'), 
    '$[*]' COLUMNS (value JSON PATH '$')
) as to_email
JOIN recipients r ON r.email_address = JSON_UNQUOTE(JSON_EXTRACT(to_email.value, '$')) 
                 AND r.workspace_id = m.workspace_id
WHERE JSON_UNQUOTE(JSON_EXTRACT(to_email.value, '$')) IS NOT NULL
  AND JSON_UNQUOTE(JSON_EXTRACT(to_email.value, '$')) != '';

-- Step 3: Create message recipient records for CC recipients
INSERT IGNORE INTO message_recipients (message_id, recipient_id, recipient_type, delivery_status, sent_at, created_at, updated_at)
SELECT 
    m.id as message_id,
    r.id as recipient_id,
    'CC' as recipient_type,
    CASE 
        WHEN m.status = 'sent' THEN 'SENT'
        WHEN m.status = 'failed' AND m.error LIKE '%bounce%' THEN 'BOUNCED'
        WHEN m.status = 'failed' THEN 'FAILED'
        WHEN m.status = 'auth_error' THEN 'DEFERRED'
        ELSE 'PENDING'
    END as delivery_status,
    CASE WHEN m.status = 'sent' THEN m.processed_at ELSE NULL END as sent_at,
    m.queued_at as created_at,
    CURRENT_TIMESTAMP as updated_at
FROM messages m
CROSS JOIN JSON_TABLE(
    COALESCE(m.cc_emails, '[]'), 
    '$[*]' COLUMNS (value JSON PATH '$')
) as cc_email
JOIN recipients r ON r.email_address = JSON_UNQUOTE(JSON_EXTRACT(cc_email.value, '$')) 
                 AND r.workspace_id = m.workspace_id
WHERE JSON_UNQUOTE(JSON_EXTRACT(cc_email.value, '$')) IS NOT NULL
  AND JSON_UNQUOTE(JSON_EXTRACT(cc_email.value, '$')) != '';

-- Step 4: Create message recipient records for BCC recipients
INSERT IGNORE INTO message_recipients (message_id, recipient_id, recipient_type, delivery_status, sent_at, created_at, updated_at)
SELECT 
    m.id as message_id,
    r.id as recipient_id,
    'BCC' as recipient_type,
    CASE 
        WHEN m.status = 'sent' THEN 'SENT'
        WHEN m.status = 'failed' AND m.error LIKE '%bounce%' THEN 'BOUNCED'
        WHEN m.status = 'failed' THEN 'FAILED'
        WHEN m.status = 'auth_error' THEN 'DEFERRED'
        ELSE 'PENDING'
    END as delivery_status,
    CASE WHEN m.status = 'sent' THEN m.processed_at ELSE NULL END as sent_at,
    m.queued_at as created_at,
    CURRENT_TIMESTAMP as updated_at
FROM messages m
CROSS JOIN JSON_TABLE(
    COALESCE(m.bcc_emails, '[]'), 
    '$[*]' COLUMNS (value JSON PATH '$')
) as bcc_email
JOIN recipients r ON r.email_address = JSON_UNQUOTE(JSON_EXTRACT(bcc_email.value, '$')) 
                 AND r.workspace_id = m.workspace_id
WHERE JSON_UNQUOTE(JSON_EXTRACT(bcc_email.value, '$')) IS NOT NULL
  AND JSON_UNQUOTE(JSON_EXTRACT(bcc_email.value, '$')) != '';

-- Step 5: Update bounce counts for recipients based on message history
UPDATE recipients r SET 
    bounce_count = (
        SELECT COUNT(*)
        FROM message_recipients mr
        WHERE mr.recipient_id = r.id 
          AND mr.delivery_status = 'BOUNCED'
    ),
    last_bounce_date = (
        SELECT MAX(mr.updated_at)
        FROM message_recipients mr
        WHERE mr.recipient_id = r.id 
          AND mr.delivery_status = 'BOUNCED'
    ),
    bounce_type = (
        SELECT CASE 
            WHEN COUNT(*) >= 3 THEN 'HARD'
            ELSE 'SOFT'
        END
        FROM message_recipients mr
        WHERE mr.recipient_id = r.id 
          AND mr.delivery_status = 'BOUNCED'
    ),
    status = CASE 
        WHEN (
            SELECT COUNT(*)
            FROM message_recipients mr
            WHERE mr.recipient_id = r.id 
              AND mr.delivery_status = 'BOUNCED'
        ) >= 3 THEN 'BOUNCED'
        ELSE r.status
    END
WHERE EXISTS (
    SELECT 1 FROM message_recipients mr 
    WHERE mr.recipient_id = r.id 
      AND mr.delivery_status = 'BOUNCED'
);

-- Step 6: Create indices for performance (if not already created)
-- These are duplicated from the CREATE TABLE statements for safety

CREATE INDEX IF NOT EXISTS idx_recipients_workspace_status ON recipients (workspace_id, status);
CREATE INDEX IF NOT EXISTS idx_recipients_campaign_id ON recipients (campaign_id);
CREATE INDEX IF NOT EXISTS idx_recipients_user_id ON recipients (user_id);
CREATE INDEX IF NOT EXISTS idx_recipients_status_created ON recipients (status, created_at);
CREATE INDEX IF NOT EXISTS idx_recipients_email_status ON recipients (email_address, status);
CREATE INDEX IF NOT EXISTS idx_recipients_bounce_tracking ON recipients (status, bounce_count, last_bounce_date);

CREATE INDEX IF NOT EXISTS idx_message_recipients_message_id ON message_recipients (message_id);
CREATE INDEX IF NOT EXISTS idx_message_recipients_recipient_id ON message_recipients (recipient_id);
CREATE INDEX IF NOT EXISTS idx_message_recipients_delivery_status ON message_recipients (delivery_status);
CREATE INDEX IF NOT EXISTS idx_message_recipients_sent_at ON message_recipients (sent_at);
CREATE INDEX IF NOT EXISTS idx_message_recipients_engagement ON message_recipients (opens, clicks, last_open_at);
CREATE INDEX IF NOT EXISTS idx_message_recipients_message_status_type ON message_recipients (message_id, delivery_status, recipient_type);

CREATE INDEX IF NOT EXISTS idx_recipient_events_message_recipient ON recipient_events (message_recipient_id);
CREATE INDEX IF NOT EXISTS idx_recipient_events_event_type_created ON recipient_events (event_type, created_at);
CREATE INDEX IF NOT EXISTS idx_recipient_events_created_at ON recipient_events (created_at);

-- Step 7: Show migration summary
SELECT 
    'Migration Summary' as summary,
    (SELECT COUNT(*) FROM recipients) as total_recipients,
    (SELECT COUNT(*) FROM message_recipients) as total_message_recipients,
    (SELECT COUNT(*) FROM recipients WHERE status = 'ACTIVE') as active_recipients,
    (SELECT COUNT(*) FROM recipients WHERE status = 'BOUNCED') as bounced_recipients,
    (SELECT COUNT(*) FROM message_recipients WHERE delivery_status = 'SENT') as sent_deliveries,
    (SELECT COUNT(*) FROM message_recipients WHERE delivery_status = 'BOUNCED') as bounced_deliveries;