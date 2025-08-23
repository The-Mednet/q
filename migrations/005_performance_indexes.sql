-- Performance indexes for SMTP relay service
-- Critical for medical-grade performance requirements

-- Messages table indexes
-- Note: Will fail silently if index already exists
CREATE INDEX idx_messages_queue_status 
ON messages(status, queued_at, retry_count);

CREATE INDEX idx_messages_recipients 
ON messages(from_email, to_emails(255));

CREATE INDEX idx_messages_date_range 
ON messages(queued_at, sent_at);

CREATE INDEX idx_messages_provider 
ON messages(provider_id);

-- Workspaces table indexes
CREATE INDEX idx_workspaces_domain 
ON workspaces(domain);

CREATE INDEX idx_workspaces_provider 
ON workspaces(provider_type, enabled);

-- Load balancing tables indexes (only create if tables exist)
-- These will be created when load balancing tables are set up

-- Add table partitioning for messages table (if supported)
-- This helps with archival and performance for large datasets
-- Note: Requires MySQL 5.7+ or MariaDB 10.2+

-- Future: Consider partitioning messages table by month
-- ALTER TABLE messages 
-- PARTITION BY RANGE (YEAR(queued_at)*100 + MONTH(queued_at)) (
--     PARTITION p202501 VALUES LESS THAN (202502),
--     PARTITION p202502 VALUES LESS THAN (202503),
--     PARTITION p202503 VALUES LESS THAN (202504),
--     PARTITION pmax VALUES LESS THAN MAXVALUE
-- );