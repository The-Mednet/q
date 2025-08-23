#!/bin/bash

echo "Testing Gmail header removal..."
swaks \
  --to test@example.com \
  --from brian@joinmednet.org \
  --header "List-Unsubscribe: <mailto:unsubscribe@example.com>" \
  --header "List-Unsubscribe-Post: List-Unsubscribe=One-Click" \
  --header "Subject: Test Gmail Header Removal" \
  --body "This email should have List-Unsubscribe headers removed" \
  --server localhost:2525 \
  --auth-user test \
  --auth-password test123
