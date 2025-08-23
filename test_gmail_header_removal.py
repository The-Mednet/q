#!/usr/bin/env python3

import smtplib
from email.mime.text import MIMEText
from email.mime.multipart import MIMEMultipart

# SMTP server configuration
smtp_server = "localhost"
smtp_port = 2525
username = "test"
password = "test123"

# Email configuration
sender = "brian@joinmednet.org"  # Gmail workspace sender
recipient = "test@example.com"
subject = "Test Gmail Header Removal"

# Create message
msg = MIMEMultipart()
msg['From'] = sender
msg['To'] = recipient
msg['Subject'] = subject

# Add headers that should be removed based on workspace.json config
msg['List-Unsubscribe'] = '<mailto:unsubscribe@example.com>'
msg['List-Unsubscribe-Post'] = 'List-Unsubscribe=One-Click'

# Add body
body = "This email should have List-Unsubscribe headers removed by Gmail workspace config"
msg.attach(MIMEText(body, 'plain'))

# Send email
try:
    # Connect to server
    print(f"Connecting to SMTP server at {smtp_server}:{smtp_port}")
    server = smtplib.SMTP(smtp_server, smtp_port)
    
    # Login
    print(f"Authenticating as {username}")
    server.login(username, password)
    
    # Send email
    print(f"Sending email from {sender} to {recipient}")
    print(f"Headers being sent:")
    print(f"  List-Unsubscribe: {msg.get('List-Unsubscribe')}")
    print(f"  List-Unsubscribe-Post: {msg.get('List-Unsubscribe-Post')}")
    
    server.send_message(msg)
    print("Email sent successfully!")
    
    # Close connection
    server.quit()
    
    print("\nExpected behavior:")
    print("- Gmail workspace (joinmednet.org) should REMOVE these headers")
    print("- Check logs for 'Removing header' messages")
    
except Exception as e:
    print(f"Error: {e}")