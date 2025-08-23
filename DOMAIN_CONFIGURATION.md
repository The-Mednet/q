# Domain Configuration Guide

## Understanding Domains in the SMTP Relay Service

This service uses domains at two different levels:

### 1. Workspace Domains (Routing)
The `domains` field at the workspace level determines which sender domains this workspace handles.

### 2. Provider-Specific Domains (Sending)
Each provider may have its own domain configuration for actually sending emails.

## Configuration Structure

```json
{
  "id": "example-workspace",
  
  // WORKSPACE DOMAINS: Which sender addresses this workspace handles
  "domains": [
    "company.com",           // Main domain
    "www.company.com",       // Website variant
    "app.company.com",       // Application subdomain
    "notifications.company.com"  // Notification subdomain
  ],
  
  // PROVIDER CONFIGURATIONS
  "gmail": {
    "enabled": true,
    "service_account_env": "GMAIL_SERVICE_ACCOUNT",
    // Gmail uses the sender's actual domain via impersonation
  },
  
  "mailgun": {
    "enabled": true,
    "api_key": "${MAILGUN_API_KEY}",
    
    // MAILGUN SENDING DOMAIN: The actual Mailgun domain for API
    "sending_domain": "mg.company.com",  // This is your Mailgun domain
    
    "tracking": {
      "opens": true,
      "clicks": true
    }
  },
  
  "mandrill": {
    "enabled": true,
    "api_key": "${MANDRILL_API_KEY}",
    // Mandrill can send from any domain without verification
  }
}
```

## How It Works

### Email Routing Flow:
1. **Incoming Email**: `sender@app.company.com` sends an email
2. **Domain Extraction**: System extracts `app.company.com`
3. **Workspace Lookup**: Finds workspace that has `app.company.com` in its `domains` list
4. **Provider Selection**: Chooses an enabled provider from that workspace
5. **Email Sending**: 
   - **Gmail**: Sends as `sender@app.company.com` (impersonation)
   - **Mailgun**: Sends via `mg.company.com` API, but shows `sender@app.company.com` as From
   - **Mandrill**: Sends as `sender@app.company.com` directly

## Examples

### Multi-Brand Organization
```json
{
  "id": "multi-brand",
  "domains": [
    "brand1.com",
    "brand2.com",
    "brand3.com"
  ],
  "mailgun": {
    "sending_domain": "mg.our-company.com",  // Single Mailgun domain for all brands
    "enabled": true
  }
}
```

### Separate Environments
```json
{
  "id": "production",
  "domains": [
    "example.com",
    "www.example.com",
    "api.example.com"
  ],
  "mailgun": {
    "sending_domain": "mg.example.com",  // Production Mailgun domain
    "enabled": true
  }
},
{
  "id": "staging",
  "domains": [
    "staging.example.com",
    "test.example.com"
  ],
  "mailgun": {
    "sending_domain": "mg-staging.example.com",  // Staging Mailgun domain
    "enabled": true
  }
}
```

### Migration Strategy
```json
{
  "id": "migrating",
  "domains": [
    "oldcompany.com",
    "newcompany.com"
  ],
  "gmail": {
    "enabled": true  // Old system
  },
  "mandrill": {
    "enabled": true  // New system
  }
}
```

## Key Points

1. **Workspace `domains`**: Controls which sender addresses are accepted
2. **Mailgun `sending_domain`**: The actual Mailgun API domain (must be verified in Mailgun)
3. **Gmail**: Uses domain impersonation, no separate sending domain needed
4. **Mandrill**: Can send from any domain, no verification required

## Backward Compatibility

For backward compatibility, the system still supports:
- Single `domain` field at workspace level (converted to `domains` array)
- Mailgun `domain` field (used if `sending_domain` is not specified)

However, it's recommended to use the new fields for clarity:
- Use `domains` (array) for workspace routing
- Use `sending_domain` for Mailgun API domain