-- Migration to replace campaign_id/user_id with invitation tracking fields
-- Date: 2025-08-25

-- Step 1: Add new invitation tracking columns to messages table
ALTER TABLE messages 
    ADD COLUMN invitation_id VARCHAR(255) AFTER metadata,
    ADD COLUMN email_type VARCHAR(100) AFTER invitation_id,
    ADD COLUMN invitation_dispatch_id VARCHAR(255) AFTER email_type;

-- Step 2: Create indexes for new fields
CREATE INDEX idx_invitation_id ON messages(invitation_id);
CREATE INDEX idx_email_type ON messages(email_type);
CREATE INDEX idx_invitation_dispatch_id ON messages(invitation_dispatch_id);

-- Step 3: Migrate existing data (map old fields to new ones)
UPDATE messages 
SET invitation_id = campaign_id,
    email_type = CASE 
        WHEN metadata->>'$.email_type' IS NOT NULL THEN metadata->>'$.email_type'
        ELSE 'invite'
    END
WHERE campaign_id IS NOT NULL;

-- Step 4: Drop old columns and their indexes
ALTER TABLE messages 
    DROP INDEX idx_campaign_id,
    DROP INDEX idx_user_id,
    DROP COLUMN campaign_id,
    DROP COLUMN user_id;

-- Step 5: Update recipients table to use invitation_id instead of campaign_id
ALTER TABLE recipients 
    ADD COLUMN invitation_id VARCHAR(255) AFTER provider_id;

-- Migrate existing campaign_id data to invitation_id
UPDATE recipients 
SET invitation_id = campaign_id 
WHERE campaign_id IS NOT NULL;

-- Drop old campaign_id column and index
ALTER TABLE recipients 
    DROP INDEX idx_campaign_id,
    DROP COLUMN campaign_id,
    DROP COLUMN user_id;

-- Create index for new invitation_id
CREATE INDEX idx_invitation_id ON recipients(invitation_id);

-- Step 6: Update message_recipients table structure if needed
-- No changes needed for this table as it references by message_id

-- Step 7: Update any stored procedures or triggers if they exist
-- (None in the current schema)

-- Add a migration tracking entry (if you have a migrations table)
-- INSERT INTO migrations (name, executed_at) VALUES ('015_invitation_tracking_fields', NOW());