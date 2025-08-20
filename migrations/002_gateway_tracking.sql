-- Migration 002: Add gateway tracking support
-- Adds columns to track which gateway was used for each message delivery

-- Add gateway tracking columns to message_recipients table
ALTER TABLE message_recipients 
ADD COLUMN gateway_id VARCHAR(255) NULL AFTER bounce_reason,
ADD COLUMN gateway_type VARCHAR(50) NULL AFTER gateway_id,
ADD COLUMN send_attempt_count INT NOT NULL DEFAULT 1 AFTER gateway_type,
ADD COLUMN last_send_attempt TIMESTAMP NULL AFTER send_attempt_count;

-- Add indexes for gateway tracking
ALTER TABLE message_recipients
ADD INDEX idx_gateway_tracking (gateway_id, gateway_type),
ADD INDEX idx_gateway_delivery (gateway_id, delivery_status, sent_at),
ADD INDEX idx_send_attempts (send_attempt_count, last_send_attempt);

-- Create gateway usage statistics table for aggregated metrics
CREATE TABLE IF NOT EXISTS gateway_usage_stats (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    gateway_id VARCHAR(255) NOT NULL,
    gateway_type VARCHAR(50) NOT NULL,
    date_bucket DATE NOT NULL,  -- Daily aggregation bucket
    hour_bucket TINYINT NULL,   -- Hour of day (0-23) for hourly stats, NULL for daily
    
    -- Volume metrics
    total_attempts INT NOT NULL DEFAULT 0,
    total_sent INT NOT NULL DEFAULT 0,
    total_bounced INT NOT NULL DEFAULT 0,
    total_failed INT NOT NULL DEFAULT 0,
    total_deferred INT NOT NULL DEFAULT 0,
    
    -- Performance metrics
    average_latency_ms INT NULL,
    success_rate DECIMAL(5,2) NOT NULL DEFAULT 0.00,
    
    -- Rate limiting metrics
    rate_limit_hits INT NOT NULL DEFAULT 0,
    circuit_breaker_trips INT NOT NULL DEFAULT 0,
    
    -- Recipient engagement (filled from webhook events)
    total_opens INT NOT NULL DEFAULT 0,
    total_clicks INT NOT NULL DEFAULT 0,
    unique_opens INT NOT NULL DEFAULT 0,
    unique_clicks INT NOT NULL DEFAULT 0,
    
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    
    -- Unique constraint to prevent duplicate stat records
    UNIQUE KEY uk_gateway_date_hour (gateway_id, date_bucket, hour_bucket),
    
    -- Performance indexes
    INDEX idx_gateway_date (gateway_id, date_bucket),
    INDEX idx_gateway_type_date (gateway_type, date_bucket),
    INDEX idx_date_bucket (date_bucket),
    INDEX idx_performance_tracking (gateway_id, success_rate, average_latency_ms)
);

-- Create gateway configuration audit table to track config changes
CREATE TABLE IF NOT EXISTS gateway_config_audit (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    gateway_id VARCHAR(255) NOT NULL,
    gateway_type VARCHAR(50) NOT NULL,
    change_type ENUM('CREATED', 'UPDATED', 'DELETED', 'ENABLED', 'DISABLED') NOT NULL,
    old_config JSON NULL,
    new_config JSON NULL,
    changed_by VARCHAR(255) NULL,  -- User or system that made the change
    change_reason TEXT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    
    -- Indexes for audit tracking
    INDEX idx_gateway_audit (gateway_id, change_type, created_at),
    INDEX idx_change_tracking (change_type, created_at),
    INDEX idx_changed_by (changed_by)
);

-- Create gateway health status table for real-time monitoring
CREATE TABLE IF NOT EXISTS gateway_health_status (
    gateway_id VARCHAR(255) PRIMARY KEY,
    gateway_type VARCHAR(50) NOT NULL,
    status ENUM('HEALTHY', 'DEGRADED', 'UNHEALTHY', 'DISABLED') NOT NULL DEFAULT 'HEALTHY',
    last_health_check TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    consecutive_failures INT NOT NULL DEFAULT 0,
    consecutive_successes INT NOT NULL DEFAULT 0,
    last_error TEXT NULL,
    circuit_breaker_state ENUM('CLOSED', 'OPEN', 'HALF_OPEN') NULL,
    circuit_breaker_failure_count INT NOT NULL DEFAULT 0,
    circuit_breaker_last_failure TIMESTAMP NULL,
    rate_limit_remaining INT NULL,
    rate_limit_reset_time TIMESTAMP NULL,
    
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    
    -- Indexes for health monitoring
    INDEX idx_status_check (status, last_health_check),
    INDEX idx_gateway_type_status (gateway_type, status),
    INDEX idx_circuit_breaker (circuit_breaker_state, circuit_breaker_failure_count),
    INDEX idx_rate_limiting (rate_limit_remaining, rate_limit_reset_time)
);

-- Create gateway routing rules table for dynamic routing configuration
CREATE TABLE IF NOT EXISTS gateway_routing_rules (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    gateway_id VARCHAR(255) NOT NULL,
    rule_type ENUM('PATTERN', 'DOMAIN', 'USER', 'CAMPAIGN', 'FALLBACK') NOT NULL,
    rule_pattern VARCHAR(500) NOT NULL,  -- Email pattern, domain, user ID, etc.
    priority INT NOT NULL DEFAULT 100,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    conditions JSON NULL,  -- Additional conditions for complex routing
    metadata JSON NULL,    -- Additional rule metadata
    
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    
    -- Indexes for routing performance
    INDEX idx_gateway_routing (gateway_id, rule_type, is_active, priority),
    INDEX idx_pattern_lookup (rule_pattern, rule_type, is_active),
    INDEX idx_priority_active (priority, is_active)
);

-- Update existing message_recipients with gateway_id for messages that have workspace_id
-- This provides backward compatibility for existing data
UPDATE message_recipients mr 
JOIN messages m ON mr.message_id = m.id 
SET mr.gateway_id = m.workspace_id, 
    mr.gateway_type = 'google_workspace'
WHERE mr.gateway_id IS NULL 
  AND m.workspace_id IS NOT NULL 
  AND m.workspace_id != '';

-- Create views for easy reporting

-- Gateway performance summary view
CREATE OR REPLACE VIEW gateway_performance_summary AS
SELECT 
    gateway_id,
    gateway_type,
    COUNT(*) as total_messages,
    SUM(CASE WHEN delivery_status = 'SENT' THEN 1 ELSE 0 END) as sent_count,
    SUM(CASE WHEN delivery_status = 'BOUNCED' THEN 1 ELSE 0 END) as bounced_count,
    SUM(CASE WHEN delivery_status = 'FAILED' THEN 1 ELSE 0 END) as failed_count,
    SUM(CASE WHEN delivery_status = 'DEFERRED' THEN 1 ELSE 0 END) as deferred_count,
    ROUND(
        100.0 * SUM(CASE WHEN delivery_status = 'SENT' THEN 1 ELSE 0 END) / COUNT(*),
        2
    ) as success_rate,
    SUM(opens) as total_opens,
    SUM(clicks) as total_clicks,
    COUNT(DISTINCT CASE WHEN opens > 0 THEN recipient_id END) as unique_opens,
    COUNT(DISTINCT CASE WHEN clicks > 0 THEN recipient_id END) as unique_clicks
FROM message_recipients 
WHERE gateway_id IS NOT NULL
GROUP BY gateway_id, gateway_type;

-- Daily gateway statistics view
CREATE OR REPLACE VIEW daily_gateway_stats AS
SELECT 
    gateway_id,
    gateway_type,
    DATE(created_at) as stat_date,
    COUNT(*) as daily_messages,
    SUM(CASE WHEN delivery_status = 'SENT' THEN 1 ELSE 0 END) as daily_sent,
    SUM(CASE WHEN delivery_status = 'BOUNCED' THEN 1 ELSE 0 END) as daily_bounced,
    SUM(CASE WHEN delivery_status = 'FAILED' THEN 1 ELSE 0 END) as daily_failed,
    ROUND(
        100.0 * SUM(CASE WHEN delivery_status = 'SENT' THEN 1 ELSE 0 END) / COUNT(*),
        2
    ) as daily_success_rate
FROM message_recipients 
WHERE gateway_id IS NOT NULL
  AND created_at >= DATE_SUB(CURDATE(), INTERVAL 30 DAY)
GROUP BY gateway_id, gateway_type, DATE(created_at)
ORDER BY gateway_id, stat_date DESC;

-- Gateway health monitoring view
CREATE OR REPLACE VIEW gateway_health_summary AS
SELECT 
    ghs.gateway_id,
    ghs.gateway_type,
    ghs.status,
    ghs.last_health_check,
    ghs.consecutive_failures,
    ghs.circuit_breaker_state,
    ghs.rate_limit_remaining,
    ghs.rate_limit_reset_time,
    COALESCE(perf.success_rate, 0) as current_success_rate,
    COALESCE(perf.total_messages, 0) as total_messages_today
FROM gateway_health_status ghs
LEFT JOIN (
    SELECT 
        gateway_id,
        ROUND(
            100.0 * SUM(CASE WHEN delivery_status = 'SENT' THEN 1 ELSE 0 END) / COUNT(*),
            2
        ) as success_rate,
        COUNT(*) as total_messages
    FROM message_recipients 
    WHERE gateway_id IS NOT NULL
      AND DATE(created_at) = CURDATE()
    GROUP BY gateway_id
) perf ON ghs.gateway_id = perf.gateway_id;