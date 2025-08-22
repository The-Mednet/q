-- Migration: Add missing performance indexes
-- Date: 2024-08-21
-- Purpose: Critical performance optimizations for high-volume email processing

-- 1. Composite index for workspace-based rate limiting queries
ALTER TABLE messages ADD INDEX idx_workspace_status_queued (workspace_id, status, queued_at);

-- 2. Index for user and campaign filtering
ALTER TABLE messages ADD INDEX idx_user_campaign_status (user_id, campaign_id, status);

-- 3. Index for rate limiting queries (GetSentCountsByWorkspaceAndSender)
ALTER TABLE messages ADD INDEX idx_rate_limiting (status, processed_at, workspace_id, from_email);

-- 4. Index for queue processing with better selectivity
ALTER TABLE messages ADD INDEX idx_queue_processing (status, retry_count, queued_at);

-- 5. Index for recipient delivery tracking
ALTER TABLE message_recipients ADD INDEX idx_recipient_status_sent (recipient_id, delivery_status, sent_at);

-- 6. Index for recipient event queries
ALTER TABLE recipient_events ADD INDEX idx_recipient_event_lookup (recipient_id, event_type, created_at);

-- 7. Index for webhook event processing
ALTER TABLE webhook_events ADD INDEX idx_webhook_status (status, created_at);

-- 8. Covering index for recipient summary queries  
ALTER TABLE message_recipients ADD INDEX idx_recipient_summary 
    (recipient_id, delivery_status, opens, clicks, last_open_at, last_click_at);

-- 9. Index for email lookup performance
ALTER TABLE recipients ADD INDEX idx_email_lookup (email_address);

-- 10. Index for message cleanup and archival
ALTER TABLE messages ADD INDEX idx_cleanup (created_at, status);

-- 11. Index for workspace-based queries
ALTER TABLE messages ADD INDEX idx_workspace_lookup (workspace_id, created_at);

-- 12. Index for failed message retry queries
ALTER TABLE messages ADD INDEX idx_retry_failed (status, retry_count, last_retry_at);

-- Note: Monitor index usage with:
-- SELECT * FROM sys.schema_unused_indexes;
-- SELECT * FROM sys.statements_with_full_table_scans;