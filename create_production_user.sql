-- Create production database and user for SMTP relay service
-- Run as MySQL root or admin user

-- Create the database if it doesn't exist
CREATE DATABASE IF NOT EXISTS relay CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

-- Create the relay user (drop if exists for idempotency)
DROP USER IF EXISTS 'relay'@'%';
CREATE USER 'relay'@'%' IDENTIFIED BY 'CHANGE_THIS_PASSWORD';

-- Grant necessary permissions on the relay database
GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, INDEX, ALTER ON relay.* TO 'relay'@'%';

-- Apply privileges
FLUSH PRIVILEGES;

-- Verify the grants
SHOW GRANTS FOR 'relay'@'%';

-- Switch to the relay database
USE relay;

-- Create tables if they don't exist (optional - the app will create them)
-- This ensures the database is ready for immediate use

-- Email queue table
CREATE TABLE IF NOT EXISTS email_queue (
    id VARCHAR(36) PRIMARY KEY,
    workspace_id VARCHAR(255) NOT NULL,
    from_email VARCHAR(255) NOT NULL,
    to_email TEXT NOT NULL,
    cc_email TEXT,
    bcc_email TEXT,
    subject TEXT,
    body_text LONGTEXT,
    body_html LONGTEXT,
    headers JSON,
    status ENUM('pending', 'processing', 'sent', 'failed') DEFAULT 'pending',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    sent_at TIMESTAMP NULL,
    attempts INT DEFAULT 0,
    last_error TEXT,
    provider VARCHAR(50),
    provider_message_id VARCHAR(255),
    
    INDEX idx_status (status),
    INDEX idx_workspace_id (workspace_id),
    INDEX idx_created_at (created_at),
    INDEX idx_from_email (from_email),
    INDEX idx_status_attempts (status, attempts),
    INDEX idx_status_created (status, created_at),
    INDEX idx_workspace_status_created (workspace_id, status, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Email attachments table
CREATE TABLE IF NOT EXISTS email_queue_attachments (
    id VARCHAR(36) PRIMARY KEY,
    email_id VARCHAR(36) NOT NULL,
    filename VARCHAR(255) NOT NULL,
    content_type VARCHAR(100),
    size INT,
    content LONGBLOB,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    INDEX idx_email_id (email_id),
    FOREIGN KEY (email_id) REFERENCES email_queue(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Recipient tracking table
CREATE TABLE IF NOT EXISTS recipients (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    email VARCHAR(255) NOT NULL,
    domain VARCHAR(255) NOT NULL,
    first_seen_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_seen_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    total_emails_sent INT DEFAULT 1,
    status ENUM('active', 'bounced', 'complained', 'unsubscribed') DEFAULT 'active',
    metadata JSON,
    
    UNIQUE KEY idx_email (email),
    INDEX idx_domain (domain),
    INDEX idx_status (status),
    INDEX idx_last_seen (last_seen_at),
    INDEX idx_domain_status (domain, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Rate limiting table
CREATE TABLE IF NOT EXISTS rate_limits (
    id VARCHAR(255) PRIMARY KEY,
    workspace_id VARCHAR(255) NOT NULL,
    user_email VARCHAR(255),
    date DATE NOT NULL,
    count INT DEFAULT 0,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    
    INDEX idx_workspace_date (workspace_id, date),
    INDEX idx_user_date (user_email, date),
    UNIQUE KEY idx_workspace_user_date (workspace_id, user_email, date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Print summary
SELECT 'Database setup complete!' as Status;
SELECT 
    'Database: relay' as Item,
    'User: relay@%' as Value,
    'Tables: email_queue, email_queue_attachments, recipients, rate_limits' as Details;