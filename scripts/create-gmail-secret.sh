#!/bin/bash

# Script to create Kubernetes secret for Gmail service accounts
# Usage: ./create-gmail-secret.sh

echo "Creating Gmail Service Account Secret for Kubernetes"
echo "====================================================="
echo ""

# Check if files exist
if [ ! -f "credentials/joinmednet-service-account.json" ]; then
    echo "Error: credentials/joinmednet-service-account.json not found"
    echo "Please place your service account JSON file in the credentials directory"
    exit 1
fi

if [ ! -f "credentials/mednetmail-service-account.json" ]; then
    echo "Error: credentials/mednetmail-service-account.json not found"
    echo "Please place your service account JSON file in the credentials directory"
    exit 1
fi

# Read the JSON files
JOINMEDNET_SA=$(cat credentials/joinmednet-service-account.json)
MEDNETMAIL_SA=$(cat credentials/mednetmail-service-account.json)

# Create the secret
kubectl create secret generic gmail-service-accounts \
  --namespace=production \
  --from-literal=GMAIL_SA_JOINMEDNET="$JOINMEDNET_SA" \
  --from-literal=GMAIL_SA_MEDNETMAIL="$MEDNETMAIL_SA" \
  --dry-run=client -o yaml > gmail-service-accounts-secret.yaml

echo "Secret YAML created: gmail-service-accounts-secret.yaml"
echo ""
echo "To apply the secret to your cluster, run:"
echo "  kubectl apply -f gmail-service-accounts-secret.yaml"
echo ""
echo "To verify the secret was created:"
echo "  kubectl get secret gmail-service-accounts -n production"
echo ""
echo "Note: The service account JSON content is now stored as environment variables"
echo "      that will be read by the application at runtime."