#!/bin/bash

echo "========================================="
echo "LOAD BALANCING TEST (Raw SMTP)"
echo "========================================="

# Test load balancing pools

echo ""
echo "Test 1: invite.com domain (capacity_weighted pool)"
{
    echo "EHLO localhost"
    echo "MAIL FROM: test@invite.com"
    echo "RCPT TO: brian@themednet.org"
    echo "DATA"
    echo "From: test@invite.com"
    echo "To: brian@themednet.org"
    echo "Subject: Load Balance Test - invite.com"
    echo ""
    echo "Testing load balancing for invite.com domain."
    echo "Should route through invite-domain-pool."
    echo "."
    echo "QUIT"
} | nc localhost 2525

sleep 1

echo ""
echo "Test 2: invitations.mednet.org domain (capacity_weighted pool)"
{
    echo "EHLO localhost"
    echo "MAIL FROM: test@invitations.mednet.org"
    echo "RCPT TO: brian@themednet.org"
    echo "DATA"
    echo "From: test@invitations.mednet.org"
    echo "To: brian@themednet.org"
    echo "Subject: Load Balance Test - invitations.mednet.org"
    echo ""
    echo "Testing load balancing for invitations.mednet.org domain."
    echo "Should route through invite-domain-pool."
    echo "."
    echo "QUIT"
} | nc localhost 2525

sleep 1

echo ""
echo "Test 3: notifications.mednet.org domain (least_used pool)"
{
    echo "EHLO localhost"
    echo "MAIL FROM: alert@notifications.mednet.org"
    echo "RCPT TO: brian@themednet.org"
    echo "DATA"
    echo "From: alert@notifications.mednet.org"
    echo "To: brian@themednet.org"
    echo "Subject: Load Balance Test - notifications.mednet.org"
    echo ""
    echo "Testing load balancing for notifications.mednet.org domain."
    echo "Should route through medical-notifications-pool."
    echo "."
    echo "QUIT"
} | nc localhost 2525

sleep 1

echo ""
echo "Test 4: mednet.org domain (round_robin pool)"
{
    echo "EHLO localhost"
    echo "MAIL FROM: info@mednet.org"
    echo "RCPT TO: brian@themednet.org"
    echo "DATA"
    echo "From: info@mednet.org"
    echo "To: brian@themednet.org"
    echo "Subject: Load Balance Test - mednet.org"
    echo ""
    echo "Testing load balancing for mednet.org domain."
    echo "Should route through general-pool."
    echo "."
    echo "QUIT"
} | nc localhost 2525

sleep 1

echo ""
echo "Test 5: Direct domain (no pool)"
{
    echo "EHLO localhost"
    echo "MAIL FROM: direct@joinmednet.org"
    echo "RCPT TO: brian@themednet.org"
    echo "DATA"
    echo "From: direct@joinmednet.org"
    echo "To: brian@themednet.org"
    echo "Subject: Direct Domain Test - joinmednet.org"
    echo ""
    echo "Testing direct domain routing (no load balancing)."
    echo "Should route directly to joinmednet workspace."
    echo "."
    echo "QUIT"
} | nc localhost 2525

echo ""
echo "========================================="
echo "Tests completed!"
echo "Check logs: tail -f /tmp/relay_load_balancing.log | grep -i 'load\\|pool\\|select'"
echo "Check database: mysql -u relay -prelay relay -e 'SELECT * FROM load_balancing_selections ORDER BY selected_at DESC LIMIT 10;'"
echo "========================================="