# Recipient Tracking System

## Overview

The Recipient Tracking System provides comprehensive email recipient management, engagement tracking, and analytics for the SMTP relay service. It tracks all recipients across campaigns, monitors delivery status, handles bounces, and provides detailed engagement metrics.

## Architecture

### Database Design

The system uses four main tables:

1. **`recipients`** - Core recipient information
2. **`message_recipients`** - Junction table linking messages to recipients
3. **`recipient_events`** - Detailed engagement tracking (opens, clicks, etc.)
4. **`recipient_lists`** + **`recipient_list_members`** - List management

### Key Features

- **Automatic Recipient Extraction**: Recipients are automatically extracted from messages during processing
- **Delivery Status Tracking**: Tracks PENDING → SENT → BOUNCED/FAILED status transitions
- **Bounce Management**: Automatic bounce detection and recipient status updates
- **Engagement Tracking**: Opens, clicks, unsubscribes, complaints via webhooks and pixels
- **Analytics**: Campaign-level and recipient-level statistics
- **List Management**: Organized recipient lists for campaign management
- **Data Retention**: Configurable cleanup of inactive recipients

## Integration Points

### 1. Message Processing Pipeline

Recipients are processed in the `QueueProcessor.Process()` method:

```go
// Process recipient information for this message
if p.recipientService != nil {
    if err := p.recipientService.ProcessMessageRecipients(msg); err != nil {
        log.Printf("Warning: Failed to process recipients for message %s: %v", msg.ID, err)
        // Continue processing the message even if recipient tracking fails
    }
}
```

### 2. Delivery Status Updates

Delivery status is updated in the `processMessage()` method based on Gmail API responses:

```go
// Update recipient delivery status to SENT
p.updateRecipientDeliveryStatus(msg, models.DeliveryStatusSent, "")
```

### 3. Webhook Integration

Engagement events are captured via webhook handlers:

```go
webhookHandler := recipient.NewWebhookHandler(recipientService)
http.HandleFunc("/webhook/mandrill", webhookHandler.HandleMandrillWebhook)
http.HandleFunc("/track/pixel", webhookHandler.HandlePixelTracking)
http.HandleFunc("/track/click", webhookHandler.HandleLinkTracking)
http.HandleFunc("/track/unsubscribe", webhookHandler.HandleUnsubscribe)
```

## API Endpoints

### Recipient Management

- `GET /api/recipients/{email}?workspace_id=xxx` - Get recipient details
- `GET /api/recipients/{email}/summary?workspace_id=xxx` - Get recipient summary with stats
- `PUT /api/recipients/{email}/status` - Update recipient status
- `GET /api/recipients?workspace_id=xxx&status=xxx&limit=50&offset=0` - List recipients
- `POST /api/recipients/cleanup` - Cleanup inactive recipients

### Campaign Analytics

- `GET /api/campaigns/{campaign_id}/stats?workspace_id=xxx` - Get campaign statistics

### Engagement Tracking

- `POST /webhook/mandrill` - Mandrill-compatible webhook events
- `GET /track/pixel?mid=xxx&email=xxx` - Email open tracking pixel
- `GET /track/click?mid=xxx&email=xxx&url=xxx` - Link click tracking with redirect
- `GET /track/unsubscribe?mid=xxx&email=xxx` - Unsubscribe page

## Data Flow

### 1. Message Processing

```
1. Message dequeued from queue
2. Recipients extracted from TO/CC/BCC fields
3. Recipient records created/updated in `recipients` table
4. Message-recipient relationships created in `message_recipients` table
5. Message sent via Gmail API
6. Delivery status updated based on send result
```

### 2. Engagement Tracking

```
1. Email opened → Pixel request → Open event recorded
2. Link clicked → Click tracking → Redirect + Click event recorded
3. Unsubscribe → Form submission → Unsubscribe event + Status update
4. Bounce notification → Webhook → Bounce event + Status update
```

### 3. Analytics Generation

```
1. Real-time stats computed via SQL aggregations
2. Campaign-level metrics: open rate, click rate, bounce rate
3. Recipient-level metrics: total messages, engagement rate
4. Historical tracking via timestamp fields
```

## Database Schema Details

### Recipients Table

```sql
CREATE TABLE recipients (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    email_address VARCHAR(320) NOT NULL,                        -- RFC 5321 max
    workspace_id VARCHAR(255) NOT NULL,
    user_id VARCHAR(255) NULL,                                   -- Optional
    campaign_id VARCHAR(255) NULL,                               -- Optional
    first_name VARCHAR(100) NULL,
    last_name VARCHAR(100) NULL,
    status ENUM('ACTIVE', 'INACTIVE', 'BOUNCED', 'UNSUBSCRIBED'),
    opt_in_date TIMESTAMP NULL,
    opt_out_date TIMESTAMP NULL,
    bounce_count INT NOT NULL DEFAULT 0,
    last_bounce_date TIMESTAMP NULL,
    bounce_type ENUM('SOFT', 'HARD') NULL,
    metadata JSON,                                               -- Extensible data
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    
    UNIQUE KEY uk_email_workspace (email_address, workspace_id),
    INDEX idx_workspace_status (workspace_id, status),
    INDEX idx_campaign_id (campaign_id),
    INDEX idx_bounce_tracking (status, bounce_count, last_bounce_date)
);
```

### Key Indexes for Performance

- **`uk_email_workspace`**: Prevents duplicate recipients per workspace
- **`idx_workspace_status`**: Fast filtering by workspace and status
- **`idx_campaign_id`**: Campaign-based queries
- **`idx_bounce_tracking`**: Bounce management queries

## Performance Considerations

### Database Optimization

1. **Composite Indexes**: Multi-column indexes for common query patterns
2. **JSON Metadata**: Flexible storage without schema changes
3. **Partitioning Ready**: Tables designed for date-based partitioning
4. **Connection Pooling**: Uses MySQL connection pooling

### Query Patterns

```sql
-- Get active recipients in workspace
SELECT * FROM recipients 
WHERE workspace_id = ? AND status = 'ACTIVE';

-- Get campaign stats
SELECT COUNT(*) as sent, 
       SUM(opens) as total_opens,
       SUM(clicks) as total_clicks
FROM message_recipients mr
JOIN recipients r ON mr.recipient_id = r.id
WHERE r.workspace_id = ? AND mr.delivery_status = 'SENT';
```

### Error Handling

- **Defensive Programming**: Continue message processing even if recipient tracking fails
- **Transaction Safety**: Uses database transactions for consistency
- **Graceful Degradation**: System functions without recipient service
- **Retry Logic**: Failed recipient operations logged but don't block email sending

## Configuration

### Environment Variables

```bash
# MySQL configuration for recipient tracking
MYSQL_HOST=localhost
MYSQL_PORT=3306
MYSQL_USER=relay
MYSQL_PASSWORD=password
MYSQL_DATABASE=relay

# Webhook endpoints for engagement tracking
WEBHOOK_BASE_URL=https://relay.example.com
```

### Application Integration

```go
// Initialize recipient service
recipientService := recipient.NewService(db)

// Add to processor
processor := NewQueueProcessor(
    queue,
    config,
    gmailClient,
    webhookClient,
    personalizer,
    recipientService,  // Add recipient service
)

// Set up webhook handlers
webhookHandler := recipient.NewWebhookHandler(recipientService)
apiHandler := recipient.NewAPIHandler(recipientService)
```

## Data Migration

### Migrating Existing Data

Run the migration script to populate recipient tables from existing messages:

```bash
mysql -u relay -p relay < migrations/001_recipient_tracking.sql
```

The migration script:
1. Creates all necessary tables and indexes
2. Extracts recipients from existing message data
3. Creates recipient records with appropriate status
4. Establishes message-recipient relationships
5. Updates bounce counts based on message history

### Migration Verification

```sql
-- Check migration results
SELECT 
    COUNT(*) as total_recipients,
    COUNT(CASE WHEN status = 'ACTIVE' THEN 1 END) as active,
    COUNT(CASE WHEN status = 'BOUNCED' THEN 1 END) as bounced
FROM recipients;
```

## Monitoring & Maintenance

### Health Checks

```sql
-- Check for orphaned message recipients
SELECT COUNT(*) FROM message_recipients mr
LEFT JOIN messages m ON mr.message_id = m.id
WHERE m.id IS NULL;

-- Check bounce rate trends
SELECT DATE(created_at) as date,
       COUNT(*) as total,
       COUNT(CASE WHEN delivery_status = 'BOUNCED' THEN 1 END) as bounces
FROM message_recipients
WHERE created_at >= DATE_SUB(NOW(), INTERVAL 7 DAY)
GROUP BY DATE(created_at);
```

### Cleanup Operations

```bash
# API call to cleanup inactive recipients (90 day retention)
curl -X POST http://localhost:8080/api/recipients/cleanup \
  -H "Content-Type: application/json" \
  -d '{"retention_days": 90}'
```

### Log Monitoring

Monitor application logs for:
- Recipient processing warnings
- Delivery status update failures
- Webhook processing errors
- Database connection issues

## Security Considerations

### Data Protection

- **Email Normalization**: All emails stored in lowercase
- **Input Validation**: Email format validation on ingestion
- **SQL Injection Protection**: Parameterized queries throughout
- **Access Control**: API endpoints should include authentication

### Privacy Compliance

- **Data Retention**: Configurable cleanup of old recipient data
- **Unsubscribe Handling**: Immediate status updates for opt-outs
- **Audit Trail**: All recipient status changes are timestamped
- **Metadata Encryption**: Consider encrypting sensitive metadata fields

## Troubleshooting

### Common Issues

1. **Recipients Not Created**: Check message processing logs for JSON parsing errors
2. **Missing Delivery Status**: Verify Gmail API response processing
3. **Engagement Not Tracked**: Check webhook endpoint configuration
4. **Performance Issues**: Review database indexes and query patterns

### Debug Queries

```sql
-- Find messages with no recipient records
SELECT m.id, m.workspace_id, m.to_emails 
FROM messages m
LEFT JOIN message_recipients mr ON m.id = mr.message_id
WHERE mr.id IS NULL AND m.status != 'queued';

-- Check for duplicate recipients
SELECT email_address, workspace_id, COUNT(*)
FROM recipients
GROUP BY email_address, workspace_id
HAVING COUNT(*) > 1;
```

## Future Enhancements

### Planned Features

1. **Advanced Segmentation**: Recipient grouping by engagement patterns
2. **Predictive Analytics**: Bounce prediction using ML models  
3. **Real-time Dashboards**: WebSocket-based live analytics
4. **A/B Testing**: Campaign variant tracking per recipient
5. **Suppression Lists**: Global suppression list management
6. **GDPR Compliance**: Right-to-be-forgotten implementation

### Scalability Improvements

1. **Read Replicas**: Separate analytics queries from transactional operations
2. **Sharding**: Partition recipients by workspace or date
3. **Caching Layer**: Redis cache for frequently accessed recipient data
4. **Event Streaming**: Kafka/Pulsar for high-volume engagement events