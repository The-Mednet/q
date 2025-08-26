-- Fix remaining workspace_id columns that were missed

-- Update pool_statistics table
ALTER TABLE pool_statistics 
  CHANGE COLUMN workspace_id provider_id VARCHAR(255);

-- Update provider_health table  
ALTER TABLE provider_health
  CHANGE COLUMN workspace_id provider_id VARCHAR(255);

-- Update provider_selections table
ALTER TABLE provider_selections
  CHANGE COLUMN workspace_id provider_id VARCHAR(255);

-- Update rate_limit_usage table
ALTER TABLE rate_limit_usage
  CHANGE COLUMN workspace_id provider_id VARCHAR(255);