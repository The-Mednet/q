# Email Gateway System Migration Guide

This guide provides step-by-step instructions for migrating from the current Google Workspace-only architecture to the new multi-gateway system.

## Overview

The new email gateway system provides:
- **Multi-Provider Support**: Google Workspace, Mailgun, SendGrid, Amazon SES
- **Intelligent Routing**: Priority-based, round-robin, weighted, and domain-based routing
- **Enhanced Reliability**: Circuit breakers, health checks, automatic failover
- **Advanced Rate Limiting**: Per-gateway, per-user, burst limiting with inheritance
- **Better Monitoring**: Comprehensive metrics and status tracking

## Migration Strategies

### 1. Compatibility Mode (Recommended)

Run both systems in parallel with gradual migration.

**Benefits:**
- Zero downtime
- Gradual testing and validation
- Easy rollback
- Minimal risk

**Implementation:**
```go
// Current workspace.json stays unchanged
// Add new workspaces-gateway.json alongside it

migrationManager := migration.NewMigrationManager(
    legacyGmailConfig,
    newGatewayConfig,
    migration.MigrationModeCompatibility,
)
```

### 2. Gradual Mode

Migrate domains one by one to the new system.

**Benefits:**
- Domain-by-domain control
- Phased rollout
- Performance testing per domain

### 3. Immediate Mode

Switch entirely to the new system.

**Benefits:**
- Full feature access immediately
- Simplified architecture
- No dual maintenance

**Risks:**
- Potential service disruption
- Requires thorough testing

## Pre-Migration Checklist

### 1. Environment Preparation

- [ ] Backup current `workspace.json` configuration
- [ ] Install required dependencies
- [ ] Set up monitoring for both systems
- [ ] Prepare rollback procedures

### 2. Configuration Validation

- [ ] Validate all existing workspace configurations
- [ ] Test Gmail API access for all workspaces
- [ ] Verify rate limits are correctly configured
- [ ] Check service account permissions

### 3. Testing Environment

- [ ] Set up staging environment with new gateway system
- [ ] Test basic email sending functionality
- [ ] Validate recipient tracking integration
- [ ] Test webhook functionality
- [ ] Verify rate limiting behavior

## Step-by-Step Migration

### Phase 1: Setup New Configuration Structure

1. **Create Enhanced Gateway Configuration**

```bash
# Copy existing workspace.json to backup
cp workspace.json workspace-legacy-backup.json

# Create new gateway configuration
cp workspaces-gateway-example.json workspaces-gateway.json
```

2. **Convert Legacy Configuration**

```go
// The system automatically converts legacy configurations
gatewayConfig, err := config.LoadGatewayConfig("workspace.json")
if err != nil {
    log.Fatal("Failed to load gateway config:", err)
}

// This will automatically detect and convert legacy format
```

3. **Update Environment Variables**

```bash
# Add new environment variables for gateway system
GATEWAY_CONFIG_FILE=workspaces-gateway.json
GATEWAY_MIGRATION_MODE=compatibility

# Optional: Mailgun configuration for new gateways
MAILGUN_API_KEY=your-mailgun-api-key
MAILGUN_DOMAIN=mg.yourdomain.com
```

### Phase 2: Deploy Compatibility Layer

1. **Update Main Application**

```go
// In cmd/server/main.go
func main() {
    // ... existing code ...
    
    // Load both configurations
    legacyConfig := &cfg.Gmail
    gatewayConfig, err := config.LoadGatewayConfig("workspaces-gateway.json")
    if err != nil {
        gatewayConfig = convertLegacyConfig(legacyConfig)
    }
    
    // Create migration manager
    migrationManager := migration.NewMigrationManager(
        legacyConfig,
        gatewayConfig,
        migration.MigrationModeCompatibility,
    )
    
    // Create compatibility processor
    compatProcessor := migration.NewBackwardCompatibilityProcessor(
        queueProcessor,
        newGatewayManager,
        migrationManager,
    )
    
    // ... rest of application startup ...
}
```

2. **Add Health Checks**

```go
// Add endpoint to monitor migration status
http.HandleFunc("/api/migration/status", func(w http.ResponseWriter, r *http.Request) {
    status := migrationManager.GetMigrationStatus()
    json.NewEncoder(w).Encode(status)
})
```

### Phase 3: Gradual Feature Testing

1. **Test New Gateway Features**

```bash
# Send test emails through new system
curl -X POST http://localhost:8080/api/test/send \
  -H "Content-Type: application/json" \
  -d '{
    "from": "test@yourdomain.com",
    "to": ["recipient@example.com"],
    "subject": "Test via New Gateway System",
    "text": "This is a test email sent through the new gateway system."
  }'
```

2. **Monitor Metrics**

```bash
# Check gateway metrics
curl http://localhost:8080/api/gateways/metrics

# Check migration status
curl http://localhost:8080/api/migration/status
```

### Phase 4: Production Migration

1. **Enable Gradual Migration**

```go
// Update migration mode
migrationManager.SetMigrationMode(migration.MigrationModeGradual)

// Mark specific domains as migrated
migrationManager.MarkDomainMigrated("newdomain.com")
```

2. **Monitor and Validate**

- Watch error rates and delivery success
- Monitor rate limiting behavior
- Check recipient tracking accuracy
- Validate webhook functionality

### Phase 5: Complete Migration

1. **Switch to New System**

```go
// Final switch to new system only
migrationManager.SetMigrationMode(migration.MigrationModeImmediate)
```

2. **Clean Up Legacy Code**

- Remove legacy Gmail client references
- Update configuration files
- Remove compatibility layers
- Update documentation

## Configuration Examples

### Legacy workspace.json (Current)

```json
[
  {
    "id": "joinmednet",
    "domain": "joinmednet.org",
    "service_account_file": "credentials/joinmednet-service-account.json",
    "display_name": "Mednet Primary",
    "rate_limits": {
      "workspace_daily": 10000,
      "per_user_daily": 1000
    }
  }
]
```

### New workspaces-gateway.json

```json
{
  "gateways": [
    {
      "id": "joinmednet-workspace",
      "type": "google_workspace",
      "display_name": "Mednet Primary Workspace",
      "domain": "joinmednet.org",
      "enabled": true,
      "priority": 1,
      "google_workspace": {
        "service_account_file": "credentials/joinmednet-service-account.json",
        "impersonation_enabled": true
      },
      "rate_limits": {
        "workspace_daily": 10000,
        "per_user_daily": 1000
      },
      "routing": {
        "can_route": ["@joinmednet.org"],
        "failover_to": ["mailgun-primary"]
      }
    },
    {
      "id": "mailgun-primary",
      "type": "mailgun",
      "display_name": "Mailgun Primary Gateway",
      "domain": "mg.joinmednet.org",
      "enabled": true,
      "priority": 2,
      "mailgun": {
        "api_key": "key-your-mailgun-api-key",
        "domain": "mg.joinmednet.org",
        "tracking": {
          "clicks": true,
          "opens": true
        }
      },
      "rate_limits": {
        "workspace_daily": 50000,
        "per_user_daily": 5000
      }
    }
  ]
}
```

## Database Updates

The existing database schema supports the new gateway system without changes. However, you may want to add gateway tracking:

```sql
-- Optional: Add gateway tracking to messages table
ALTER TABLE messages ADD COLUMN gateway_id VARCHAR(255);
ALTER TABLE messages ADD COLUMN gateway_type VARCHAR(50);
ALTER TABLE messages ADD INDEX idx_gateway_id (gateway_id);

-- Optional: Add gateway performance tracking
CREATE TABLE IF NOT EXISTS gateway_metrics (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    gateway_id VARCHAR(255) NOT NULL,
    gateway_type VARCHAR(50) NOT NULL,
    total_sent INT NOT NULL DEFAULT 0,
    total_failed INT NOT NULL DEFAULT 0,
    success_rate DECIMAL(5,2) NOT NULL DEFAULT 100.00,
    avg_latency_ms INT NOT NULL DEFAULT 0,
    recorded_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_gateway_metrics_gateway (gateway_id),
    INDEX idx_gateway_metrics_recorded (recorded_at)
);
```

## Monitoring and Alerting

### Key Metrics to Monitor

1. **Gateway Health**
   - Circuit breaker states
   - Health check success rates
   - Error rates per gateway

2. **Performance**
   - Average latency per gateway
   - Success rates
   - Rate limit utilization

3. **Migration Progress**
   - Messages processed by each system
   - Domain migration status
   - Error rates during migration

### Recommended Alerts

```yaml
# Example New Relic alert conditions
- name: "Gateway Circuit Breaker Open"
  condition: "circuit_breaker_state = 'open'"
  
- name: "High Gateway Error Rate"
  condition: "error_rate > 5%"
  
- name: "Migration System Error Rate"
  condition: "migration_error_rate > 1%"
```

## Rollback Procedures

### Emergency Rollback

1. **Immediate Rollback to Legacy**

```go
// Set migration mode to legacy only
migrationManager.SetMigrationMode(migration.MigrationModeLegacyOnly)
```

2. **Configuration Rollback**

```bash
# Restore original configuration
cp workspace-legacy-backup.json workspace.json

# Restart application
systemctl restart relay
```

### Partial Rollback

```go
// Rollback specific domains
migrationManager.MarkDomainLegacy("problematic-domain.com")
```

## Testing Checklist

### Pre-Migration Testing

- [ ] All existing functionality works with compatibility layer
- [ ] Rate limiting behaves correctly
- [ ] Recipient tracking continues to work
- [ ] Webhooks are delivered properly
- [ ] Performance is acceptable

### Post-Migration Testing

- [ ] New gateway features work correctly
- [ ] Failover mechanisms activate properly
- [ ] Circuit breakers protect against failures
- [ ] Monitoring and alerting are functioning
- [ ] All domains can send emails

## Troubleshooting

### Common Issues

1. **Configuration Loading Errors**
   - Verify JSON syntax in gateway configuration
   - Check file permissions and paths
   - Validate gateway-specific configuration

2. **Authentication Failures**
   - Verify Google Workspace service accounts
   - Check Mailgun API keys
   - Validate domain configurations

3. **Rate Limiting Issues**
   - Check rate limit inheritance
   - Verify per-user limits
   - Monitor burst limit usage

4. **Routing Problems**
   - Validate routing patterns
   - Check domain matching logic
   - Verify failover configuration

### Debug Commands

```bash
# Check gateway status
curl http://localhost:8080/api/gateways/status

# View rate limiting status
curl http://localhost:8080/api/ratelimits/status

# Check circuit breaker states
curl http://localhost:8080/api/circuit-breakers/status

# Migration status
curl http://localhost:8080/api/migration/status
```

## Support and Resources

- **Documentation**: [Internal Wiki Link]
- **Monitoring**: New Relic Dashboard
- **Logs**: Check application logs for detailed error information
- **Support**: Contact the email infrastructure team

Remember: This migration can be done gradually and safely with proper testing and monitoring. Take time to validate each phase before proceeding to the next.