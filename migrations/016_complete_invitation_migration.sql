-- Complete the invitation tracking migration for recipients table
-- Date: 2025-08-25

-- Step 1: Add invitation_id column (skip if already exists)
-- This will fail silently if column already exists
ALTER TABLE recipients 
    ADD COLUMN invitation_id VARCHAR(255) AFTER provider_id;

-- Step 2: Migrate existing campaign_id data to invitation_id
UPDATE recipients 
SET invitation_id = campaign_id 
WHERE campaign_id IS NOT NULL AND invitation_id IS NULL;

-- Step 3: Drop old columns and their indexes if they exist
ALTER TABLE recipients 
    DROP INDEX IF EXISTS idx_campaign_id,
    DROP INDEX IF EXISTS idx_user_id;

ALTER TABLE recipients
    DROP COLUMN IF EXISTS campaign_id,
    DROP COLUMN IF EXISTS user_id;

-- Step 4: Create index for new invitation_id if it doesn't exist
ALTER TABLE recipients
    ADD INDEX IF NOT EXISTS idx_invitation_id (invitation_id);

-- Verify the migration
SELECT 'Migration completed successfully' as status;