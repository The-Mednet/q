-- Performance optimization indexes for SMTP relay service
-- These indexes optimize common query patterns identified during code audit

-- Messages table composite indexes for queue operations
CREATE INDEX idx_messages_queue_processing 
ON messages(workspace_id, status, queued_at)
COMMENT 'Optimize queue processing queries';

CREATE INDEX idx_messages_status_queued 
ON messages(status, queued_at)
COMMENT 'Optimize status-based message queries';

CREATE INDEX idx_messages_workspace_status_priority 
ON messages(workspace_id, status, priority DESC, queued_at)
COMMENT 'Optimize priority queue processing';

-- Message recipients indexes for delivery tracking
CREATE INDEX idx_recipients_delivery_status 
ON message_recipients(delivery_status, sent_at)
COMMENT 'Optimize delivery status queries';

CREATE INDEX idx_recipients_message_status 
ON message_recipients(message_id, delivery_status)
COMMENT 'Optimize per-message recipient queries';

CREATE INDEX idx_recipients_email_status 
ON message_recipients(email_address, delivery_status, sent_at)
COMMENT 'Optimize per-recipient history queries';

-- Workspace providers composite index
CREATE INDEX idx_workspace_providers_lookup 
ON workspace_providers(workspace_id, provider_type, enabled)
COMMENT 'Optimize provider lookup queries';

-- Rate limits lookup optimization
CREATE INDEX idx_user_rate_limits_lookup 
ON workspace_user_rate_limits(workspace_id, email_address)
COMMENT 'Optimize rate limit checks';

-- Provider statistics time-based queries
CREATE INDEX idx_provider_stats_time_range 
ON provider_statistics(provider_id, stat_date, stat_hour)
COMMENT 'Optimize time-range statistics queries';

-- Load balancing selections for analytics
CREATE INDEX idx_provider_selections_time 
ON provider_selections(selected_at, pool_id)
COMMENT 'Optimize selection analytics queries';

CREATE INDEX idx_provider_selections_workspace 
ON provider_selections(workspace_id, selected_at)
COMMENT 'Optimize per-workspace selection queries';

-- Header rewrite rules lookup
CREATE INDEX idx_header_rules_active 
ON provider_header_rewrite_rules(provider_id, enabled, priority)
COMMENT 'Optimize active header rule queries';

-- Campaign tracking optimization
CREATE INDEX idx_messages_campaign 
ON messages(campaign_id, created_at)
WHERE campaign_id IS NOT NULL
COMMENT 'Optimize campaign-based queries';

-- User tracking optimization  
CREATE INDEX idx_messages_user 
ON messages(user_id, created_at)
WHERE user_id IS NOT NULL
COMMENT 'Optimize user-based queries';

-- Analyze tables to update statistics after adding indexes
ANALYZE TABLE messages;
ANALYZE TABLE message_recipients;
ANALYZE TABLE workspace_providers;
ANALYZE TABLE workspace_user_rate_limits;
ANALYZE TABLE provider_statistics;
ANALYZE TABLE provider_selections;
ANALYZE TABLE provider_header_rewrite_rules;

-- Verification query to confirm indexes were created
SELECT 
    TABLE_NAME,
    INDEX_NAME,
    COLUMN_NAME,
    SEQ_IN_INDEX,
    INDEX_COMMENT
FROM information_schema.STATISTICS
WHERE TABLE_SCHEMA = DATABASE()
AND TABLE_NAME IN (
    'messages', 
    'message_recipients', 
    'workspace_providers',
    'workspace_user_rate_limits',
    'provider_statistics',
    'provider_selections',
    'provider_header_rewrite_rules'
)
AND INDEX_NAME LIKE 'idx_%'
ORDER BY TABLE_NAME, INDEX_NAME, SEQ_IN_INDEX;