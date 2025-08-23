#!/usr/bin/env python3
"""
Test script for load balancing feature.
Sends emails to generic domains that should be routed through load balancing pools.
"""

import smtplib
import os
from email.mime.text import MIMEText
from email.mime.multipart import MIMEMultipart
from datetime import datetime

# SMTP configuration
SMTP_HOST = "localhost"
SMTP_PORT = 2525
SMTP_USERNAME = "test"
SMTP_PASSWORD = "test123"

def send_test_email(from_addr, to_addr, subject_suffix=""):
    """Send a test email through the relay."""
    msg = MIMEMultipart()
    msg['From'] = from_addr
    msg['To'] = to_addr
    msg['Subject'] = f"Load Balancing Test {subject_suffix} - {datetime.now().strftime('%H:%M:%S')}"
    
    body = f"""
    This is a test email for load balancing.
    
    From: {from_addr}
    To: {to_addr}
    Time: {datetime.now()}
    
    This email should be routed through the load balancing pool
    for the domain in the From address.
    """
    
    msg.attach(MIMEText(body, 'plain'))
    
    try:
        with smtplib.SMTP(SMTP_HOST, SMTP_PORT) as server:
            server.login(SMTP_USERNAME, SMTP_PASSWORD)
            server.send_message(msg)
            print(f"✓ Email sent: {from_addr} -> {to_addr}")
            return True
    except Exception as e:
        print(f"✗ Failed to send: {from_addr} -> {to_addr}")
        print(f"  Error: {e}")
        return False

def main():
    """Run load balancing tests."""
    print("=" * 60)
    print("LOAD BALANCING TEST")
    print("=" * 60)
    
    # Test cases for different pools
    test_cases = [
        # invite-domain-pool (capacity_weighted strategy)
        ("test1@invite.com", "brian@themednet.org", "Pool: invite-domain"),
        ("test2@invite.com", "brian@themednet.org", "Pool: invite-domain"),
        ("test3@invitations.mednet.org", "brian@themednet.org", "Pool: invite-domain"),
        
        # medical-notifications-pool (least_used strategy)
        ("alert@notifications.mednet.org", "brian@themednet.org", "Pool: medical-notifications"),
        ("system@alerts.mednet.org", "brian@themednet.org", "Pool: medical-notifications"),
        
        # general-pool (round_robin strategy)
        ("info@mednet.org", "brian@themednet.org", "Pool: general"),
        ("support@mail.mednet.org", "brian@themednet.org", "Pool: general"),
        
        # Direct domain routing (no pool)
        ("direct@joinmednet.org", "brian@themednet.org", "Direct: joinmednet.org"),
        ("direct@themednet.org", "brian@themednet.org", "Direct: themednet.org"),
    ]
    
    success_count = 0
    total_count = len(test_cases)
    
    print(f"\nSending {total_count} test emails...\n")
    
    for from_addr, to_addr, description in test_cases:
        print(f"Test: {description}")
        if send_test_email(from_addr, to_addr, description):
            success_count += 1
        print()
    
    print("=" * 60)
    print(f"Results: {success_count}/{total_count} emails sent successfully")
    print("=" * 60)
    
    if success_count == total_count:
        print("\n✓ All tests passed!")
    else:
        print(f"\n✗ {total_count - success_count} tests failed")
    
    print("\nCheck the relay logs and database for load balancing details:")
    print("  - Logs: tail -f /tmp/relay_load_balancing.log")
    print("  - Database: mysql -u relay -prelay relay")
    print("    SELECT * FROM load_balancing_selections ORDER BY selected_at DESC LIMIT 10;")

if __name__ == "__main__":
    main()