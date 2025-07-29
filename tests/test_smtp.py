#!/usr/bin/env python3
"""
Test script for sending emails through the Q SMTP relay service.
Tests workspace routing, campaign tracking, and user tracking.
"""

import smtplib
import argparse
import uuid
from email.mime.text import MIMEText
from email.mime.multipart import MIMEMultipart
from datetime import datetime

def send_test_email(
    smtp_host="localhost",
    smtp_port=2525,
    from_email="brian@joinmednet.org",
    from_name=None,
    to_email="b@smada.org",
    campaign_id=123,
    user_id=456,
    subject="Get Expert Answers to Complex Clinical Questions",
    message_type="text",
    custom_text=None,
    custom_html=None,
    message_file=None
):
    """Send a test email through the Q SMTP relay"""
    
    # Generate defaults if not provided
    if not campaign_id:
        campaign_id = f"test-campaign-{uuid.uuid4().hex[:8]}"
    if not user_id:
        user_id = f"user-{uuid.uuid4().hex[:6]}"
    if not subject:
        subject = f"Test Email - {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}"
    
    # Handle message content from file
    if message_file:
        try:
            with open(message_file, 'r', encoding='utf-8') as f:
                file_content = f.read()
                if message_file.endswith('.html'):
                    custom_html = file_content
                    message_type = "html"
                else:
                    custom_text = file_content
        except Exception as e:
            print(f"❌ Error reading message file: {e}")
            return False
    
    # Create message
    if message_type == "html":
        msg = MIMEMultipart("alternative")
        
        # Use custom text or default
        if custom_text:
            text_content = custom_text
        else:
            text_content = f"""
Hi Dr. Adams,
I'm reaching out to invite you to join theMednet, a physician-only Q&A platform run by a multidisciplinary group of faculty and fellows. Our mission is to answer every doctor's question and connect you with expert subspecialists from top institutions like MGH, UCSF, Michigan, and Duke, addressing clinical scenarios not covered by UpToDate, PubMed, or guidelines.
Join with this customized link (please do not share):

https://themednet.org/join/1234567890

Join over 37,000 physicians across internal medicine subspecialties and see why we have received grant funding from the NIH while earning up to 20 AMA PRA Category 1 Credits™ annually through our partnership with the University of Chicago.


Jason John, MD
Assistant Professor, University of Colorado
Internal Medicine Deputy Editor, theMednet

---
Test email sent via Q SMTP Relay
Campaign ID: {campaign_id}
User ID: {user_id}
From: {from_email}
Timestamp: {datetime.now().isoformat()}
            """.strip()
        
        # Use custom HTML or default
        if custom_html:
            html_content = custom_html
        else:
            html_content = f"""
<!DOCTYPE html>
<html>
<head>
    <title>theMednet Invitation</title>
</head>
<body>
    <p>Hi Dr. Adams,</p>
    <p>I'm reaching out to invite you to join <strong>theMednet</strong>, a physician-only Q&A platform run by a multidisciplinary group of faculty and fellows. Our mission is to answer every doctor's question and connect you with expert subspecialists from top institutions like MGH, UCSF, Michigan, and Duke, addressing clinical scenarios not covered by UpToDate, PubMed, or guidelines.</p>
    
    <p><strong>Join with this customized link (please do not share):</strong></p>
    <p><a href="https://themednet.org/join/1234567890">https://themednet.org/join/1234567890</a></p>
    
    <p>Join over 37,000 physicians across internal medicine subspecialties and see why we have received grant funding from the NIH while earning up to 20 AMA PRA Category 1 Credits™ annually through our partnership with the University of Chicago.</p>
    
    <p>Jason John, MD<br>
    Assistant Professor, University of Colorado<br>
    Internal Medicine Deputy Editor, theMednet</p>
    
    <hr>
    <div style="background: #f5f5f5; padding: 10px; font-size: 12px; color: #666;">
        <strong>Test Email Metadata:</strong><br>
        Campaign ID: {campaign_id}<br>
        User ID: {user_id}<br>
        From: {from_email}<br>
        Timestamp: {datetime.now().isoformat()}
    </div>
</body>
</html>
            """.strip()
        
        msg.attach(MIMEText(text_content, "plain"))
        msg.attach(MIMEText(html_content, "html"))
    else:
        # Use custom text or default for plain text emails
        if custom_text:
            content = custom_text
        else:
            content = f"""
Hi Dr. Adams,
I'm reaching out to invite you to join theMednet, a physician-only Q&A platform run by a multidisciplinary group of faculty and fellows. Our mission is to answer every doctor's question and connect you with expert subspecialists from top institutions like MGH, UCSF, Michigan, and Duke, addressing clinical scenarios not covered by UpToDate, PubMed, or guidelines.
Join with this customized link (please do not share):

https://themednet.org/join/1234567890

Join over 37,000 physicians across internal medicine subspecialties and see why we have received grant funding from the NIH while earning up to 20 AMA PRA Category 1 Credits™ annually through our partnership with the University of Chicago.


Jason John, MD
Assistant Professor, University of Colorado
Internal Medicine Deputy Editor, theMednet

---
Test Email Metadata:
Campaign ID: {campaign_id}
User ID: {user_id}
From: {from_email}
Timestamp: {datetime.now().isoformat()}
            """.strip()
        
        msg = MIMEText(content)
    
    # Set basic headers with optional display name
    if from_name:
        msg["From"] = f"{from_name} <{from_email}>"
    else:
        msg["From"] = from_email
    msg["To"] = to_email
    msg["Subject"] = subject
    
    # Add custom tracking headers
    msg["X-Campaign-ID"] = campaign_id
    msg["X-User-ID"] = user_id
    
    # Add additional test headers
    msg["X-Test-Script"] = "q-smtp-test.py"
    msg["X-Test-Timestamp"] = datetime.now().isoformat()
    
    try:
        # Connect to SMTP server
        print(f"Connecting to SMTP server at {smtp_host}:{smtp_port}...")
        with smtplib.SMTP(smtp_host, smtp_port) as server:
            # No authentication needed for local SMTP relay
            print("Sending email...")
            
            # Send the email
            server.send_message(msg)
            
            print("✅ Email sent successfully!")
            print(f"   From: {from_email}")
            print(f"   To: {to_email}")
            print(f"   Subject: {subject}")
            print(f"   Campaign ID: {campaign_id}")
            print(f"   User ID: {user_id}")
            
            return True
            
    except Exception as e:
        print(f"❌ Failed to send email: {e}")
        return False

def test_workspace_routing():
    """Test different workspace routing scenarios"""
    
    test_cases = [
        {
            "name": "Direct workspace match",
            "from_email": "doctor1@joinmednet.org",
            "description": "Should route to joinmednet.org workspace"
        },
        {
            "name": "Legacy domain random routing", 
            "from_email": "user@mednet.org",
            "description": "Should randomly select workspace for legacy domain"
        },
        {
            "name": "Another legacy domain",
            "from_email": "sender@themednet.org", 
            "description": "Should randomly select workspace for legacy domain"
        }
    ]
    
    print("\n🧪 Testing Workspace Routing...\n")
    
    for i, test in enumerate(test_cases, 1):
        print(f"Test {i}: {test['name']}")
        print(f"Description: {test['description']}")
        
        success = send_test_email(
            from_email=test["from_email"],
            to_email="test-recipient@example.com",
            campaign_id=f"routing-test-{i}",
            user_id=f"test-user-{i}",
            subject=f"Workspace Routing Test {i}: {test['name']}"
        )
        
        if success:
            print("✅ Test passed\n")
        else:
            print("❌ Test failed\n")

def test_rate_limiting():
    """Test rate limiting by sending multiple emails quickly"""
    
    print("\n🧪 Testing Rate Limiting...\n")
    
    from_email = "rate-test@joinmednet.org"
    campaign_id = f"rate-limit-test-{uuid.uuid4().hex[:8]}"
    
    print(f"Sending 5 emails quickly from {from_email}")
    print("This should help test the rate limiting functionality.\n")
    
    for i in range(1, 6):
        print(f"Sending email {i}/5...")
        success = send_test_email(
            from_email=from_email,
            to_email=f"test{i}@example.com",
            campaign_id=campaign_id,
            user_id=f"rate-test-user-{i}",
            subject=f"Rate Limit Test Email {i}/5",
            message_type="html"
        )
        
        if not success:
            print(f"❌ Failed on email {i}")
            break
            
        print()

def main():
    parser = argparse.ArgumentParser(description="Test Q SMTP Relay Service")
    parser.add_argument("--host", default="localhost", help="SMTP host (default: localhost)")
    parser.add_argument("--port", type=int, default=2525, help="SMTP port (default: 2525)")
    parser.add_argument("--from", dest="from_email", default="test@joinmednet.org", 
                       help="Sender email address")
    parser.add_argument("--from-name", dest="from_name", 
                       help="Sender display name (e.g., 'Dr. John Smith')")
    parser.add_argument("--to", dest="to_email", default="recipient@example.com",
                       help="Recipient email address")
    parser.add_argument("--campaign", dest="campaign_id", help="Campaign ID")
    parser.add_argument("--user", dest="user_id", help="User ID")
    parser.add_argument("--subject", help="Email subject")
    parser.add_argument("--html", action="store_true", help="Send HTML email")
    parser.add_argument("--text", help="Custom plain text message content")
    parser.add_argument("--html-content", help="Custom HTML message content")
    parser.add_argument("--file", help="Read message content from file (.txt for plain text, .html for HTML)")
    parser.add_argument("--test-routing", action="store_true", 
                       help="Run workspace routing tests")
    parser.add_argument("--test-rate-limit", action="store_true",
                       help="Run rate limiting tests")
    
    args = parser.parse_args()
    
    if args.test_routing:
        test_workspace_routing()
    elif args.test_rate_limit:
        test_rate_limiting()
    else:
        # Send single test email
        message_type = "html" if args.html or args.html_content else "text"
        send_test_email(
            smtp_host=args.host,
            smtp_port=args.port,
            from_email=args.from_email,
            from_name=args.from_name,
            to_email=args.to_email,
            campaign_id=args.campaign_id,
            user_id=args.user_id,
            subject=args.subject,
            message_type=message_type,
            custom_text=args.text,
            custom_html=args.html_content,
            message_file=args.file
        )

if __name__ == "__main__":
    main()