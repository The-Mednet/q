-- Fixed migration script for adding recipient tracking tables
-- Compatible with MySQL 5.7+ and 8.0+

USE relay;

-- Check if recipient tracking tables already exist
SET @table_exists = (SELECT COUNT(*)
    FROM information_schema.tables 
    WHERE table_schema = 'relay' 
    AND table_name = 'recipients');

-- Only create tables if they don't exist
SET @sql = IF(@table_exists > 0, 
    'SELECT "Recipients table already exists, skipping creation" as status',
    '
-- Recipients tracking table
CREATE TABLE recipients (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    email_address VARCHAR(320) NOT NULL,                        -- RFC 5321 max email length
    workspace_id VARCHAR(255) NOT NULL,
    user_id VARCHAR(255) NULL,                                   -- Can be null for guest recipients
    campaign_id VARCHAR(255) NULL,                               -- Can be null for non-campaign emails
    first_name VARCHAR(100) NULL,
    last_name VARCHAR(100) NULL,
    status ENUM(''ACTIVE'', ''INACTIVE'', ''BOUNCED'', ''UNSUBSCRIBED'') NOT NULL DEFAULT ''ACTIVE'',
    opt_in_date TIMESTAMP NULL,
    opt_out_date TIMESTAMP NULL,
    bounce_count INT NOT NULL DEFAULT 0,
    last_bounce_date TIMESTAMP NULL,
    bounce_type ENUM(''SOFT'', ''HARD'') NULL,
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
CREATE TABLE message_recipients (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    message_id VARCHAR(36) NOT NULL,
    recipient_id BIGINT NOT NULL,
    recipient_type ENUM(''TO'', ''CC'', ''BCC'') NOT NULL,
    delivery_status ENUM(''PENDING'', ''SENT'', ''BOUNCED'', ''FAILED'', ''DEFERRED'') NOT NULL DEFAULT ''PENDING'',
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
CREATE TABLE recipient_events (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    message_recipient_id BIGINT NOT NULL,
    event_type ENUM(''OPEN'', ''CLICK'', ''UNSUBSCRIBE'', ''COMPLAINT'', ''BOUNCE'') NOT NULL,
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
CREATE TABLE recipient_lists (
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
CREATE TABLE recipient_list_members (
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

SELECT "Recipient tracking tables created successfully!" as status;
');

PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;