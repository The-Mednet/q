# Enhanced Gmail Provider Implementation

## Overview

The Gmail provider has been significantly enhanced to address authentication issues and provide better error handling, debugging capabilities, and fallback mechanisms when dealing with Google Workspace domain-wide delegation.

## Key Improvements

### 1. Enhanced Authentication Error Handling

The provider now includes a dedicated `GmailAuthError` type that provides detailed information about authentication failures:

```go
type GmailAuthError struct {
    SenderEmail string
    ErrorType   string // "invalid_grant", "domain_wide_delegation", "user_not_found", etc.
    Message     string
    Cause       error
}
```

### 2. Sender Validation

- **Email Format Validation**: Validates email format before attempting authentication
- **Domain Validation**: Ensures sender email matches supported domains
- **Validation Caching**: Caches validation results to avoid repeated validation attempts
- **Pre-impersonation Checks**: Validates sender before attempting OAuth2 token generation

### 3. Default Sender Fallback

New configuration options allow for graceful fallback:

```json
{
  "gmail": {
    "service_account_file": "credentials/service-account.json",
    "enabled": true,
    "default_sender": "noreply@yourdomain.com",
    "require_valid_sender": true
  }
}
```

- **`default_sender`**: Fallback email when the original sender fails authentication
- **`require_valid_sender`**: Whether to enforce sender validation before attempting sends

### 4. Comprehensive Error Messages

The provider now provides detailed guidance for common setup issues:

- **Invalid Grant Errors**: Detects "Not a valid email or user ID" and provides specific guidance
- **Domain-wide Delegation Issues**: Provides step-by-step setup instructions
- **Service Account Problems**: Clear messages about missing or invalid service account files
- **API Permission Issues**: Specific guidance for scope and permission problems

### 5. Enhanced Logging and Debugging

- **Authentication Flow Logging**: Detailed logs of authentication attempts and results
- **Cache Statistics**: Provides insights into service and validation cache usage
- **Health Check Improvements**: Better health check with meaningful error messages
- **Token Validation**: Tests OAuth2 token generation before caching services

### 6. Validation Cache Management

- **Automatic Expiration**: Validation results expire after 5 minutes
- **Manual Cache Clearing**: Methods to clear validation cache for troubleshooting
- **Cache Statistics**: Track valid, invalid, and expired validation attempts

## Configuration Examples

### Basic Configuration (with fallback)
```json
{
  "id": "mednet-primary",
  "domain": "mednet.org",
  "gmail": {
    "service_account_file": "credentials/mednet-service-account.json",
    "enabled": true,
    "default_sender": "noreply@mednet.org",
    "require_valid_sender": true
  }
}
```

### Testing Configuration (lenient validation)
```json
{
  "id": "testing",
  "domain": "testing.example.com", 
  "gmail": {
    "service_account_file": "credentials/testing-service-account.json",
    "enabled": true,
    "require_valid_sender": false
  }
}
```

## Error Types and Resolutions

### 1. `invalid_grant` - "Not a valid email or user ID"

**Cause**: The sender email is not a valid Google Workspace user.

**Resolution**:
1. Verify the sender email exists in Google Workspace
2. Ensure domain-wide delegation is properly configured
3. Check that the service account has the correct scopes

### 2. `unauthorized_client` 

**Cause**: Service account not authorized for domain-wide delegation.

**Resolution**:
1. Enable domain-wide delegation for the service account
2. Add the service account client ID to Google Admin Console
3. Grant the scope `https://www.googleapis.com/auth/gmail.send`

### 3. `service_account_file`

**Cause**: Service account file is missing or unreadable.

**Resolution**:
1. Verify the file path is correct
2. Check file permissions
3. Ensure the file contains valid JSON

### 4. `user_not_found`

**Cause**: Gmail user doesn't exist or isn't accessible.

**Resolution**:
1. Verify the user exists in Google Workspace
2. Check that the user account is active
3. Ensure the service account has permission to impersonate the user

## API Methods

### New Methods

- `ValidateSender(ctx context.Context, senderEmail string) error` - Validates a sender without caching a service
- `ClearValidationCache(senderEmail string)` - Clears validation cache for debugging
- `getValidationCacheStats() map[string]interface{}` - Returns validation cache statistics

### Enhanced Methods

- `SendMessage()` - Now includes fallback logic and detailed error handling
- `HealthCheck()` - Enhanced with better validation and error reporting
- `GetProviderInfo()` - Includes new capabilities and configuration metadata

## Environment Variables

Add these to your `.env` file for the new features:

```bash
# Gmail Workspace Configuration
GMAIL_SERVICE_ACCOUNT_FILE=credentials/service-account.json
GMAIL_DOMAIN=yourdomain.com

# For multiple workspaces
GMAIL_WORKSPACES_FILE=workspaces.json
# Or from environment variable
GMAIL_WORKSPACES_JSON='[{"id":"workspace1",...}]'
```

## Troubleshooting

### Authentication Issues

1. **Check service account file**: Ensure the file exists and is readable
2. **Verify domain-wide delegation**: Check Google Admin Console settings
3. **Test with default sender**: Configure a default sender for fallback
4. **Enable detailed logging**: Set `LOG_LEVEL=debug` for verbose output
5. **Check validation cache**: Use provider info API to see cache statistics

### Health Check Failures

1. **Use default sender for health checks**: If configured, health checks will use the default sender
2. **Check service account permissions**: Ensure the service account can access Gmail API
3. **Verify network connectivity**: Ensure the service can reach Google APIs

### Performance Optimization

1. **Enable validation caching**: Set `require_valid_sender: true` to cache validation results
2. **Configure appropriate cache expiration**: Validation cache expires after 5 minutes
3. **Monitor cache statistics**: Use the provider info API to track cache effectiveness

## Security Considerations

1. **Service Account Security**: Store service account files securely and limit access
2. **Scope Limitation**: Only grant the minimum required scopes (`gmail.send`)
3. **Sender Validation**: Enable `require_valid_sender` in production environments
4. **Default Sender**: Choose a default sender that's appropriate for your use case
5. **Regular Auditing**: Monitor authentication failures and cache statistics

## Migration from Previous Version

The enhanced provider is backward compatible with existing configurations. To take advantage of new features:

1. **Add default sender**: Add `default_sender` to your workspace configuration
2. **Enable validation**: Set `require_valid_sender: true` for stricter validation
3. **Update error handling**: Handle the new `GmailAuthError` type in your application code
4. **Monitor new metrics**: Use the enhanced provider info to monitor authentication health

## Testing the Implementation

```bash
# Test build
go build -o /dev/null ./internal/provider/

# Test with enhanced logging
LOG_LEVEL=debug ./relay

# Monitor provider health
curl http://localhost:8080/api/providers/gmail-workspace/info
```

The enhanced Gmail provider provides robust, production-ready email sending with comprehensive error handling and debugging capabilities suitable for Mednet's critical healthcare communication needs.