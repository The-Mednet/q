-- Add is_default field to load_balancing_pools to mark a pool as the default fallback
ALTER TABLE load_balancing_pools 
ADD COLUMN is_default BOOLEAN DEFAULT FALSE COMMENT 'Whether this pool is the default fallback pool' AFTER enabled;

-- Add index for quick lookup of default pool
ALTER TABLE load_balancing_pools 
ADD INDEX idx_is_default (is_default);

-- Set general-pool as the default if it exists
UPDATE load_balancing_pools SET is_default = TRUE WHERE id = 'general-pool';