-- ====================================================================
-- Workspace Data Migration Script
-- Version: 005
-- 
-- This script provides stored procedures to migrate workspace configuration
-- data from the old JSON-based structure to the new normalized tables.
-- 
-- Run this AFTER 004_comprehensive_provider_management.sql
-- ====================================================================

USE relay;

-- ====================================================================
-- Migration Helper Procedures
-- ====================================================================

DELIMITER //

-- Procedure to migrate a single workspace from JSON config
DROP PROCEDURE IF EXISTS MigrateWorkspaceFromJSON//
CREATE PROCEDURE MigrateWorkspaceFromJSON(
    IN p_workspace_id VARCHAR(255),
    IN p_display_name VARCHAR(255),
    IN p_domain VARCHAR(255),
    IN p_rate_limits JSON,
    IN p_gmail_config JSON,
    IN p_mailgun_config JSON,
    IN p_mandrill_config JSON,
    IN p_created_by VARCHAR(255)
)
BEGIN
    DECLARE EXIT HANDLER FOR SQLEXCEPTION
    BEGIN
        GET DIAGNOSTICS CONDITION 1
            @error_code = RETURNED_SQLSTATE,
            @error_message = MESSAGE_TEXT;
        
        SELECT CONCAT('Migration failed for workspace ', p_workspace_id, ': ', @error_message) AS error;
        ROLLBACK;
        RESIGNAL;
    END;

    START TRANSACTION;
    
    -- 1. Create or update workspace
    INSERT INTO workspaces (id, display_name, domain, enabled, created_by)
    VALUES (p_workspace_id, p_display_name, p_domain, TRUE, p_created_by)
    ON DUPLICATE KEY UPDATE
        display_name = VALUES(display_name),
        domain = VALUES(domain),
        updated_at = CURRENT_TIMESTAMP;
    
    -- 2. Add primary domain
    INSERT INTO workspace_domains (workspace_id, domain, is_primary, verified)
    VALUES (p_workspace_id, p_domain, TRUE, TRUE)
    ON DUPLICATE KEY UPDATE
        is_primary = VALUES(is_primary),
        verified = VALUES(verified);
    
    -- 3. Migrate rate limits
    IF p_rate_limits IS NOT NULL THEN
        INSERT INTO workspace_rate_limits (
            workspace_id, 
            workspace_daily, 
            per_user_daily
        ) VALUES (
            p_workspace_id,
            JSON_EXTRACT(p_rate_limits, '$.workspace_daily'),
            JSON_EXTRACT(p_rate_limits, '$.per_user_daily')
        )
        ON DUPLICATE KEY UPDATE
            workspace_daily = VALUES(workspace_daily),
            per_user_daily = VALUES(per_user_daily);
        
        -- Migrate custom user limits
        IF JSON_CONTAINS_PATH(p_rate_limits, 'one', '$.custom_user_limits') THEN
            SET @custom_limits = JSON_EXTRACT(p_rate_limits, '$.custom_user_limits');
            SET @keys = JSON_KEYS(@custom_limits);
            SET @key_count = JSON_LENGTH(@keys);
            SET @counter = 0;
            
            WHILE @counter < @key_count DO
                SET @email = JSON_UNQUOTE(JSON_EXTRACT(@keys, CONCAT('$[', @counter, ']')));
                SET @limit = JSON_EXTRACT(@custom_limits, CONCAT('$.', @email));
                
                INSERT INTO workspace_user_rate_limits (
                    workspace_id, email_address, daily_limit, 
                    note, created_by
                ) VALUES (
                    p_workspace_id, @email, @limit,
                    'Migrated from workspace.json', p_created_by
                )
                ON DUPLICATE KEY UPDATE
                    daily_limit = VALUES(daily_limit);
                
                SET @counter = @counter + 1;
            END WHILE;
        END IF;
    END IF;
    
    -- 4. Migrate Gmail provider
    IF p_gmail_config IS NOT NULL THEN
        SET @gmail_enabled = COALESCE(JSON_EXTRACT(p_gmail_config, '$.enabled'), TRUE);
        
        IF @gmail_enabled THEN
            INSERT INTO workspace_providers (
                workspace_id, provider_type, provider_name,
                enabled, priority, weight, config
            ) VALUES (
                p_workspace_id, 'gmail', 'Primary Gmail',
                @gmail_enabled, 10, 1.0, p_gmail_config
            );
            
            SET @gmail_provider_id = LAST_INSERT_ID();
            
            INSERT INTO gmail_provider_configs (
                provider_id,
                service_account_file,
                service_account_env,
                default_sender,
                require_valid_sender
            ) VALUES (
                @gmail_provider_id,
                JSON_UNQUOTE(JSON_EXTRACT(p_gmail_config, '$.service_account_file')),
                JSON_UNQUOTE(JSON_EXTRACT(p_gmail_config, '$.service_account_env')),
                JSON_UNQUOTE(JSON_EXTRACT(p_gmail_config, '$.default_sender')),
                COALESCE(JSON_EXTRACT(p_gmail_config, '$.require_valid_sender'), TRUE)
            );
            
            -- Migrate Gmail header rewrite rules
            IF JSON_CONTAINS_PATH(p_gmail_config, 'one', '$.header_rewrite.rules') THEN
                SET @gmail_rules = JSON_EXTRACT(p_gmail_config, '$.header_rewrite.rules');
                SET @rule_count = JSON_LENGTH(@gmail_rules);
                SET @rule_counter = 0;
                
                WHILE @rule_counter < @rule_count DO
                    SET @rule = JSON_EXTRACT(@gmail_rules, CONCAT('$[', @rule_counter, ']'));
                    
                    INSERT INTO provider_header_rewrite_rules (
                        provider_id, header_name, new_value, rule_order,
                        description
                    ) VALUES (
                        @gmail_provider_id,
                        JSON_UNQUOTE(JSON_EXTRACT(@rule, '$.header_name')),
                        JSON_UNQUOTE(JSON_EXTRACT(@rule, '$.new_value')),
                        @rule_counter * 10,
                        CONCAT('Migrated Gmail rule for ', JSON_UNQUOTE(JSON_EXTRACT(@rule, '$.header_name')))
                    );
                    
                    SET @rule_counter = @rule_counter + 1;
                END WHILE;
            END IF;
        END IF;
    END IF;
    
    -- 5. Migrate Mailgun provider
    IF p_mailgun_config IS NOT NULL THEN
        SET @mailgun_enabled = COALESCE(JSON_EXTRACT(p_mailgun_config, '$.enabled'), TRUE);
        
        IF @mailgun_enabled THEN
            INSERT INTO workspace_providers (
                workspace_id, provider_type, provider_name,
                enabled, priority, weight, config
            ) VALUES (
                p_workspace_id, 'mailgun', 'Primary Mailgun',
                @mailgun_enabled, 20, 1.0, p_mailgun_config
            );
            
            SET @mailgun_provider_id = LAST_INSERT_ID();
            
            INSERT INTO mailgun_provider_configs (
                provider_id,
                api_key_env,
                domain,
                base_url,
                region,
                track_opens,
                track_clicks,
                track_unsubscribes
            ) VALUES (
                @mailgun_provider_id,
                'MAILGUN_API_KEY',
                JSON_UNQUOTE(JSON_EXTRACT(p_mailgun_config, '$.domain')),
                COALESCE(JSON_UNQUOTE(JSON_EXTRACT(p_mailgun_config, '$.base_url')), 'https://api.mailgun.net/v3'),
                COALESCE(JSON_UNQUOTE(JSON_EXTRACT(p_mailgun_config, '$.region')), 'us'),
                COALESCE(JSON_EXTRACT(p_mailgun_config, '$.tracking.opens'), TRUE),
                COALESCE(JSON_EXTRACT(p_mailgun_config, '$.tracking.clicks'), TRUE),
                COALESCE(JSON_EXTRACT(p_mailgun_config, '$.tracking.unsubscribe'), TRUE)
            );
            
            -- Migrate Mailgun header rewrite rules
            IF JSON_CONTAINS_PATH(p_mailgun_config, 'one', '$.header_rewrite.rules') THEN
                SET @mailgun_rules = JSON_EXTRACT(p_mailgun_config, '$.header_rewrite.rules');
                SET @rule_count = JSON_LENGTH(@mailgun_rules);
                SET @rule_counter = 0;
                
                WHILE @rule_counter < @rule_count DO
                    SET @rule = JSON_EXTRACT(@mailgun_rules, CONCAT('$[', @rule_counter, ']'));
                    
                    INSERT INTO provider_header_rewrite_rules (
                        provider_id, header_name, new_value, rule_order,
                        description
                    ) VALUES (
                        @mailgun_provider_id,
                        JSON_UNQUOTE(JSON_EXTRACT(@rule, '$.header_name')),
                        JSON_UNQUOTE(JSON_EXTRACT(@rule, '$.new_value')),
                        @rule_counter * 10,
                        CONCAT('Migrated Mailgun rule for ', JSON_UNQUOTE(JSON_EXTRACT(@rule, '$.header_name')))
                    );
                    
                    SET @rule_counter = @rule_counter + 1;
                END WHILE;
            END IF;
        END IF;
    END IF;
    
    -- 6. Migrate Mandrill provider
    IF p_mandrill_config IS NOT NULL THEN
        SET @mandrill_enabled = COALESCE(JSON_EXTRACT(p_mandrill_config, '$.enabled'), TRUE);
        
        IF @mandrill_enabled THEN
            INSERT INTO workspace_providers (
                workspace_id, provider_type, provider_name,
                enabled, priority, weight, config
            ) VALUES (
                p_workspace_id, 'mandrill', 'Primary Mandrill',
                @mandrill_enabled, 30, 1.0, p_mandrill_config
            );
            
            SET @mandrill_provider_id = LAST_INSERT_ID();
            
            INSERT INTO mandrill_provider_configs (
                provider_id,
                api_key_env,
                base_url,
                subaccount,
                default_tags,
                track_opens,
                track_clicks,
                auto_text,
                auto_html,
                inline_css,
                url_strip_qs
            ) VALUES (
                @mandrill_provider_id,
                'MANDRILL_API_KEY',
                COALESCE(JSON_UNQUOTE(JSON_EXTRACT(p_mandrill_config, '$.base_url')), 'https://mandrillapp.com/api/1.0'),
                JSON_UNQUOTE(JSON_EXTRACT(p_mandrill_config, '$.subaccount')),
                JSON_EXTRACT(p_mandrill_config, '$.default_tags'),
                COALESCE(JSON_EXTRACT(p_mandrill_config, '$.tracking.opens'), TRUE),
                COALESCE(JSON_EXTRACT(p_mandrill_config, '$.tracking.clicks'), TRUE),
                COALESCE(JSON_EXTRACT(p_mandrill_config, '$.tracking.auto_text'), TRUE),
                COALESCE(JSON_EXTRACT(p_mandrill_config, '$.tracking.auto_html'), FALSE),
                COALESCE(JSON_EXTRACT(p_mandrill_config, '$.tracking.inline_css'), TRUE),
                COALESCE(JSON_EXTRACT(p_mandrill_config, '$.tracking.url_strip_qs'), FALSE)
            );
            
            -- Migrate Mandrill header rewrite rules
            IF JSON_CONTAINS_PATH(p_mandrill_config, 'one', '$.header_rewrite.rules') THEN
                SET @mandrill_rules = JSON_EXTRACT(p_mandrill_config, '$.header_rewrite.rules');
                SET @rule_count = JSON_LENGTH(@mandrill_rules);
                SET @rule_counter = 0;
                
                WHILE @rule_counter < @rule_count DO
                    SET @rule = JSON_EXTRACT(@mandrill_rules, CONCAT('$[', @rule_counter, ']'));
                    
                    INSERT INTO provider_header_rewrite_rules (
                        provider_id, header_name, new_value, rule_order,
                        description
                    ) VALUES (
                        @mandrill_provider_id,
                        JSON_UNQUOTE(JSON_EXTRACT(@rule, '$.header_name')),
                        JSON_UNQUOTE(JSON_EXTRACT(@rule, '$.new_value')),
                        @rule_counter * 10,
                        CONCAT('Migrated Mandrill rule for ', JSON_UNQUOTE(JSON_EXTRACT(@rule, '$.header_name')))
                    );
                    
                    SET @rule_counter = @rule_counter + 1;
                END WHILE;
            END IF;
        END IF;
    END IF;
    
    COMMIT;
    
    SELECT CONCAT('Successfully migrated workspace: ', p_workspace_id) AS result;
END//

-- Procedure to migrate sample workspaces from the example files
DROP PROCEDURE IF EXISTS MigrateSampleWorkspaces//
CREATE PROCEDURE MigrateSampleWorkspaces()
BEGIN
    DECLARE EXIT HANDLER FOR SQLEXCEPTION
    BEGIN
        GET DIAGNOSTICS CONDITION 1
            @error_code = RETURNED_SQLSTATE,
            @error_message = MESSAGE_TEXT;
        
        SELECT CONCAT('Sample migration failed: ', @error_message) AS error;
        ROLLBACK;
        RESIGNAL;
    END;

    START TRANSACTION;
    
    -- Migrate joinmednet workspace
    CALL MigrateWorkspaceFromJSON(
        'joinmednet',
        'Mednet Primary',
        'joinmednet.org',
        JSON_OBJECT(
            'workspace_daily', 10000,
            'per_user_daily', 1000,
            'custom_user_limits', JSON_OBJECT(
                'vip@joinmednet.org', 5000,
                'bulk@joinmednet.org', 500
            )
        ),
        JSON_OBJECT(
            'service_account_file', 'credentials/joinmednet-service-account.json',
            'enabled', TRUE,
            'default_sender', 'noreply@joinmednet.org',
            'require_valid_sender', TRUE
        ),
        NULL,
        NULL,
        'system-migration'
    );
    
    -- Migrate client1 workspace  
    CALL MigrateWorkspaceFromJSON(
        'client1',
        'Client 1 Workspace',
        'client1.com',
        JSON_OBJECT(
            'per_user_daily', 500
        ),
        JSON_OBJECT(
            'service_account_file', 'credentials/client1-service-account.json',
            'enabled', TRUE,
            'default_sender', 'support@client1.com',
            'require_valid_sender', FALSE
        ),
        NULL,
        NULL,
        'system-migration'
    );
    
    -- Migrate testing workspace
    CALL MigrateWorkspaceFromJSON(
        'testing',
        'Testing Environment',
        'testing.example.com',
        JSON_OBJECT(
            'workspace_daily', 100,
            'per_user_daily', 10
        ),
        JSON_OBJECT(
            'service_account_file', 'credentials/testing-service-account.json',
            'enabled', TRUE,
            'require_valid_sender', FALSE
        ),
        NULL,
        NULL,
        'system-migration'
    );
    
    -- Migrate mailgun example with header rewrite
    CALL MigrateWorkspaceFromJSON(
        'mailgun-with-header-rewrite',
        'Mailgun with Header Rewriting',
        'mail.example.com',
        JSON_OBJECT(
            'workspace_daily', 10000,
            'per_user_daily', 500
        ),
        NULL,
        JSON_OBJECT(
            'api_key', 'your-mailgun-api-key',
            'domain', 'mail.example.com',
            'base_url', 'https://api.mailgun.net/v3',
            'region', 'us',
            'enabled', TRUE,
            'tracking', JSON_OBJECT(
                'opens', TRUE,
                'clicks', TRUE,
                'unsubscribe', TRUE
            ),
            'header_rewrite', JSON_OBJECT(
                'enabled', TRUE,
                'rules', JSON_ARRAY(
                    JSON_OBJECT(
                        'header_name', 'List-Unsubscribe',
                        'new_value', '<https://mail.example.com/unsubscribe?email=%recipient%>, <mailto:unsubscribe@mail.example.com?subject=unsubscribe>'
                    ),
                    JSON_OBJECT(
                        'header_name', 'List-Unsubscribe-Post',
                        'new_value', 'List-Unsubscribe=One-Click'
                    ),
                    JSON_OBJECT(
                        'header_name', 'Return-Path',
                        'new_value', 'bounces@mail.example.com'
                    )
                )
            )
        ),
        NULL,
        'system-migration'
    );
    
    -- Migrate mandrill example
    CALL MigrateWorkspaceFromJSON(
        'mandrill-transactional',
        'Mandrill Transactional Workspace',
        'example.com',
        JSON_OBJECT(
            'workspace_daily', 10000,
            'per_user_daily', 500
        ),
        NULL,
        NULL,
        JSON_OBJECT(
            'api_key', '${MANDRILL_API_KEY}',
            'base_url', 'https://mandrillapp.com/api/1.0',
            'enabled', TRUE,
            'subaccount', 'production',
            'default_tags', JSON_ARRAY('transactional', 'production'),
            'tracking', JSON_OBJECT(
                'opens', TRUE,
                'clicks', TRUE,
                'auto_text', TRUE,
                'auto_html', FALSE,
                'inline_css', TRUE,
                'url_strip_qs', FALSE
            ),
            'header_rewrite', JSON_OBJECT(
                'enabled', TRUE,
                'rules', JSON_ARRAY(
                    JSON_OBJECT(
                        'header_name', 'List-Unsubscribe',
                        'new_value', '<mailto:unsubscribe@example.com?subject=Unsubscribe>'
                    ),
                    JSON_OBJECT(
                        'header_name', 'List-Unsubscribe-Post',
                        'new_value', 'List-Unsubscribe=One-Click'
                    )
                )
            )
        ),
        'system-migration'
    );
    
    COMMIT;
    
    SELECT 'All sample workspaces migrated successfully' AS result;
END//

-- Procedure to validate migration
DROP PROCEDURE IF EXISTS ValidateMigration//
CREATE PROCEDURE ValidateMigration()
BEGIN
    SELECT 'Migration Validation Report' AS report_type;
    
    SELECT 
        'Workspaces' AS entity,
        COUNT(*) AS total_count,
        COUNT(CASE WHEN enabled THEN 1 END) AS enabled_count
    FROM workspaces
    UNION ALL
    SELECT 
        'Workspace Domains' AS entity,
        COUNT(*) AS total_count,
        COUNT(CASE WHEN verified THEN 1 END) AS verified_count
    FROM workspace_domains
    UNION ALL
    SELECT 
        'Rate Limit Configs' AS entity,
        COUNT(*) AS total_count,
        COUNT(CASE WHEN workspace_daily IS NOT NULL THEN 1 END) AS with_workspace_limits
    FROM workspace_rate_limits
    UNION ALL
    SELECT 
        'Custom User Limits' AS entity,
        COUNT(*) AS total_count,
        AVG(daily_limit) AS avg_limit
    FROM workspace_user_rate_limits
    UNION ALL
    SELECT 
        'Providers' AS entity,
        COUNT(*) AS total_count,
        COUNT(CASE WHEN enabled THEN 1 END) AS enabled_count
    FROM workspace_providers
    UNION ALL
    SELECT 
        'Header Rewrite Rules' AS entity,
        COUNT(*) AS total_count,
        COUNT(CASE WHEN enabled THEN 1 END) AS enabled_count
    FROM provider_header_rewrite_rules;
    
    -- Provider breakdown
    SELECT 'Provider Breakdown' AS report_section;
    SELECT 
        provider_type,
        COUNT(*) AS count,
        COUNT(CASE WHEN enabled THEN 1 END) AS enabled_count,
        COUNT(CASE WHEN is_healthy THEN 1 END) AS healthy_count
    FROM workspace_providers
    GROUP BY provider_type;
    
    -- Workspaces with multiple providers
    SELECT 'Multi-Provider Workspaces' AS report_section;
    SELECT 
        workspace_id,
        COUNT(*) AS provider_count,
        GROUP_CONCAT(provider_type) AS provider_types
    FROM workspace_providers
    GROUP BY workspace_id
    HAVING COUNT(*) > 1;
END//

DELIMITER ;

-- ====================================================================
-- Data Cleanup Procedures
-- ====================================================================

DELIMITER //

-- Procedure to clean up orphaned records
DROP PROCEDURE IF EXISTS CleanupOrphanedRecords//
CREATE PROCEDURE CleanupOrphanedRecords()
BEGIN
    DECLARE orphaned_count INT DEFAULT 0;
    
    -- Clean up provider configs without providers
    DELETE gpc FROM gmail_provider_configs gpc
    LEFT JOIN workspace_providers wp ON gpc.provider_id = wp.id
    WHERE wp.id IS NULL;
    SET orphaned_count = orphaned_count + ROW_COUNT();
    
    DELETE mgc FROM mailgun_provider_configs mgc
    LEFT JOIN workspace_providers wp ON mgc.provider_id = wp.id
    WHERE wp.id IS NULL;
    SET orphaned_count = orphaned_count + ROW_COUNT();
    
    DELETE mdc FROM mandrill_provider_configs mdc
    LEFT JOIN workspace_providers wp ON mdc.provider_id = wp.id
    WHERE wp.id IS NULL;
    SET orphaned_count = orphaned_count + ROW_COUNT();
    
    -- Clean up header rules without providers
    DELETE phrr FROM provider_header_rewrite_rules phrr
    LEFT JOIN workspace_providers wp ON phrr.provider_id = wp.id
    WHERE wp.id IS NULL;
    SET orphaned_count = orphaned_count + ROW_COUNT();
    
    -- Clean up rate limits without workspaces
    DELETE wrl FROM workspace_rate_limits wrl
    LEFT JOIN workspaces w ON wrl.workspace_id = w.id
    WHERE w.id IS NULL;
    SET orphaned_count = orphaned_count + ROW_COUNT();
    
    SELECT CONCAT('Cleaned up ', orphaned_count, ' orphaned records') AS result;
END//

DELIMITER ;

-- ====================================================================
-- Usage Instructions
-- ====================================================================

-- To migrate sample workspaces from the example JSON files:
-- CALL MigrateSampleWorkspaces();

-- To validate the migration:
-- CALL ValidateMigration();

-- To clean up any orphaned records:
-- CALL CleanupOrphanedRecords();

-- To migrate a custom workspace from JSON:
-- CALL MigrateWorkspaceFromJSON(
--     'your-workspace-id',
--     'Your Workspace Name',
--     'yourdomain.com',
--     JSON_OBJECT('workspace_daily', 5000, 'per_user_daily', 100),
--     JSON_OBJECT('enabled', TRUE, 'default_sender', 'noreply@yourdomain.com'),
--     NULL, -- no mailgun config
--     NULL, -- no mandrill config
--     'your-username'
-- );

SELECT 'Migration procedures created successfully. Use CALL MigrateSampleWorkspaces() to migrate example data.' AS status;