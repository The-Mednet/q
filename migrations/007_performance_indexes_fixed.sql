-- Performance optimization indexes for SMTP relay service
-- Fixed version that matches actual database schema

-- Messages table composite indexes for queue operations
CREATE INDEX idx_messages_queue_processing 
ON messages(workspace_id, status, queued_at);

CREATE INDEX idx_messages_status_queued 
ON messages(status, queued_at);

-- Message recipients indexes (if table exists)
-- CREATE INDEX idx_recipients_email ON message_recipients(email_address);

-- Campaign and user tracking optimization
CREATE INDEX idx_messages_campaign_queued 
ON messages(campaign_id, queued_at);

CREATE INDEX idx_messages_user_queued 
ON messages(user_id, queued_at);

-- Provider lookup optimization
CREATE INDEX idx_messages_provider 
ON messages(provider_id, status);

-- Pool tracking
CREATE INDEX idx_messages_pool 
ON messages(pool_id, status);

-- Workspace providers composite index (if not exists)
-- Check first if table exists
DROP PROCEDURE IF EXISTS AddIndexIfTableExists;

DELIMITER $$
CREATE PROCEDURE AddIndexIfTableExists()
BEGIN
    IF EXISTS (SELECT * FROM information_schema.tables 
               WHERE table_schema = DATABASE() 
               AND table_name = 'workspace_providers') THEN
        
        -- Check if index already exists
        IF NOT EXISTS (SELECT * FROM information_schema.statistics 
                      WHERE table_schema = DATABASE() 
                      AND table_name = 'workspace_providers' 
                      AND index_name = 'idx_workspace_providers_lookup') THEN
            SET @sql = 'CREATE INDEX idx_workspace_providers_lookup ON workspace_providers(workspace_id, provider_type, enabled)';
            PREPARE stmt FROM @sql;
            EXECUTE stmt;
            DEALLOCATE PREPARE stmt;
        END IF;
    END IF;
    
    -- Add index for rate limits if table exists
    IF EXISTS (SELECT * FROM information_schema.tables 
               WHERE table_schema = DATABASE() 
               AND table_name = 'workspace_user_rate_limits') THEN
        
        IF NOT EXISTS (SELECT * FROM information_schema.statistics 
                      WHERE table_schema = DATABASE() 
                      AND table_name = 'workspace_user_rate_limits' 
                      AND index_name = 'idx_user_rate_limits_lookup') THEN
            SET @sql = 'CREATE INDEX idx_user_rate_limits_lookup ON workspace_user_rate_limits(workspace_id, email_address)';
            PREPARE stmt FROM @sql;
            EXECUTE stmt;
            DEALLOCATE PREPARE stmt;
        END IF;
    END IF;
    
    -- Add index for provider selections if table exists
    IF EXISTS (SELECT * FROM information_schema.tables 
               WHERE table_schema = DATABASE() 
               AND table_name = 'provider_selections') THEN
        
        IF NOT EXISTS (SELECT * FROM information_schema.statistics 
                      WHERE table_schema = DATABASE() 
                      AND table_name = 'provider_selections' 
                      AND index_name = 'idx_provider_selections_time') THEN
            SET @sql = 'CREATE INDEX idx_provider_selections_time ON provider_selections(selected_at, pool_id)';
            PREPARE stmt FROM @sql;
            EXECUTE stmt;
            DEALLOCATE PREPARE stmt;
        END IF;
    END IF;
END$$
DELIMITER ;

CALL AddIndexIfTableExists();
DROP PROCEDURE AddIndexIfTableExists;

-- Analyze tables to update statistics
ANALYZE TABLE messages;

-- Show created indexes
SELECT 
    TABLE_NAME,
    INDEX_NAME,
    GROUP_CONCAT(COLUMN_NAME ORDER BY SEQ_IN_INDEX) as COLUMNS
FROM information_schema.STATISTICS
WHERE TABLE_SCHEMA = DATABASE()
AND TABLE_NAME = 'messages'
AND INDEX_NAME LIKE 'idx_%'
GROUP BY TABLE_NAME, INDEX_NAME;