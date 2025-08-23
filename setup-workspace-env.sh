#!/bin/bash

# Example script showing how to set up workspace configuration from environment variable
# This would typically be used with AWS Secrets Manager in production

# Method 1: Set directly as environment variable
export GMAIL_WORKSPACES_JSON='[
  {
    "id": "joinmednet",
    "domain": "joinmednet.org",
    "display_name": "joinmednet.org Workspace",
    "rate_limits": {
      "workspace_daily": 500,
      "per_user_daily": 100,
      "custom_user_limits": {
        "vip@joinmednet.org": 5000,
        "bulk@joinmednet.org": 500
      }
    },
    "gmail": {
      "service_account_file": "/Users/bea/dev/mednet/q/credentials/joinmednet-service-account.json",
      "enabled": true
    }
  },
  {
    "id": "mailgun-primary",
    "domain": "mail.joinmednet.org",
    "display_name": "Mailgun Primary Workspace",
    "rate_limits": {
      "workspace_daily": 1000,
      "per_user_daily": 100,
      "custom_user_limits": {
        "brian@mail.joinmednet.org": 2000
      }
    },
    "mailgun": {
      "api_key": "key-your-mailgun-api-key-here",
      "domain": "mail.joinmednet.org",
      "base_url": "https://api.mailgun.net/v3",
      "region": "us",
      "enabled": true,
      "tracking": {
        "opens": true,
        "clicks": true,
        "unsubscribe": true
      },
      "default_tags": ["mednet", "relay"]
    }
  }
]'

# Method 2: Load from AWS Secrets Manager (example for production)
# export GMAIL_WORKSPACES_JSON=$(aws secretsmanager get-secret-value --secret-id "relay/workspaces" --query 'SecretString' --output text)

# Method 3: Load from file and set as env var (for testing)
# export GMAIL_WORKSPACES_JSON=$(cat workspace.json)

echo "Workspace configuration set in environment variable"
echo "You can now run the SMTP relay service and it will use the environment variable instead of workspace.json"

# Start the service
# ./main