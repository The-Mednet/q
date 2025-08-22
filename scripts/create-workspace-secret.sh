#!/bin/bash

# Script to create Kubernetes secret for workspace configuration
# Usage: ./create-workspace-secret.sh

echo "Creating Workspace Configuration Secret for Kubernetes"
echo "======================================================"
echo ""

# Define the workspace configuration JSON
WORKSPACE_CONFIG='[
  {
    "id": "joinmednet",
    "domains": ["joinmednet.org", "www.joinmednet.org", "app.joinmednet.org"],
    "display_name": "JoinMednet Multi-Domain Workspace",
    "rate_limits": {
      "workspace_daily": 500,
      "per_user_daily": 100
    },
    "gmail": {
      "service_account_env": "RELAY_JOINMEDNET_SERVICE_ACCOUNT",
      "enabled": true,
      "default_sender": "brian@joinmednet.org",
      "header_rewrite": {
        "enabled": true,
        "rules": [
          {
            "header_name": "List-Unsubscribe"
          },
          {
            "header_name": "List-Unsubscribe-Post"
          }
        ]
      }
    }
  },
  {
    "id": "mednetmail",
    "domains": ["mednetmail.org", "mail.mednetmail.org", "notify.mednetmail.org"],
    "display_name": "MednetMail Multi-Domain Workspace",
    "rate_limits": {
      "workspace_daily": 500,
      "per_user_daily": 100
    },
    "gmail": {
      "service_account_env": "RELAY_MEDNETMAIL_SERVICE_ACCOUNT",
      "enabled": true,
      "default_sender": "brian@mednetmail.org",
      "header_rewrite": {
        "enabled": true,
        "rules": [
          {
            "header_name": "List-Unsubscribe"
          },
          {
            "header_name": "List-Unsubscribe-Post"
          }
        ]
      }
    }
  },
  {
    "id": "mailgun-primary",
    "domain": "mail.joinmednet.org",
    "display_name": "Mailgun Primary Workspace",
    "rate_limits": {
      "workspace_daily": 1000,
      "per_user_daily": 100
    },
    "mailgun": {
      "api_key": "$(echo $MAILGUN_API_KEY)",
      "sending_domain": "mg.joinmednet.org",
      "base_url": "https://api.mailgun.net/v3",
      "region": "us",
      "enabled": true,
      "tracking": {
        "opens": true,
        "clicks": true,
        "unsubscribe": true
      },
      "header_rewrite": {
        "enabled": true,
        "rules": [
          {
            "header_name": "List-Unsubscribe",
            "new_value": "<%unsubscribe_url%>"
          },
          {
            "header_name": "List-Unsubscribe-Post",
            "new_value": "List-Unsubscribe=One-Click"
          }
        ]
      }
    }
  },
  {
    "id": "mandrill-transactional",
    "domain": "transactional.joinmednet.org",
    "display_name": "Mandrill Transactional Workspace",
    "rate_limits": {
      "workspace_daily": 10000,
      "per_user_daily": 500
    },
    "mandrill": {
      "api_key": "$(echo $MANDRILL_API_KEY)",
      "enabled": false,
      "subaccount": "production",
      "tracking": {
        "opens": true,
        "clicks": true,
        "auto_text": true,
        "inline_css": true
      },
      "header_rewrite": {
        "enabled": true,
        "rules": [
          {
            "header_name": "List-Unsubscribe",
            "new_value": "<mailto:unsubscribe@joinmednet.org>"
          }
        ]
      }
    }
  }
]'

# Create the secret
kubectl create secret generic q-workspace-config \
  --namespace=production \
  --from-literal=RELAY_WORKSPACE_CONFIG="$WORKSPACE_CONFIG" \
  --dry-run=client -o yaml > q-workspace-config-secret.yaml

echo "Secret YAML created: q-workspace-config-secret.yaml"
echo ""
echo "To apply the secret to your cluster, run:"
echo "  kubectl apply -f q-workspace-config-secret.yaml"
echo ""
echo "To verify the secret was created:"
echo "  kubectl get secret q-workspace-config -n production"
echo ""
echo "Note: The workspace configuration is now stored as a single JSON string"
echo "      in the RELAY_WORKSPACE_CONFIG environment variable."
echo ""
echo "The application will load configuration with this priority:"
echo "  1. RELAY_WORKSPACE_CONFIG env var (JSON string)"
echo "  2. WORKSPACE_CONFIG_FILE env var (file path)"
echo "  3. Default workspace.json file"