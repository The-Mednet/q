CREATE DATABASE IF NOT EXISTS smtp_relay;

USE smtp_relay;

CREATE TABLE IF NOT EXISTS messages (
    id VARCHAR(36) PRIMARY KEY,
    from_email VARCHAR(255) NOT NULL,
    to_emails TEXT NOT NULL,
    cc_emails TEXT,
    bcc_emails TEXT,
    subject TEXT,
    html_body LONGTEXT,
    text_body LONGTEXT,
    headers JSON,
    attachments JSON,
    metadata JSON,
    campaign_id VARCHAR(255),
    user_id VARCHAR(255),
    workspace_id VARCHAR(255),
    status ENUM('queued', 'processing', 'sent', 'failed', 'auth_error') NOT NULL DEFAULT 'queued',
    queued_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    processed_at TIMESTAMP NULL,
    error TEXT,
    retry_count INT DEFAULT 0,
    INDEX idx_status (status),
    INDEX idx_queued_at (queued_at),
    INDEX idx_status_queued (status, queued_at),
    INDEX idx_campaign_id (campaign_id),
    INDEX idx_user_id (user_id),
    INDEX idx_workspace_id (workspace_id)
);

CREATE TABLE IF NOT EXISTS webhook_events (
    id VARCHAR(36) PRIMARY KEY,
    message_id VARCHAR(36) NOT NULL,
    event_type VARCHAR(50) NOT NULL,
    event_data JSON,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    sent_at TIMESTAMP NULL,
    status ENUM('pending', 'sent', 'failed') NOT NULL DEFAULT 'pending',
    error TEXT,
    retry_count INT DEFAULT 0,
    INDEX idx_message_id (message_id),
    INDEX idx_status (status),
    FOREIGN KEY (message_id) REFERENCES messages(id) ON DELETE CASCADE
);