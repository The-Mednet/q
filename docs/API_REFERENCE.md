# SMTP Relay Service - API Reference

## Table of Contents

1. [SMTP Protocol Interface](#1-smtp-protocol-interface)
2. [REST API Endpoints](#2-rest-api-endpoints)
3. [Webhook Endpoints](#3-webhook-endpoints)
4. [Administrative API](#4-administrative-api)
5. [Provider-Specific APIs](#5-provider-specific-apis)
6. [Error Codes & Responses](#6-error-codes--responses)

---

## 1. SMTP Protocol Interface

### 1.1 Connection Information

```
Host: smtp.mednet.com
Port: 2525 (non-TLS), 2526 (TLS)
Authentication: Optional (recommended for production)
Max Message Size: 25MB
Timeout: 300 seconds
```

### 1.2 SMTP Commands

#### EHLO/HELO - Initiate Session

```smtp
EHLO client.example.com
250-smtp.mednet.com Hello client.example.com
250-SIZE 26214400
250-8BITMIME
250-PIPELINING
250-STARTTLS
250 DSN
```

#### MAIL FROM - Specify Sender

```smtp
MAIL FROM:<sender@example.com> SIZE=1024
250 2.1.0 Ok
```

**Parameters:**
- `SIZE`: Optional message size in bytes
- `BODY`: Optional encoding (7BIT, 8BITMIME)

#### RCPT TO - Specify Recipients

```smtp
RCPT TO:<recipient@example.com>
250 2.1.5 Ok
```

**Multiple Recipients:**
```smtp
RCPT TO:<recipient1@example.com>
250 2.1.5 Ok
RCPT TO:<recipient2@example.com>
250 2.1.5 Ok
```

#### DATA - Send Message Content

```smtp
DATA
354 End data with <CR><LF>.<CR><LF>
From: sender@example.com
To: recipient@example.com
Subject: Test Message
Date: Mon, 15 Jan 2024 10:00:00 -0500
Message-ID: <unique-id@example.com>
Content-Type: text/plain; charset=UTF-8

This is the message body.
.
250 2.0.0 Ok: queued as 12345-67890-ABCDE
```

### 1.3 Extended Headers

#### Workspace Routing

```smtp
X-Workspace-ID: workspace-123
X-Campaign-ID: CAMP-2024-001
X-User-ID: USER-456
```

#### Priority & Metadata

```smtp
X-Priority: high
X-Retry-Count: 3
X-Metadata: {"source":"api","version":"2.0"}
```

#### Personalization

```smtp
X-Enable-Personalization: true
X-LLM-Model: gpt-4
X-Personalization-Context: {"tone":"professional","industry":"healthcare"}
```

### 1.4 SMTP Response Codes

| Code | Meaning | Description |
|------|---------|-------------|
| 220 | Service ready | SMTP service is ready |
| 221 | Closing connection | Goodbye message |
| 250 | Requested action completed | Success |
| 251 | User not local, will forward | Message will be relayed |
| 354 | Start mail input | Ready to receive message data |
| 421 | Service not available | Temporary failure |
| 450 | Mailbox unavailable | Temporary failure |
| 451 | Aborted - local error | Processing error |
| 452 | Insufficient storage | Over quota |
| 500 | Syntax error | Invalid command |
| 501 | Syntax error in parameters | Invalid arguments |
| 502 | Command not implemented | Unsupported command |
| 503 | Bad command sequence | Commands out of order |
| 550 | Mailbox unavailable | Permanent failure |
| 551 | User not local | Invalid recipient |
| 552 | Storage allocation exceeded | Message too large |
| 553 | Mailbox name not allowed | Invalid address format |
| 554 | Transaction failed | General failure |

---

## 2. REST API Endpoints

### 2.1 Message Queue Operations

#### GET /api/messages - List Messages

**Request:**
```http
GET /api/messages?status=queued&limit=50&offset=0
Authorization: Bearer <token>
```

**Query Parameters:**
| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| status | string | all | Filter by status (queued, processing, sent, failed) |
| limit | integer | 50 | Number of results per page |
| offset | integer | 0 | Pagination offset |
| workspace_id | string | - | Filter by workspace |
| from | string | - | Filter by sender email |
| to | string | - | Filter by recipient email |
| date_from | ISO8601 | - | Start date filter |
| date_to | ISO8601 | - | End date filter |

**Response:**
```json
{
  "success": true,
  "data": {
    "messages": [
      {
        "id": "msg-123456",
        "from": "sender@example.com",
        "to": ["recipient@example.com"],
        "cc": [],
        "bcc": [],
        "subject": "Test Email",
        "status": "queued",
        "workspace_id": "ws-001",
        "campaign_id": "camp-123",
        "queued_at": "2024-01-15T10:00:00Z",
        "processed_at": null,
        "error": null,
        "retry_count": 0,
        "metadata": {
          "source": "api",
          "user_id": "user-456"
        }
      }
    ],
    "pagination": {
      "total": 150,
      "limit": 50,
      "offset": 0,
      "has_more": true
    }
  }
}
```

#### GET /api/messages/{id} - Get Message Details

**Request:**
```http
GET /api/messages/msg-123456
Authorization: Bearer <token>
```

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "msg-123456",
    "from": "sender@example.com",
    "to": ["recipient@example.com"],
    "cc": ["cc@example.com"],
    "bcc": ["bcc@example.com"],
    "subject": "Test Email",
    "html_body": "<html>...</html>",
    "text_body": "Plain text content...",
    "headers": {
      "Message-ID": "<unique@example.com>",
      "Date": "Mon, 15 Jan 2024 10:00:00 -0500"
    },
    "attachments": [
      {
        "filename": "document.pdf",
        "content_type": "application/pdf",
        "size": 102400
      }
    ],
    "status": "sent",
    "workspace_id": "ws-001",
    "campaign_id": "camp-123",
    "user_id": "user-456",
    "queued_at": "2024-01-15T10:00:00Z",
    "processed_at": "2024-01-15T10:00:30Z",
    "provider": "gmail",
    "provider_message_id": "gmail-msg-789",
    "error": null,
    "retry_count": 0,
    "metadata": {
      "source": "api",
      "version": "2.0"
    }
  }
}
```

#### POST /api/messages - Send Email

**Request:**
```http
POST /api/messages
Authorization: Bearer <token>
Content-Type: application/json

{
  "from": "sender@example.com",
  "to": ["recipient@example.com"],
  "cc": ["cc@example.com"],
  "bcc": ["bcc@example.com"],
  "subject": "Important Message",
  "html_body": "<h1>Hello</h1><p>This is a test email</p>",
  "text_body": "Hello\n\nThis is a test email",
  "attachments": [
    {
      "filename": "report.pdf",
      "content_type": "application/pdf",
      "content": "base64_encoded_content_here"
    }
  ],
  "headers": {
    "Reply-To": "noreply@example.com",
    "X-Priority": "high"
  },
  "workspace_id": "ws-001",
  "campaign_id": "camp-123",
  "metadata": {
    "user_id": "user-456",
    "source": "api"
  },
  "personalization": {
    "enabled": true,
    "variables": {
      "first_name": "John",
      "last_name": "Doe",
      "custom_field": "value"
    }
  },
  "scheduling": {
    "send_at": "2024-01-15T14:00:00Z",
    "timezone": "America/New_York"
  }
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "message_id": "msg-789012",
    "status": "queued",
    "queued_at": "2024-01-15T10:00:00Z",
    "estimated_send_time": "2024-01-15T14:00:00Z"
  }
}
```

#### DELETE /api/messages/{id} - Cancel Message

**Request:**
```http
DELETE /api/messages/msg-123456
Authorization: Bearer <token>
```

**Response:**
```json
{
  "success": true,
  "message": "Message cancelled successfully"
}
```

**Note:** Only messages with status `queued` can be cancelled.

#### PUT /api/messages/{id}/retry - Retry Failed Message

**Request:**
```http
PUT /api/messages/msg-123456/retry
Authorization: Bearer <token>
```

**Response:**
```json
{
  "success": true,
  "data": {
    "message_id": "msg-123456",
    "status": "queued",
    "retry_count": 1
  }
}
```

### 2.2 Statistics & Monitoring

#### GET /api/stats - System Statistics

**Request:**
```http
GET /api/stats
Authorization: Bearer <token>
```

**Response:**
```json
{
  "success": true,
  "data": {
    "system": {
      "version": "2.0.0",
      "uptime_seconds": 86400,
      "current_time": "2024-01-15T10:00:00Z"
    },
    "queue": {
      "total": 10000,
      "queued": 150,
      "processing": 10,
      "sent": 9500,
      "failed": 340,
      "auth_error": 0
    },
    "processing": {
      "rate_per_minute": 120,
      "avg_processing_time_ms": 450,
      "p95_processing_time_ms": 800,
      "p99_processing_time_ms": 1200
    },
    "providers": {
      "gmail": {
        "status": "healthy",
        "success_rate": 0.98,
        "avg_latency_ms": 200
      },
      "mailgun": {
        "status": "healthy",
        "success_rate": 0.99,
        "avg_latency_ms": 150
      }
    },
    "rate_limits": {
      "global": {
        "used_today": 45000,
        "limit_today": 100000,
        "reset_at": "2024-01-16T00:00:00Z"
      }
    }
  }
}
```

#### GET /api/stats/realtime - Real-time Metrics

**Request:**
```http
GET /api/stats/realtime
Authorization: Bearer <token>
```

**Response (Server-Sent Events):**
```
event: metrics
data: {"timestamp":"2024-01-15T10:00:00Z","emails_per_second":5,"queue_depth":150,"active_processors":3}

event: metrics
data: {"timestamp":"2024-01-15T10:00:01Z","emails_per_second":6,"queue_depth":148,"active_processors":3}
```

### 2.3 Rate Limiting

#### GET /api/rate-limit - Rate Limit Status

**Request:**
```http
GET /api/rate-limit?workspace_id=ws-001
Authorization: Bearer <token>
```

**Response:**
```json
{
  "success": true,
  "data": {
    "global": {
      "limit": 100000,
      "remaining": 55000,
      "reset_at": "2024-01-16T00:00:00Z"
    },
    "workspace": {
      "workspace_id": "ws-001",
      "limit": 10000,
      "used": 4500,
      "remaining": 5500,
      "reset_at": "2024-01-16T00:00:00Z"
    },
    "users": {
      "user-123": {
        "limit": 500,
        "used": 150,
        "remaining": 350
      },
      "user-456": {
        "limit": 1000,
        "used": 800,
        "remaining": 200
      }
    }
  }
}
```

### 2.4 Recipient Management

#### GET /api/recipients - List Recipients

**Request:**
```http
GET /api/recipients?workspace_id=ws-001&status=ACTIVE&limit=20
Authorization: Bearer <token>
```

**Response:**
```json
{
  "success": true,
  "data": {
    "recipients": [
      {
        "id": 12345,
        "email": "john.doe@example.com",
        "workspace_id": "ws-001",
        "first_name": "John",
        "last_name": "Doe",
        "status": "ACTIVE",
        "bounce_count": 0,
        "created_at": "2024-01-01T00:00:00Z",
        "updated_at": "2024-01-15T10:00:00Z",
        "metadata": {
          "source": "signup",
          "tags": ["newsletter", "updates"]
        }
      }
    ],
    "pagination": {
      "total": 5000,
      "limit": 20,
      "offset": 0
    }
  }
}
```

#### GET /api/recipients/{email} - Get Recipient Details

**Request:**
```http
GET /api/recipients/john.doe@example.com
Authorization: Bearer <token>
```

**Response:**
```json
{
  "success": true,
  "data": {
    "id": 12345,
    "email": "john.doe@example.com",
    "workspace_id": "ws-001",
    "user_id": "user-789",
    "campaign_id": null,
    "first_name": "John",
    "last_name": "Doe",
    "status": "ACTIVE",
    "opt_in_date": "2024-01-01T00:00:00Z",
    "opt_out_date": null,
    "bounce_count": 0,
    "last_bounce_date": null,
    "bounce_type": null,
    "engagement": {
      "total_received": 50,
      "total_opened": 35,
      "total_clicked": 12,
      "last_open_date": "2024-01-14T15:30:00Z",
      "last_click_date": "2024-01-14T15:31:00Z"
    },
    "metadata": {
      "source": "signup",
      "tags": ["newsletter", "updates"],
      "preferences": {
        "frequency": "weekly",
        "topics": ["medical", "research"]
      }
    },
    "created_at": "2024-01-01T00:00:00Z",
    "updated_at": "2024-01-15T10:00:00Z"
  }
}
```

#### POST /api/recipients - Add Recipient

**Request:**
```http
POST /api/recipients
Authorization: Bearer <token>
Content-Type: application/json

{
  "email": "new.user@example.com",
  "workspace_id": "ws-001",
  "first_name": "New",
  "last_name": "User",
  "opt_in": true,
  "metadata": {
    "source": "api",
    "tags": ["newsletter"]
  }
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "id": 12346,
    "email": "new.user@example.com",
    "status": "ACTIVE",
    "created_at": "2024-01-15T10:00:00Z"
  }
}
```

#### PUT /api/recipients/{email} - Update Recipient

**Request:**
```http
PUT /api/recipients/john.doe@example.com
Authorization: Bearer <token>
Content-Type: application/json

{
  "first_name": "Jonathan",
  "status": "ACTIVE",
  "metadata": {
    "tags": ["newsletter", "premium"],
    "preferences": {
      "frequency": "daily"
    }
  }
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "id": 12345,
    "email": "john.doe@example.com",
    "first_name": "Jonathan",
    "status": "ACTIVE",
    "updated_at": "2024-01-15T10:05:00Z"
  }
}
```

#### DELETE /api/recipients/{email} - Remove Recipient

**Request:**
```http
DELETE /api/recipients/john.doe@example.com
Authorization: Bearer <token>
```

**Response:**
```json
{
  "success": true,
  "message": "Recipient removed successfully"
}
```

### 2.5 Campaign Management

#### GET /api/campaigns/{id}/stats - Campaign Statistics

**Request:**
```http
GET /api/campaigns/camp-123/stats
Authorization: Bearer <token>
```

**Response:**
```json
{
  "success": true,
  "data": {
    "campaign_id": "camp-123",
    "name": "January Newsletter",
    "status": "completed",
    "stats": {
      "total_recipients": 5000,
      "sent": 4950,
      "delivered": 4900,
      "bounced": 50,
      "opened": 2450,
      "clicked": 980,
      "unsubscribed": 25,
      "complained": 2
    },
    "rates": {
      "delivery_rate": 0.98,
      "open_rate": 0.50,
      "click_rate": 0.20,
      "bounce_rate": 0.01,
      "unsubscribe_rate": 0.005,
      "complaint_rate": 0.0004
    },
    "timeline": {
      "created_at": "2024-01-10T00:00:00Z",
      "started_at": "2024-01-10T10:00:00Z",
      "completed_at": "2024-01-10T11:30:00Z"
    }
  }
}
```

---

## 3. Webhook Endpoints

### 3.1 Mandrill-Compatible Webhooks

#### POST /webhook/mandrill - Receive Mandrill Events

**Incoming Webhook Format:**
```json
{
  "mandrill_events": [
    {
      "event": "send",
      "_id": "msg-123456",
      "msg": {
        "_id": "msg-123456",
        "ts": 1705320000,
        "state": "sent",
        "subject": "Test Email",
        "email": "recipient@example.com",
        "sender": "sender@example.com",
        "tags": ["newsletter"],
        "metadata": {
          "user_id": "user-456",
          "campaign_id": "camp-123"
        }
      },
      "ts": 1705320000
    }
  ]
}
```

**Event Types:**
- `send` - Message sent successfully
- `bounce` - Message bounced
- `soft_bounce` - Temporary failure
- `hard_bounce` - Permanent failure
- `open` - Message opened
- `click` - Link clicked
- `spam` - Marked as spam
- `unsub` - Unsubscribed
- `reject` - Message rejected

### 3.2 Engagement Tracking

#### GET /webhook/pixel - Email Open Tracking

**Request:**
```http
GET /webhook/pixel?mid=msg-123456&rid=12345
```

**Response:**
- Returns 1x1 transparent GIF
- Records open event

#### GET /webhook/click - Link Click Tracking

**Request:**
```http
GET /webhook/click?mid=msg-123456&rid=12345&url=https%3A%2F%2Fexample.com
```

**Response:**
- 302 redirect to target URL
- Records click event

#### POST /webhook/unsubscribe - Handle Unsubscribe

**Request:**
```http
POST /webhook/unsubscribe
Content-Type: application/x-www-form-urlencoded

email=john.doe@example.com&campaign_id=camp-123&reason=not_interested
```

**Response:**
```html
<html>
<body>
  <h1>Unsubscribed Successfully</h1>
  <p>You have been unsubscribed from our mailing list.</p>
</body>
</html>
```

---

## 4. Administrative API

### 4.1 Provider Management

#### GET /api/admin/providers - List Providers

**Request:**
```http
GET /api/admin/providers
Authorization: Bearer <admin-token>
```

**Response:**
```json
{
  "success": true,
  "data": {
    "providers": [
      {
        "id": "gmail-ws-001",
        "type": "gmail",
        "workspace_id": "ws-001",
        "status": "healthy",
        "enabled": true,
        "priority": 1,
        "weight": 70,
        "config": {
          "service_account": "sa-123@project.iam.gserviceaccount.com",
          "domain": "example.com"
        },
        "metrics": {
          "total_sent": 10000,
          "success_rate": 0.98,
          "avg_latency_ms": 200
        },
        "last_health_check": "2024-01-15T10:00:00Z"
      }
    ]
  }
}
```

#### POST /api/admin/providers/{id}/health-check - Force Health Check

**Request:**
```http
POST /api/admin/providers/gmail-ws-001/health-check
Authorization: Bearer <admin-token>
```

**Response:**
```json
{
  "success": true,
  "data": {
    "provider_id": "gmail-ws-001",
    "status": "healthy",
    "latency_ms": 150,
    "checked_at": "2024-01-15T10:00:00Z"
  }
}
```

### 4.2 Configuration Management

#### GET /api/admin/config - Get Configuration

**Request:**
```http
GET /api/admin/config
Authorization: Bearer <admin-token>
```

**Response:**
```json
{
  "success": true,
  "data": {
    "smtp": {
      "host": "0.0.0.0",
      "port": 2525,
      "max_size": 26214400
    },
    "queue": {
      "batch_size": 50,
      "process_interval": "30s",
      "max_retries": 3
    },
    "rate_limits": {
      "global_daily": 100000,
      "workspace_default": 10000,
      "user_default": 500
    },
    "providers": {
      "gmail": {
        "enabled": true,
        "rate_limit": 2000
      },
      "mailgun": {
        "enabled": true,
        "rate_limit": 100000
      }
    }
  }
}
```

#### PUT /api/admin/config - Update Configuration

**Request:**
```http
PUT /api/admin/config
Authorization: Bearer <admin-token>
Content-Type: application/json

{
  "queue": {
    "batch_size": 100,
    "process_interval": "15s"
  },
  "rate_limits": {
    "global_daily": 150000
  }
}
```

**Response:**
```json
{
  "success": true,
  "message": "Configuration updated successfully",
  "restart_required": false
}
```

### 4.3 Maintenance Operations

#### POST /api/admin/maintenance/mode - Toggle Maintenance Mode

**Request:**
```http
POST /api/admin/maintenance/mode
Authorization: Bearer <admin-token>
Content-Type: application/json

{
  "enabled": true,
  "message": "Scheduled maintenance in progress",
  "expected_duration_minutes": 30
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "maintenance_mode": true,
    "started_at": "2024-01-15T10:00:00Z",
    "expected_end": "2024-01-15T10:30:00Z"
  }
}
```

#### POST /api/admin/maintenance/cleanup - Database Cleanup

**Request:**
```http
POST /api/admin/maintenance/cleanup
Authorization: Bearer <admin-token>
Content-Type: application/json

{
  "older_than_days": 90,
  "include_sent": true,
  "include_failed": true,
  "dry_run": false
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "deleted": {
      "messages": 15000,
      "webhook_events": 45000,
      "recipient_events": 120000
    },
    "space_freed_mb": 2048
  }
}
```

---

## 5. Provider-Specific APIs

### 5.1 Gmail Provider

#### POST /api/providers/gmail/validate - Validate Service Account

**Request:**
```http
POST /api/providers/gmail/validate
Authorization: Bearer <token>
Content-Type: application/json

{
  "workspace_id": "ws-001",
  "service_account_json": "{...}",
  "test_email": "test@example.com"
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "valid": true,
    "service_account": "sa-123@project.iam.gserviceaccount.com",
    "scopes": [
      "https://www.googleapis.com/auth/gmail.send",
      "https://www.googleapis.com/auth/gmail.readonly"
    ],
    "domain_wide_delegation": true
  }
}
```

### 5.2 Mailgun Provider

#### POST /api/providers/mailgun/validate - Validate API Key

**Request:**
```http
POST /api/providers/mailgun/validate
Authorization: Bearer <token>
Content-Type: application/json

{
  "api_key": "key-abc123def456",
  "domain": "mg.example.com",
  "region": "US"
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "valid": true,
    "domain_verified": true,
    "dns_records": {
      "mx": true,
      "txt": true,
      "cname": true
    },
    "sending_enabled": true
  }
}
```

#### GET /api/providers/mailgun/domains - List Mailgun Domains

**Request:**
```http
GET /api/providers/mailgun/domains
Authorization: Bearer <token>
```

**Response:**
```json
{
  "success": true,
  "data": {
    "domains": [
      {
        "name": "mg.example.com",
        "state": "active",
        "type": "custom",
        "created_at": "2024-01-01T00:00:00Z",
        "smtp_login": "postmaster@mg.example.com",
        "is_disabled": false,
        "skip_verification": false
      }
    ]
  }
}
```

---

## 6. Error Codes & Responses

### 6.1 Standard Error Response Format

```json
{
  "success": false,
  "error": {
    "code": "RATE_LIMIT_EXCEEDED",
    "message": "Daily rate limit exceeded for workspace ws-001",
    "details": {
      "limit": 10000,
      "used": 10000,
      "reset_at": "2024-01-16T00:00:00Z"
    },
    "request_id": "req-abc123",
    "timestamp": "2024-01-15T10:00:00Z"
  }
}
```

### 6.2 Error Codes

| Code | HTTP Status | Description |
|------|-------------|-------------|
| **Authentication & Authorization** |
| AUTH_REQUIRED | 401 | Authentication required |
| INVALID_TOKEN | 401 | Invalid or expired token |
| INSUFFICIENT_PERMISSIONS | 403 | Insufficient permissions |
| WORKSPACE_ACCESS_DENIED | 403 | No access to workspace |
| **Validation Errors** |
| INVALID_REQUEST | 400 | Invalid request format |
| MISSING_REQUIRED_FIELD | 400 | Required field missing |
| INVALID_EMAIL_FORMAT | 400 | Invalid email address format |
| MESSAGE_TOO_LARGE | 413 | Message exceeds size limit |
| **Rate Limiting** |
| RATE_LIMIT_EXCEEDED | 429 | Rate limit exceeded |
| QUOTA_EXCEEDED | 429 | Daily quota exceeded |
| **Resource Errors** |
| MESSAGE_NOT_FOUND | 404 | Message not found |
| RECIPIENT_NOT_FOUND | 404 | Recipient not found |
| WORKSPACE_NOT_FOUND | 404 | Workspace not found |
| **Processing Errors** |
| QUEUE_FULL | 503 | Queue is full |
| PROVIDER_ERROR | 502 | Provider API error |
| ALL_PROVIDERS_DOWN | 503 | All providers unavailable |
| **System Errors** |
| INTERNAL_ERROR | 500 | Internal server error |
| DATABASE_ERROR | 500 | Database error |
| MAINTENANCE_MODE | 503 | Service in maintenance mode |

### 6.3 Rate Limit Headers

All API responses include rate limit information:

```http
X-RateLimit-Limit: 1000
X-RateLimit-Remaining: 950
X-RateLimit-Reset: 1705363200
X-RateLimit-Reset-After: 3600
X-RateLimit-Workspace: ws-001
```

### 6.4 Pagination Headers

List endpoints include pagination headers:

```http
X-Total-Count: 5000
X-Page-Count: 100
X-Current-Page: 1
X-Per-Page: 50
Link: </api/messages?page=2>; rel="next", </api/messages?page=100>; rel="last"
```

---

## Authentication

### Bearer Token Authentication

```http
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

### API Key Authentication

```http
X-API-Key: sk_live_abc123def456ghi789
```

### Workspace Context

```http
X-Workspace-ID: ws-001
```

---

## Client Libraries

### Go Client Example

```go
package main

import (
    "github.com/mednet/smtp-relay-client-go"
)

func main() {
    client := smtprelay.NewClient(
        smtprelay.WithAPIKey("sk_live_..."),
        smtprelay.WithWorkspace("ws-001"),
    )
    
    msg := &smtprelay.Message{
        From:    "sender@example.com",
        To:      []string{"recipient@example.com"},
        Subject: "Test Email",
        HTML:    "<h1>Hello World</h1>",
    }
    
    resp, err := client.SendMessage(msg)
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("Message sent: %s\n", resp.MessageID)
}
```

### Python Client Example

```python
from smtp_relay import Client

client = Client(
    api_key="sk_live_...",
    workspace_id="ws-001"
)

response = client.send_message(
    from_email="sender@example.com",
    to=["recipient@example.com"],
    subject="Test Email",
    html="<h1>Hello World</h1>",
    text="Hello World"
)

print(f"Message sent: {response.message_id}")
```

### Node.js Client Example

```javascript
const SMTPRelay = require('@mednet/smtp-relay-client');

const client = new SMTPRelay({
  apiKey: 'sk_live_...',
  workspaceId: 'ws-001'
});

async function sendEmail() {
  const response = await client.sendMessage({
    from: 'sender@example.com',
    to: ['recipient@example.com'],
    subject: 'Test Email',
    html: '<h1>Hello World</h1>',
    text: 'Hello World'
  });
  
  console.log(`Message sent: ${response.messageId}`);
}

sendEmail();
```

---

## API Versioning

The API uses URL versioning. The current version is v1.

```
https://api.smtp.mednet.com/v1/messages
```

### Deprecation Policy

- APIs are supported for minimum 12 months after deprecation notice
- Deprecation notices sent via email and API headers
- Deprecated endpoints return `Sunset` header with deprecation date

```http
Sunset: Sat, 31 Dec 2024 23:59:59 GMT
Deprecation: true
Link: <https://docs.mednet.com/migration>; rel="deprecation"
```

---

## Rate Limits

### Default Limits

| Tier | Requests/Hour | Requests/Day | Burst |
|------|---------------|--------------|-------|
| Free | 100 | 1,000 | 10 |
| Basic | 1,000 | 10,000 | 50 |
| Pro | 10,000 | 100,000 | 100 |
| Enterprise | Custom | Custom | Custom |

### Endpoint-Specific Limits

| Endpoint | Limit | Window |
|----------|-------|--------|
| POST /api/messages | 100 | per minute |
| GET /api/messages | 1000 | per minute |
| GET /api/stats/realtime | 10 | per minute |
| POST /api/admin/* | 10 | per minute |

---

*API Reference Version: 1.0.0*  
*Last Updated: 2024-01-15*  
*API Base URL: https://api.smtp.mednet.com/v1*