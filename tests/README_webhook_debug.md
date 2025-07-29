# Webhook Debug Server

A simple FastAPI server for debugging webhooks from the SMTP relay service, particularly Mandrill-compatible webhook events.

## Features

- 🔍 **Detailed Logging**: Logs all incoming requests with headers, body, query params, and timestamps
- 📝 **Multiple Formats**: Handles JSON, form data, and multipart form data
- 🎯 **Mandrill Compatible**: Specific endpoints for Mandrill webhook events
- 📊 **Real-time Monitoring**: Console and file logging with structured output
- 🌐 **REST API**: Clean API with OpenAPI documentation
- ⚡ **Fast Setup**: Single Python file with minimal dependencies

## Quick Start

### 1. Install Dependencies

```bash
cd tests
pip install -r requirements.txt
```

### 2. Start the Server

```bash
python webhook_debug_server.py
```

The server will start on `http://localhost:8000`

### 3. View API Documentation

Visit `http://localhost:8000/docs` for interactive API documentation.

### 4. Test the Server

```bash
python test_webhook_client.py
```

## Available Endpoints

### Core Endpoints
- `GET /` - Server information and available endpoints
- `GET /health` - Health check endpoint
- `GET /docs` - Interactive API documentation

### Webhook Endpoints
- `POST /webhook/generic` - Generic webhook handler for any data
- `POST /webhook/test` - Test endpoint with request echo
- `POST /webhook/mandrill` - Main Mandrill webhook endpoint
- `POST /webhook/mandrill/{event_type}` - Specific Mandrill event handlers

### Supported Mandrill Event Types
- `sent` - Email was sent successfully
- `delivered` - Email was delivered to recipient
- `bounced` - Email bounced (hard bounce)
- `soft_bounced` - Email soft bounced (temporary failure)
- `rejected` - Email was rejected
- `clicked` - Recipient clicked a link in the email
- `opened` - Recipient opened the email
- `spam` - Email was marked as spam
- `unsub` - Recipient unsubscribed

## Request Logging

All requests are logged with detailed information:

```json
{
  "timestamp": "2025-01-29T12:34:56.789Z",
  "endpoint": "/webhook/mandrill",
  "method": "POST",
  "url": "http://localhost:8000/webhook/mandrill?source=smtp_relay",
  "headers": {
    "content-type": "application/x-www-form-urlencoded",
    "user-agent": "SMTP-Relay/1.0",
    "x-request-id": "req-123"
  },
  "query_params": {
    "source": "smtp_relay"
  },
  "body": {
    "content_type": "application/x-www-form-urlencoded",
    "size_bytes": 245,
    "parsed": {
      "mandrill_events": ["..."]
    },
    "raw_preview": "mandrill_events=%5B%7B%22event%22..."
  },
  "client": {
    "host": "127.0.0.1",
    "port": 52341
  }
}
```

## Log Files

- **Console**: Real-time request logging with colors
- **File**: `webhook_debug.log` - Persistent log file for all requests

## Example Usage

### Testing with curl

```bash
# Test generic webhook with JSON
curl -X POST http://localhost:8000/webhook/generic \
  -H "Content-Type: application/json" \
  -d '{"event": "test", "data": {"key": "value"}}'

# Test Mandrill webhook with form data
curl -X POST http://localhost:8000/webhook/mandrill \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d 'mandrill_events=[{"event":"send","msg":{"_id":"123","email":"test@example.com"}}]'

# Test specific Mandrill event
curl -X POST http://localhost:8000/webhook/mandrill/delivered \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d 'mandrill_events=[{"event":"deliver","msg":{"_id":"456","state":"delivered"}}]'
```

### Integration with SMTP Relay Service

Configure your SMTP relay service to send webhooks to:

```
http://localhost:8000/webhook/mandrill
```

Or for specific events:

```
http://localhost:8000/webhook/mandrill/sent
http://localhost:8000/webhook/mandrill/delivered
http://localhost:8000/webhook/mandrill/bounced
```

## Development

### Running in Development Mode

For auto-reload during development, modify the server startup:

```python
uvicorn.run(
    "webhook_debug_server:app",
    host="0.0.0.0",
    port=8000,
    reload=True,  # Enable auto-reload
    log_level="info"
)
```

### Customizing Port

Change the port by modifying the `uvicorn.run()` call:

```python
uvicorn.run(
    "webhook_debug_server:app",
    host="0.0.0.0",
    port=9000,  # Custom port
    reload=False
)
```

### Adding Custom Endpoints

Add new webhook endpoints by creating new FastAPI route handlers:

```python
@app.post("/webhook/custom")
async def custom_webhook(request: Request):
    body = await request.body()
    details = log_request_details(request, body, "/webhook/custom")
    
    return JSONResponse(
        status_code=200,
        content={"status": "received", "timestamp": details["timestamp"]}
    )
```

## Troubleshooting

### Server Won't Start
- Check if port 8000 is already in use
- Install dependencies: `pip install -r requirements.txt`
- Check Python version (requires Python 3.7+)

### No Logs Appearing
- Check file permissions for `webhook_debug.log`
- Verify webhook client is sending requests to correct endpoints
- Check server console output for errors

### Connection Refused
- Ensure server is running: `python webhook_debug_server.py`
- Check firewall settings if accessing from remote machine
- Verify correct host/port in webhook configuration

## Files

- `webhook_debug_server.py` - Main FastAPI server
- `test_webhook_client.py` - Test client with example requests
- `requirements.txt` - Python dependencies
- `webhook_debug.log` - Request log file (created automatically)
- `README_webhook_debug.md` - This documentation