#!/bin/bash

# Test email with headers to Mailgun workspace
echo "Testing header rewrite for Mailgun workspace..."

# Test 1: Email with existing List-Unsubscribe header (should be replaced)
echo "Test 1: Email with existing header (should be replaced)"
{
    echo "EHLO localhost"
    echo "MAIL FROM: test@mail.joinmednet.org"
    echo "RCPT TO: recipient@example.com"
    echo "DATA"
    echo "From: test@mail.joinmednet.org"
    echo "To: recipient@example.com"
    echo "Subject: Test Email with Existing Header"
    echo "List-Unsubscribe: <https://old-provider.com/unsubscribe/xyz123>"
    echo ""
    echo "This email has an existing List-Unsubscribe header that should be replaced."
    echo "."
    echo "QUIT"
} | nc localhost 2525

echo ""
echo "Test 2: Email without headers (should add missing headers)"
{
    echo "EHLO localhost"
    echo "MAIL FROM: test@mail.joinmednet.org"
    echo "RCPT TO: recipient@example.com"
    echo "DATA"
    echo "From: test@mail.joinmednet.org"
    echo "To: recipient@example.com"
    echo "Subject: Test Email without Headers"
    echo ""
    echo "This email has no unsubscribe headers - they should be added."
    echo "."
    echo "QUIT"
} | nc localhost 2525

echo ""
echo "Test 3: Gmail workspace email (should pass through unchanged)"
{
    echo "EHLO localhost"
    echo "MAIL FROM: test@joinmednet.org"
    echo "RCPT TO: recipient@example.com"
    echo "DATA"
    echo "From: test@joinmednet.org"
    echo "To: recipient@example.com"
    echo "Subject: Gmail Test Email"
    echo "List-Unsubscribe: <https://mandrill.com/unsubscribe/abc123>"
    echo ""
    echo "This Gmail email should pass through unchanged."
    echo "."
    echo "QUIT"
} | nc localhost 2525

echo ""
echo "Done! Check the server logs for header rewrite messages."