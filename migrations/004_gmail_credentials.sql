-- Migration: Store Gmail service account credentials in database
-- This allows secure storage and management of credentials through the UI

-- Add service_account_json column to store the full credentials JSON
ALTER TABLE workspaces 
ADD COLUMN service_account_json TEXT AFTER provider_config;

-- Add index for faster lookups by provider type
CREATE INDEX idx_workspaces_provider_type ON workspaces(provider_type);

-- Update existing Gmail workspaces to move credentials from provider_config to dedicated column
-- (This is safe as we'll handle both old and new formats in the code)
UPDATE workspaces 
SET service_account_json = JSON_EXTRACT(provider_config, '$.service_account_json')
WHERE provider_type = 'gmail' 
  AND JSON_EXTRACT(provider_config, '$.service_account_json') IS NOT NULL;

-- Add audit columns for tracking credential updates
ALTER TABLE workspaces
ADD COLUMN credentials_updated_at TIMESTAMP NULL DEFAULT NULL AFTER service_account_json,
ADD COLUMN credentials_updated_by VARCHAR(255) NULL DEFAULT NULL AFTER credentials_updated_at;

-- Create a separate table for credential history/audit trail (optional but recommended)
CREATE TABLE IF NOT EXISTS credential_audit_log (
    id INT AUTO_INCREMENT PRIMARY KEY,
    workspace_id VARCHAR(100) NOT NULL,
    action ENUM('uploaded', 'updated', 'deleted', 'rotated') NOT NULL,
    performed_by VARCHAR(255),
    performed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    details JSON,
    INDEX idx_workspace_id (workspace_id),
    INDEX idx_performed_at (performed_at),
    FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
);