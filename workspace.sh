#!/bin/sh

# Set your project
#gcloud config set project mednet-q

# Using the admin account, grant org admin to brian@mednetmail.org
ORG_ID=$(gcloud organizations list --format="value(ID)")

# Grant Organization Administrator
gcloud organizations add-iam-policy-binding $ORG_ID \
  --member="user:brian@mednetmail.org" \
  --role="roles/resourcemanager.organizationAdmin"

# Grant Organization Policy Administrator
gcloud organizations add-iam-policy-binding $ORG_ID \
  --member="user:brian@mednetmail.org" \
  --role="roles/orgpolicy.policyAdmin"

# Create the service account
gcloud iam service-accounts create my-service-account \
  --display-name="SMPTY Relay Service Account" \
  --description="Service account for API access"

# Create and download the key
gcloud iam service-accounts keys create ~/key.json \
  --iam-account=smtp-relay@mednet-q.iam.gserviceaccount.com

# Grant it permissions (example: owner role on the project)
gcloud projects add-iam-policy-binding mednet-q \
  --member="serviceAccount:smtp-relay@mednet-q.iam.gserviceaccount.com" \
  --role="roles/owner"

# Get your org ID
ORG_ID=$(gcloud organizations list --format="value(ID)")

# Grant org-level role to the service account
gcloud organizations add-iam-policy-binding $ORG_ID \
  --member="serviceAccount:smtp-relay@mednet-q.iam.gserviceaccount.com" \
  --role="roles/resourcemanager.organizationViewer"

# List service accounts
gcloud iam service-accounts list

# Check the key was created
ls -la ~/key.json
