#!/usr/bin/env python3
"""
Test client for the webhook debug server.

This script demonstrates how to send various types of webhook requests
to test the debug server functionality.
"""

import json
import requests
import time
from urllib.parse import urlencode


def test_webhook_endpoints():
    """Test various webhook endpoints with different data types."""
    base_url = "http://localhost:8000"
    
    print("🧪 Testing Webhook Debug Server\n")
    
    # Test 1: Generic JSON webhook
    print("1️⃣  Testing generic webhook with JSON data...")
    json_data = {
        "event": "email_sent",
        "message_id": "test-123",
        "recipient": "test@example.com",
        "timestamp": int(time.time()),
        "metadata": {
            "campaign": "welcome_series",
            "user_id": "user-456"
        }
    }
    
    try:
        response = requests.post(
            f"{base_url}/webhook/generic",
            json=json_data,
            headers={"User-Agent": "TestClient/1.0"}
        )
        print(f"   ✅ Status: {response.status_code}")
        print(f"   📄 Response: {response.json()}")
    except Exception as e:
        print(f"   ❌ Error: {e}")
    
    print()
    
    # Test 2: Mandrill-style form data
    print("2️⃣  Testing Mandrill webhook with form data...")
    mandrill_events = [
        {
            "event": "send",
            "msg": {
                "_id": "abc123",
                "ts": int(time.time()),
                "subject": "Test Email Subject",
                "email": "recipient@example.com",
                "sender": "sender@example.com",
                "tags": ["welcome", "onboarding"],
                "state": "sent"
            }
        }
    ]
    
    form_data = {
        "mandrill_events": json.dumps(mandrill_events)
    }
    
    try:
        response = requests.post(
            f"{base_url}/webhook/mandrill",
            data=form_data,
            headers={
                "Content-Type": "application/x-www-form-urlencoded",
                "User-Agent": "Mandrill-Webhook/1.0"
            }
        )
        print(f"   ✅ Status: {response.status_code}")
        print(f"   📄 Response: {response.json()}")
    except Exception as e:
        print(f"   ❌ Error: {e}")
    
    print()
    
    # Test 3: Specific Mandrill event type
    print("3️⃣  Testing specific Mandrill event (delivered)...")
    delivered_data = {
        "mandrill_events": json.dumps([{
            "event": "deliver",
            "msg": {
                "_id": "def456",
                "ts": int(time.time()),
                "subject": "Successfully Delivered Email",
                "email": "delivered@example.com",
                "sender": "noreply@mednet.com",
                "state": "delivered"
            }
        }])
    }
    
    try:
        response = requests.post(
            f"{base_url}/webhook/mandrill/delivered",
            data=delivered_data,
            headers={"Content-Type": "application/x-www-form-urlencoded"}
        )
        print(f"   ✅ Status: {response.status_code}")
        print(f"   📄 Response: {response.json()}")
    except Exception as e:
        print(f"   ❌ Error: {e}")
    
    print()
    
    # Test 4: Test endpoint with various headers
    print("4️⃣  Testing test endpoint with custom headers...")
    test_data = {"test": "data", "numbers": [1, 2, 3]}
    
    try:
        response = requests.post(
            f"{base_url}/webhook/test?source=test_client&version=1.0",
            json=test_data,
            headers={
                "X-Custom-Header": "CustomValue",
                "X-Request-ID": "test-request-123",
                "Authorization": "Bearer test-token"
            }
        )
        print(f"   ✅ Status: {response.status_code}")
        print(f"   📄 Response: {response.json()}")
    except Exception as e:
        print(f"   ❌ Error: {e}")
    
    print()
    
    # Test 5: Health check
    print("5️⃣  Testing health check...")
    try:
        response = requests.get(f"{base_url}/health")
        print(f"   ✅ Status: {response.status_code}")
        print(f"   📄 Response: {response.json()}")
    except Exception as e:
        print(f"   ❌ Error: {e}")
    
    print("\n✨ All tests completed! Check the webhook debug server logs for detailed request information.")


def test_error_cases():
    """Test error cases and edge conditions."""
    base_url = "http://localhost:8000"
    
    print("\n🚨 Testing Error Cases\n")
    
    # Test invalid event type
    print("1️⃣  Testing invalid Mandrill event type...")
    try:
        response = requests.post(f"{base_url}/webhook/mandrill/invalid_event")
        print(f"   ⚠️  Status: {response.status_code}")
        print(f"   📄 Response: {response.json()}")
    except Exception as e:
        print(f"   ❌ Error: {e}")
    
    print()
    
    # Test 404
    print("2️⃣  Testing 404 endpoint...")
    try:
        response = requests.post(f"{base_url}/nonexistent/endpoint")
        print(f"   ⚠️  Status: {response.status_code}")
        print(f"   📄 Response: {response.json()}")
    except Exception as e:
        print(f"   ❌ Error: {e}")


if __name__ == "__main__":
    print("🔧 Make sure the webhook debug server is running:")
    print("   python webhook_debug_server.py")
    print("\n" + "="*60 + "\n")
    
    try:
        # Quick health check to see if server is running
        response = requests.get("http://localhost:8000/health", timeout=5)
        if response.status_code == 200:
            test_webhook_endpoints()
            test_error_cases()
        else:
            print("❌ Server responded but not healthy")
    except requests.exceptions.ConnectionError:
        print("❌ Cannot connect to webhook debug server at http://localhost:8000")
        print("   Please start the server first: python webhook_debug_server.py")
    except Exception as e:
        print(f"❌ Unexpected error: {e}")