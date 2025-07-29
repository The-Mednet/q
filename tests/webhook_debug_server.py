#!/usr/bin/env python3
"""
FastAPI Webhook Debug Server for SMTP Relay Service

This server is designed to help debug webhooks from the SMTP relay service,
particularly Mandrill-compatible webhook events. It logs all incoming requests
with detailed information including headers, body, query parameters, and timestamps.

Usage:
    python webhook_debug_server.py

The server will start on http://localhost:8000 and accept webhook requests
on various endpoints to match different webhook types.
"""

import json
import logging
import sys
from datetime import datetime
from typing import Any, Dict, Optional

import uvicorn
from fastapi import FastAPI, Request, HTTPException
from fastapi.responses import JSONResponse, PlainTextResponse


# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s',
    handlers=[
        logging.StreamHandler(sys.stdout),
        logging.FileHandler('webhook_debug.log', mode='a')
    ]
)
logger = logging.getLogger("webhook_debug")

app = FastAPI(
    title="Webhook Debug Server",
    description="Debug server for SMTP relay webhook testing",
    version="1.0.0"
)


def log_request_details(request: Request, body: bytes, endpoint: str) -> Dict[str, Any]:
    """Log detailed information about the incoming request."""
    timestamp = datetime.utcnow().isoformat() + "Z"
    
    # Parse body content
    body_str = body.decode('utf-8') if body else ""
    parsed_body = None
    content_type = request.headers.get('content-type', '').lower()
    
    # Try to parse JSON
    if 'application/json' in content_type and body_str:
        try:
            parsed_body = json.loads(body_str)
        except json.JSONDecodeError:
            parsed_body = {"error": "Invalid JSON", "raw": body_str}
    
    # Try to parse form data
    elif 'application/x-www-form-urlencoded' in content_type and body_str:
        try:
            from urllib.parse import parse_qs
            parsed_body = parse_qs(body_str)
        except Exception as e:
            parsed_body = {"error": f"Form parse error: {str(e)}", "raw": body_str}
    
    # Try to parse multipart form data
    elif 'multipart/form-data' in content_type and body_str:
        parsed_body = {"note": "Multipart form data - see raw body", "raw": body_str}
    
    else:
        parsed_body = {"raw": body_str} if body_str else None
    
    request_details = {
        "timestamp": timestamp,
        "endpoint": endpoint,
        "method": request.method,
        "url": str(request.url),
        "headers": dict(request.headers),
        "query_params": dict(request.query_params),
        "body": {
            "content_type": content_type,
            "size_bytes": len(body) if body else 0,
            "parsed": parsed_body,
            "raw_preview": body_str[:500] + "..." if len(body_str) > 500 else body_str
        },
        "client": {
            "host": request.client.host if request.client else "unknown",
            "port": request.client.port if request.client else "unknown"
        }
    }
    
    # Log to console and file
    log_message = f"\n{'='*80}\nWEBHOOK REQUEST RECEIVED\n{'='*80}\n{json.dumps(request_details, indent=2)}\n{'='*80}"
    logger.info(log_message)
    
    return request_details


@app.get("/")
async def root():
    """Root endpoint with server information."""
    return {
        "message": "Webhook Debug Server for SMTP Relay Service",
        "status": "running",
        "timestamp": datetime.utcnow().isoformat() + "Z",
        "endpoints": [
            "/webhook/mandrill",
            "/webhook/mandrill/sent",
            "/webhook/mandrill/delivered", 
            "/webhook/mandrill/bounced",
            "/webhook/mandrill/rejected",
            "/webhook/mandrill/clicked",
            "/webhook/mandrill/opened",
            "/webhook/generic",
            "/webhook/test"
        ]
    }


@app.post("/")
async def root_post(request: Request):
    """Handle POST requests to root path - likely misconfigured webhook."""
    body = await request.body()
    details = log_request_details(request, body, "/")
    
    return JSONResponse(
        status_code=200,
        content={
            "message": "POST request received at root path",
            "note": "This might be a misconfigured webhook. Consider using specific webhook endpoints like /webhook/mandrill",
            "status": "received",
            "timestamp": details["timestamp"],
            "suggested_endpoints": [
                "/webhook/mandrill",
                "/webhook/generic", 
                "/webhook/test"
            ]
        }
    )


@app.get("/health")
async def health_check():
    """Health check endpoint."""
    return {"status": "healthy", "timestamp": datetime.utcnow().isoformat() + "Z"}


# Generic webhook endpoint for any webhook data
@app.post("/webhook/generic")
async def generic_webhook(request: Request):
    """Generic webhook endpoint that accepts any POST data."""
    body = await request.body()
    details = log_request_details(request, body, "/webhook/generic")
    
    return JSONResponse(
        status_code=200,
        content={
            "status": "received",
            "message": "Generic webhook received successfully",
            "timestamp": details["timestamp"],
            "request_id": f"generic-{int(datetime.utcnow().timestamp())}"
        }
    )


# Mandrill-compatible webhook endpoints
@app.post("/webhook/mandrill")
async def mandrill_webhook(request: Request):
    """Main Mandrill webhook endpoint."""
    body = await request.body()
    details = log_request_details(request, body, "/webhook/mandrill")
    
    # Mandrill sends events as form data with 'mandrill_events' parameter
    content_type = request.headers.get('content-type', '').lower()
    
    response_data = {
        "status": "received",
        "message": "Mandrill webhook received successfully", 
        "timestamp": details["timestamp"],
        "request_id": f"mandrill-{int(datetime.utcnow().timestamp())}"
    }
    
    # If it looks like Mandrill format, try to parse events
    if 'application/x-www-form-urlencoded' in content_type:
        try:
            from urllib.parse import parse_qs
            form_data = parse_qs(body.decode('utf-8'))
            if 'mandrill_events' in form_data:
                events_json = form_data['mandrill_events'][0]
                events = json.loads(events_json)
                response_data["events_count"] = len(events)
                logger.info(f"Parsed {len(events)} Mandrill events")
        except Exception as e:
            logger.warning(f"Failed to parse Mandrill events: {str(e)}")
    
    return JSONResponse(status_code=200, content=response_data)


@app.post("/webhook/mandrill/{event_type}")
async def mandrill_event_webhook(event_type: str, request: Request):
    """Specific Mandrill event webhook endpoints (sent, delivered, bounced, etc.)."""
    body = await request.body()
    details = log_request_details(request, body, f"/webhook/mandrill/{event_type}")
    
    # Validate event type
    valid_events = ['sent', 'delivered', 'bounced', 'rejected', 'clicked', 'opened', 'soft_bounced', 'spam', 'unsub']
    if event_type not in valid_events:
        raise HTTPException(
            status_code=400, 
            detail=f"Invalid event type '{event_type}'. Valid types: {', '.join(valid_events)}"
        )
    
    return JSONResponse(
        status_code=200,
        content={
            "status": "received",
            "message": f"Mandrill {event_type} webhook received successfully",
            "event_type": event_type,
            "timestamp": details["timestamp"],
            "request_id": f"mandrill-{event_type}-{int(datetime.utcnow().timestamp())}"
        }
    )


@app.post("/webhook/test")
async def test_webhook(request: Request):
    """Test webhook endpoint for development testing."""
    body = await request.body()
    details = log_request_details(request, body, "/webhook/test")
    
    return JSONResponse(
        status_code=200,
        content={
            "status": "received",
            "message": "Test webhook received successfully",
            "echo": {
                "headers": dict(request.headers),
                "query_params": dict(request.query_params),
                "body_size": len(body) if body else 0,
                "content_type": request.headers.get('content-type', 'unknown')
            },
            "timestamp": details["timestamp"],
            "request_id": f"test-{int(datetime.utcnow().timestamp())}"
        }
    )


# Handle GET requests to webhook endpoints (for testing)
@app.get("/webhook/{path:path}")
async def webhook_get_handler(path: str, request: Request):
    """Handle GET requests to webhook endpoints for testing."""
    details = log_request_details(request, b"", f"/webhook/{path}")
    
    return JSONResponse(
        status_code=200,
        content={
            "message": f"GET request received for webhook path: {path}",
            "note": "This endpoint expects POST requests for webhook data",
            "timestamp": details["timestamp"],
            "request_id": f"get-{path}-{int(datetime.utcnow().timestamp())}"
        }
    )


# Error handlers
@app.exception_handler(404)
async def not_found_handler(request: Request, exc):
    """Handle 404 errors with helpful information."""
    logger.warning(f"404 - Path not found: {request.url.path}")
    return JSONResponse(
        status_code=404,
        content={
            "error": "Endpoint not found",
            "path": request.url.path,
            "message": "Check available endpoints at /",
            "timestamp": datetime.utcnow().isoformat() + "Z"
        }
    )


@app.exception_handler(500)
async def internal_error_handler(request: Request, exc):
    """Handle internal server errors."""
    logger.error(f"500 - Internal server error: {str(exc)}")
    return JSONResponse(
        status_code=500,
        content={
            "error": "Internal server error",
            "message": "Check server logs for details",
            "timestamp": datetime.utcnow().isoformat() + "Z"
        }
    )


if __name__ == "__main__":
    print("🚀 Starting Webhook Debug Server...")
    print("📝 Logs will be written to both console and 'webhook_debug.log'")
    print("🌐 Server will be available at: http://localhost:8000")
    print("📚 API documentation at: http://localhost:8000/docs")
    print("❤️  Health check at: http://localhost:8000/health")
    print("\n🔗 Available webhook endpoints:")
    print("   • POST /webhook/mandrill - Main Mandrill webhook")
    print("   • POST /webhook/mandrill/{event_type} - Specific event types")
    print("   • POST /webhook/generic - Generic webhook handler")
    print("   • POST /webhook/test - Test endpoint with echo")
    print("\n⏹️  Press Ctrl+C to stop the server\n")
    
    try:
        uvicorn.run(
            "webhook_debug_server:app",
            host="0.0.0.0",
            port=8000,
            reload=False,  # Set to True for development if you want auto-reload
            log_level="info",
            access_log=True
        )
    except KeyboardInterrupt:
        print("\n👋 Webhook Debug Server stopped")
        sys.exit(0)