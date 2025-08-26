#!/bin/bash

# System Configuration Verification Script
# Checks database configuration and system readiness

echo "========================================="
echo "SMTP Relay System Configuration Check"
echo "========================================="
echo ""

# Database connection
DB_USER="relay"
DB_PASS="relay"
DB_NAME="relay"

# Function to run MySQL query
run_query() {
    mysql -u $DB_USER -p$DB_PASS $DB_NAME -e "$1" 2>/dev/null
}

# Function to check table existence
check_table() {
    local table=$1
    local count=$(mysql -u $DB_USER -p$DB_PASS $DB_NAME -se "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='$DB_NAME' AND table_name='$table'" 2>/dev/null)
    if [ "$count" -eq "1" ]; then
        echo "✓ Table $table exists"
        return 0
    else
        echo "✗ Table $table missing"
        return 1
    fi
}

echo "1. DATABASE TABLES"
echo "=================="
check_table "workspaces"
check_table "messages"
check_table "workspace_providers"
check_table "workspace_rate_limits"
check_table "workspace_user_rate_limits"
check_table "load_balancing_pools"
check_table "pool_workspaces"
echo ""

echo "2. CONFIGURED WORKSPACES"
echo "========================"
run_query "SELECT id, domain, provider_type, display_name FROM workspaces;"
echo ""

echo "3. PROVIDER CONFIGURATIONS"
echo "=========================="
run_query "SELECT wp.workspace_id, wp.provider_type, wp.enabled, wp.priority FROM workspace_providers wp ORDER BY wp.priority;"
echo ""

echo "4. LOAD BALANCING POOLS"
echo "======================="
run_query "SELECT id, name, strategy, enabled, is_default FROM load_balancing_pools;"
echo ""

echo "5. POOL WORKSPACE MAPPINGS"
echo "=========================="
run_query "SELECT pool_id, workspace_id, weight, enabled FROM pool_workspaces ORDER BY pool_id;"
echo ""

echo "6. POOL DOMAIN PATTERNS"
echo "======================="
run_query "SELECT pool_id, domain_pattern FROM pool_domain_patterns ORDER BY pool_id;"
echo ""

echo "7. RATE LIMITS"
echo "=============="
echo "Workspace Rate Limits:"
run_query "SELECT workspace_id, workspace_daily, per_user_daily FROM workspace_rate_limits;"
echo ""
echo "Custom User Rate Limits:"
run_query "SELECT workspace_id, email_address, daily_limit FROM workspace_user_rate_limits ORDER BY workspace_id;"
echo ""

echo "8. GMAIL PROVIDER CONFIGS"
echo "========================="
run_query "SELECT g.provider_id, wp.workspace_id, g.default_sender, 
         CASE WHEN g.service_account_file IS NOT NULL THEN 'File' 
              WHEN g.service_account_env IS NOT NULL THEN 'Env' 
              ELSE 'None' END as credential_source
         FROM gmail_provider_configs g 
         JOIN workspace_providers wp ON g.provider_id = wp.id;"
echo ""

echo "9. MAILGUN PROVIDER CONFIGS"
echo "==========================="
run_query "SELECT m.provider_id, wp.workspace_id, m.domain, m.region, 
         m.track_opens, m.track_clicks 
         FROM mailgun_provider_configs m 
         JOIN workspace_providers wp ON m.provider_id = wp.id;"
echo ""

echo "10. HEADER REWRITE RULES"
echo "========================"
run_query "SELECT hr.provider_id, wp.workspace_id, hr.header_name, hr.action, hr.new_value, hr.enabled 
         FROM provider_header_rewrite_rules hr 
         JOIN workspace_providers wp ON hr.provider_id = wp.id 
         ORDER BY wp.workspace_id, hr.priority;"
echo ""

echo "11. SYSTEM HEALTH CHECKS"
echo "========================"
# Check if services are running
echo -n "SMTP Server (port 2525): "
nc -z localhost 2525 2>/dev/null && echo "✓ Running" || echo "✗ Not running"

echo -n "API Server (port 8080): "
nc -z localhost 8080 2>/dev/null && echo "✓ Running" || echo "✗ Not running"

echo -n "Dashboard (port 3000): "
nc -z localhost 3000 2>/dev/null && echo "✓ Running" || echo "✗ Not running"
echo ""

echo "12. TEST SMTP CONNECTION"
echo "========================"
# Test SMTP authentication
(echo "EHLO test"; sleep 1; echo "QUIT") | nc localhost 2525 2>/dev/null | grep -q "250" && echo "✓ SMTP responds correctly" || echo "✗ SMTP not responding"
echo ""

echo "========================================="
echo "Configuration Check Complete"
echo "========================================="