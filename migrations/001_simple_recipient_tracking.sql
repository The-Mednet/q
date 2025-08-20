-- Simple migration script - only creates tables if they don't exist
-- Indexes are created as part of the CREATE TABLE statements

USE relay;

-- Recipients tracking table
CREATE TABLE IF NOT EXISTS recipients (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    email_address VARCHAR(320) NOT NULL,
    workspace_id VARCHAR(255) NOT NULL,
    user_id VARCHAR(255) NULL,
    campaign_id VARCHAR(255) NULL,
    first_name VARCHAR(100) NULL,
    last_name VARCHAR(100) NULL,
    status ENUM('ACTIVE', 'INACTIVE', 'BOUNCED', 'UNSUBSCRIBED') NOT NULL DEFAULT 'ACTIVE',
    opt_in_date TIMESTAMP NULL,
    opt_out_date TIMESTAMP NULL,
    bounce_count INT NOT NULL DEFAULT 0,
    last_bounce_date TIMESTAMP NULL,
    bounce_type ENUM('SOFT', 'HARD') NULL,
    metadata JSON,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    
    UNIQUE KEY uk_email_workspace (email_address, workspace_id),
    INDEX idx_workspace_status (workspace_id, status),
    INDEX idx_campaign_id (campaign_id),
    INDEX idx_user_id (user_id),
    INDEX idx_status_created (status, created_at),
    INDEX idx_email_status (email_address, status),
    INDEX idx_bounce_tracking (status, bounce_count, last_bounce_date)
);

-- Message recipients junction table
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
    
    UNIQUE KEY uk_message_recipient (message_id, recipient_id),
    INDEX idx_message_id (message_id),
    INDEX idx_recipient_id (recipient_id),
    INDEX idx_delivery_status (delivery_status),
    INDEX idx_sent_at (sent_at),
    INDEX idx_engagement (opens, clicks, last_open_at),
    INDEX idx_message_status_type (message_id, delivery_status, recipient_type),
    
    FOREIGN KEY (message_id) REFERENCES messages(id) ON DELETE CASCADE,
    FOREIGN KEY (recipient_id) REFERENCES recipients(id) ON DELETE CASCADE
);

-- Recipient engagement events table
CREATE TABLE IF NOT EXISTS recipient_events (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    message_recipient_id BIGINT NOT NULL,
    event_type ENUM('OPEN', 'CLICK', 'UNSUBSCRIBE', 'COMPLAINT', 'BOUNCE') NOT NULL,
    event_data JSON,
    ip_address VARCHAR(45) NULL,
    user_agent TEXT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    
    INDEX idx_message_recipient (message_recipient_id),
    INDEX idx_event_type_created (event_type, created_at),
    INDEX idx_created_at (created_at),
    
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

SELECT 'Migration completed successfully!' as result;