-- Load Balancing Database Schema
-- This script creates the necessary tables for the load balancing feature
-- Run this script after the main schema.sql to add load balancing support

USE relay;

-- Load balancing pools table
-- Stores configuration for each load balancing pool
CREATE TABLE IF NOT EXISTS load_balancing_pools (
    id VARCHAR(255) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    domain_patterns JSON NOT NULL COMMENT 'JSON array of domain patterns this pool handles',
    strategy ENUM('capacity_weighted', 'round_robin', 'least_used', 'random_weighted') DEFAULT 'capacity_weighted' COMMENT 'Selection algorithm for this pool',
    enabled BOOLEAN DEFAULT TRUE COMMENT 'Whether this pool is active',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    
    -- Indexes for performance
    INDEX idx_enabled (enabled),
    INDEX idx_created_at (created_at),
    INDEX idx_updated_at (updated_at)
) ENGINE=InnoDB COMMENT='Load balancing pool configurations';

-- Pool workspaces junction table
-- Maps workspaces to pools with their configuration
CREATE TABLE IF NOT EXISTS pool_workspaces (
    pool_id VARCHAR(255) NOT NULL,
    workspace_id VARCHAR(255) NOT NULL,
    weight DECIMAL(5,2) DEFAULT 1.0 COMMENT 'Weight for load balancing selection (higher = more traffic)',
    enabled BOOLEAN DEFAULT TRUE COMMENT 'Whether this workspace is active in the pool',
    min_capacity_threshold DECIMAL(4,3) DEFAULT 0.01 COMMENT 'Minimum capacity percentage to be eligible (0.01 = 1%)',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    
    -- Composite primary key
    PRIMARY KEY (pool_id, workspace_id),
    
    -- Indexes for performance
    INDEX idx_pool_enabled (pool_id, enabled),
    INDEX idx_workspace_enabled (workspace_id, enabled),
    INDEX idx_weight (weight),
    
    -- Foreign key constraint to pools table
    FOREIGN KEY (pool_id) REFERENCES load_balancing_pools(id) ON DELETE CASCADE
) ENGINE=InnoDB COMMENT='Workspace membership in load balancing pools';

-- Load balancing selection history table
-- Records each selection decision for analytics and debugging
CREATE TABLE IF NOT EXISTS load_balancing_selections (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    pool_id VARCHAR(255) NOT NULL,
    workspace_id VARCHAR(255) NOT NULL,
    sender_email VARCHAR(320) NOT NULL COMMENT 'Email address that triggered the selection',
    selected_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    success BOOLEAN NOT NULL COMMENT 'Whether the selection resulted in successful email delivery',
    capacity_score DECIMAL(5,4) NOT NULL COMMENT 'Capacity score at time of selection (0.0000 to 1.0000)',
    selection_reason VARCHAR(500) NULL COMMENT 'Human-readable reason for the selection',
    response_time_ms INT NULL COMMENT 'Time taken for selection decision in milliseconds',
    
    -- Indexes for analytics and performance
    INDEX idx_pool_workspace (pool_id, workspace_id),
    INDEX idx_selected_at (selected_at),
    INDEX idx_sender_selected (sender_email, selected_at),
    INDEX idx_success_selected (success, selected_at),
    INDEX idx_capacity_score (capacity_score),
    
    -- Composite indexes for common queries
    INDEX idx_pool_success_time (pool_id, success, selected_at),
    INDEX idx_workspace_success_time (workspace_id, success, selected_at),
    
    -- Foreign key to maintain referential integrity
    FOREIGN KEY (pool_id) REFERENCES load_balancing_pools(id) ON DELETE CASCADE
) ENGINE=InnoDB COMMENT='History of load balancing selection decisions';

-- Pool health status table
-- Tracks the health status of workspaces within pools
CREATE TABLE IF NOT EXISTS pool_workspace_health (
    pool_id VARCHAR(255) NOT NULL,
    workspace_id VARCHAR(255) NOT NULL,
    is_healthy BOOLEAN DEFAULT TRUE,
    last_health_check TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_error TEXT NULL COMMENT 'Last error message if health check failed',
    consecutive_failures INT DEFAULT 0 COMMENT 'Number of consecutive health check failures',
    response_time_ms INT NULL COMMENT 'Last health check response time in milliseconds',
    
    -- Composite primary key
    PRIMARY KEY (pool_id, workspace_id),
    
    -- Indexes
    INDEX idx_pool_health (pool_id, is_healthy),
    INDEX idx_workspace_health (workspace_id, is_healthy),
    INDEX idx_last_check (last_health_check),
    INDEX idx_consecutive_failures (consecutive_failures),
    
    -- Foreign keys
    FOREIGN KEY (pool_id) REFERENCES load_balancing_pools(id) ON DELETE CASCADE,
    
    -- Update timestamp on changes
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB COMMENT='Health status tracking for workspaces in pools';

-- Pool metrics aggregation table
-- Stores aggregated metrics for pools over time
CREATE TABLE IF NOT EXISTS pool_metrics (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    pool_id VARCHAR(255) NOT NULL,
    metric_date DATE NOT NULL,
    hour_of_day TINYINT NOT NULL COMMENT 'Hour (0-23) for hourly aggregation',
    total_selections INT DEFAULT 0,
    successful_selections INT DEFAULT 0,
    failed_selections INT DEFAULT 0,
    avg_capacity_score DECIMAL(5,4) NULL,
    avg_response_time_ms INT NULL,
    unique_senders INT DEFAULT 0,
    unique_workspaces_used INT DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    -- Unique constraint to prevent duplicates
    UNIQUE KEY uk_pool_date_hour (pool_id, metric_date, hour_of_day),
    
    -- Indexes for queries
    INDEX idx_pool_date (pool_id, metric_date),
    INDEX idx_metric_date (metric_date),
    INDEX idx_total_selections (total_selections),
    
    -- Foreign key
    FOREIGN KEY (pool_id) REFERENCES load_balancing_pools(id) ON DELETE CASCADE
) ENGINE=InnoDB COMMENT='Aggregated metrics for load balancing pools';

-- Views for common queries
-- Pool status view - shows current status of all pools
CREATE OR REPLACE VIEW pool_status_view AS
SELECT 
    p.id as pool_id,
    p.name as pool_name,
    p.strategy,
    p.enabled as pool_enabled,
    COUNT(pw.workspace_id) as total_workspaces,
    COUNT(CASE WHEN pw.enabled THEN 1 END) as enabled_workspaces,
    COUNT(CASE WHEN ph.is_healthy THEN 1 END) as healthy_workspaces,
    AVG(pw.weight) as avg_weight,
    p.created_at,
    p.updated_at
FROM load_balancing_pools p
LEFT JOIN pool_workspaces pw ON p.id = pw.pool_id
LEFT JOIN pool_workspace_health ph ON p.id = ph.pool_id AND pw.workspace_id = ph.workspace_id
GROUP BY p.id, p.name, p.strategy, p.enabled, p.created_at, p.updated_at;

-- Pool performance view - shows recent performance metrics
CREATE OR REPLACE VIEW pool_performance_view AS
SELECT 
    ls.pool_id,
    p.name as pool_name,
    COUNT(*) as total_selections_24h,
    COUNT(CASE WHEN ls.success THEN 1 END) as successful_selections_24h,
    ROUND(COUNT(CASE WHEN ls.success THEN 1 END) * 100.0 / COUNT(*), 2) as success_rate_pct,
    AVG(ls.capacity_score) as avg_capacity_score,
    AVG(ls.response_time_ms) as avg_response_time_ms,
    COUNT(DISTINCT ls.workspace_id) as workspaces_used_24h,
    COUNT(DISTINCT ls.sender_email) as unique_senders_24h,
    MAX(ls.selected_at) as last_selection_time
FROM load_balancing_selections ls
JOIN load_balancing_pools p ON ls.pool_id = p.id
WHERE ls.selected_at >= DATE_SUB(NOW(), INTERVAL 24 HOUR)
GROUP BY ls.pool_id, p.name;

-- Workspace performance in pools view
CREATE OR REPLACE VIEW workspace_pool_performance_view AS
SELECT 
    ls.pool_id,
    ls.workspace_id,
    p.name as pool_name,
    pw.weight,
    pw.enabled as workspace_enabled,
    COALESCE(ph.is_healthy, TRUE) as is_healthy,
    COUNT(*) as selections_24h,
    COUNT(CASE WHEN ls.success THEN 1 END) as successful_selections_24h,
    ROUND(COUNT(CASE WHEN ls.success THEN 1 END) * 100.0 / COUNT(*), 2) as success_rate_pct,
    AVG(ls.capacity_score) as avg_capacity_score,
    AVG(ls.response_time_ms) as avg_response_time_ms,
    MAX(ls.selected_at) as last_selected_at
FROM load_balancing_selections ls
JOIN load_balancing_pools p ON ls.pool_id = p.id
JOIN pool_workspaces pw ON ls.pool_id = pw.pool_id AND ls.workspace_id = pw.workspace_id
LEFT JOIN pool_workspace_health ph ON ls.pool_id = ph.pool_id AND ls.workspace_id = ph.workspace_id
WHERE ls.selected_at >= DATE_SUB(NOW(), INTERVAL 24 HOUR)
GROUP BY ls.pool_id, ls.workspace_id, p.name, pw.weight, pw.enabled, ph.is_healthy;

-- Indexes on existing tables to support load balancing queries
-- These indexes improve performance of queries that join with load balancing tables

-- Add index to messages table for workspace_id if not already present
-- Note: These indexes may already exist, errors can be ignored
-- CREATE INDEX idx_workspace_status_time ON messages (workspace_id, status, queued_at);
-- CREATE INDEX idx_from_email_status ON messages (from_email, status);

-- Stored procedures for common operations

DELIMITER //

-- Procedure to update pool metrics (can be called by a scheduled job)
DROP PROCEDURE IF EXISTS UpdatePoolMetrics;
CREATE PROCEDURE UpdatePoolMetrics(IN target_date DATE)
BEGIN
    DECLARE done INT DEFAULT FALSE;
    DECLARE pool_id_var VARCHAR(255);
    DECLARE pool_cursor CURSOR FOR SELECT id FROM load_balancing_pools WHERE enabled = TRUE;
    DECLARE CONTINUE HANDLER FOR NOT FOUND SET done = TRUE;

    -- Set target date to today if not provided
    IF target_date IS NULL THEN
        SET target_date = CURDATE();
    END IF;

    OPEN pool_cursor;
    pool_loop: LOOP
        FETCH pool_cursor INTO pool_id_var;
        IF done THEN
            LEAVE pool_loop;
        END IF;

        -- Insert/update metrics for each hour of the target date
        INSERT INTO pool_metrics (
            pool_id, metric_date, hour_of_day,
            total_selections, successful_selections, failed_selections,
            avg_capacity_score, avg_response_time_ms,
            unique_senders, unique_workspaces_used
        )
        SELECT 
            pool_id_var,
            target_date,
            HOUR(selected_at) as hour_of_day,
            COUNT(*) as total_selections,
            SUM(CASE WHEN success THEN 1 ELSE 0 END) as successful_selections,
            SUM(CASE WHEN NOT success THEN 1 ELSE 0 END) as failed_selections,
            AVG(capacity_score) as avg_capacity_score,
            AVG(response_time_ms) as avg_response_time_ms,
            COUNT(DISTINCT sender_email) as unique_senders,
            COUNT(DISTINCT workspace_id) as unique_workspaces_used
        FROM load_balancing_selections
        WHERE pool_id = pool_id_var 
          AND DATE(selected_at) = target_date
        GROUP BY HOUR(selected_at)
        ON DUPLICATE KEY UPDATE
            total_selections = VALUES(total_selections),
            successful_selections = VALUES(successful_selections),
            failed_selections = VALUES(failed_selections),
            avg_capacity_score = VALUES(avg_capacity_score),
            avg_response_time_ms = VALUES(avg_response_time_ms),
            unique_senders = VALUES(unique_senders),
            unique_workspaces_used = VALUES(unique_workspaces_used);

    END LOOP;
    CLOSE pool_cursor;
END//

-- Procedure to cleanup old selection history (retention policy)
DROP PROCEDURE IF EXISTS CleanupOldSelections;
CREATE PROCEDURE CleanupOldSelections(IN retention_days INT)
BEGIN
    DECLARE deleted_count INT DEFAULT 0;
    
    -- Set default retention to 30 days if not provided
    IF retention_days IS NULL OR retention_days <= 0 THEN
        SET retention_days = 30;
    END IF;

    -- Delete old selection records
    DELETE FROM load_balancing_selections 
    WHERE selected_at < DATE_SUB(NOW(), INTERVAL retention_days DAY);
    
    SET deleted_count = ROW_COUNT();
    
    -- Log the cleanup
    SELECT CONCAT('Cleaned up ', deleted_count, ' selection records older than ', retention_days, ' days') as result;
END//

-- Function to get pool health score (0-1 based on healthy workspaces)
-- Note: Commented out as it requires SUPER privileges for binary logging
-- Uncomment and run manually if needed
/*
DROP FUNCTION IF EXISTS GetPoolHealthScore;
CREATE FUNCTION GetPoolHealthScore(pool_id_param VARCHAR(255))
RETURNS DECIMAL(4,3)
READS SQL DATA
DETERMINISTIC
BEGIN
    DECLARE total_workspaces INT DEFAULT 0;
    DECLARE healthy_workspaces INT DEFAULT 0;
    DECLARE health_score DECIMAL(4,3) DEFAULT 0.000;
    
    -- Count total and healthy workspaces
    SELECT 
        COUNT(*),
        COUNT(CASE WHEN COALESCE(ph.is_healthy, TRUE) AND pw.enabled THEN 1 END)
    INTO total_workspaces, healthy_workspaces
    FROM pool_workspaces pw
    LEFT JOIN pool_workspace_health ph ON pw.pool_id = ph.pool_id AND pw.workspace_id = ph.workspace_id
    WHERE pw.pool_id = pool_id_param;
    
    -- Calculate health score
    IF total_workspaces > 0 THEN
        SET health_score = healthy_workspaces / total_workspaces;
    END IF;
    
    RETURN health_score;
END//
*/

DELIMITER ;

-- Insert sample data for testing (commented out for production)
/*
-- Sample load balancing pool
INSERT INTO load_balancing_pools (id, name, domain_patterns, strategy, enabled) VALUES
('invite-domain-pool', 'Invite Domain Distribution', '["invite.com", "invitations.mednet.org"]', 'capacity_weighted', TRUE);

-- Sample pool workspaces (you'll need to update these with actual workspace IDs)
INSERT INTO pool_workspaces (pool_id, workspace_id, weight, enabled) VALUES
('invite-domain-pool', 'gmail-workspace-1', 2.0, TRUE),
('invite-domain-pool', 'mailgun-workspace-1', 1.5, TRUE),
('invite-domain-pool', 'mandrill-workspace-1', 1.0, TRUE);
*/

-- Grant permissions (adjust as needed for your user)
-- GRANT SELECT, INSERT, UPDATE, DELETE ON relay.load_balancing_pools TO 'relay_user'@'%';
-- GRANT SELECT, INSERT, UPDATE, DELETE ON relay.pool_workspaces TO 'relay_user'@'%';
-- GRANT SELECT, INSERT, UPDATE, DELETE ON relay.load_balancing_selections TO 'relay_user'@'%';
-- GRANT SELECT, INSERT, UPDATE, DELETE ON relay.pool_workspace_health TO 'relay_user'@'%';
-- GRANT SELECT, INSERT, UPDATE, DELETE ON relay.pool_metrics TO 'relay_user'@'%';

-- Create events for automated maintenance (if MySQL Event Scheduler is enabled)
-- SET GLOBAL event_scheduler = ON;

-- Event to update pool metrics daily
/*
CREATE EVENT IF NOT EXISTS update_pool_metrics_daily
ON SCHEDULE EVERY 1 DAY
STARTS CONCAT(CURDATE(), ' 01:00:00')
DO
  CALL UpdatePoolMetrics(CURDATE() - INTERVAL 1 DAY);
*/

-- Event to cleanup old selection history weekly
/*
CREATE EVENT IF NOT EXISTS cleanup_old_selections_weekly
ON SCHEDULE EVERY 1 WEEK
STARTS CONCAT(CURDATE(), ' 02:00:00')
DO
  CALL CleanupOldSelections(30);
*/