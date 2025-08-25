package loadbalancer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/jmoiron/sqlx"
)

// PoolManager manages load balancing pools and provides access to pool configurations
type PoolManager struct {
	db            *sqlx.DB
	pools         map[string]*LoadBalancingPool // poolID -> pool
	domainToPools map[string][]string          // domain -> poolIDs
	mu            sync.RWMutex
	config        *LoadBalancerConfig
}

// NewPoolManager creates a new pool manager with database connection
func NewPoolManager(db *sqlx.DB, config *LoadBalancerConfig) *PoolManager {
	if config == nil {
		config = DefaultLoadBalancerConfig()
	}
	
	return &PoolManager{
		db:            db,
		pools:         make(map[string]*LoadBalancingPool),
		domainToPools: make(map[string][]string),
		config:        config,
	}
}

// LoadPools loads all enabled pools from the database
func (pm *PoolManager) LoadPools(ctx context.Context) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Clear existing pools
	pm.pools = make(map[string]*LoadBalancingPool)
	pm.domainToPools = make(map[string][]string)

	// Load pool configurations from database
	pools, err := pm.loadPoolsFromDB(ctx)
	if err != nil {
		return fmt.Errorf("failed to load pools from database: %w", err)
	}

	if len(pools) == 0 {
		log.Printf("No load balancing pools found in database")
		return nil
	}

	// Build domain mapping
	for _, pool := range pools {
		pm.pools[pool.ID] = pool
		
		// Map each domain pattern to this pool
		for _, pattern := range pool.DomainPatterns {
			// Clean up the domain pattern (remove @ prefix if present)
			domain := strings.TrimPrefix(pattern, "@")
			if domain != "" {
				pm.domainToPools[domain] = append(pm.domainToPools[domain], pool.ID)
			}
		}
		
		log.Printf("Loaded pool '%s' with %d workspaces and %d domain patterns", 
			pool.ID, len(pool.Workspaces), len(pool.DomainPatterns))
	}

	log.Printf("Successfully loaded %d load balancing pools covering %d domains", 
		len(pm.pools), len(pm.domainToPools))

	return nil
}

// loadPoolsFromDB loads pools and their workspaces from the database
func (pm *PoolManager) loadPoolsFromDB(ctx context.Context) ([]*LoadBalancingPool, error) {
	// Query pools
	query := `
		SELECT id, name, domain_patterns, strategy, enabled, created_at, updated_at
		FROM load_balancing_pools
		WHERE enabled = true
		ORDER BY id`

	rows, err := pm.db.QueryxContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query pools: %w", err)
	}
	defer rows.Close()

	var pools []*LoadBalancingPool
	
	for rows.Next() {
		var pool LoadBalancingPool
		var domainPatternsJSON []byte
		
		err := rows.Scan(
			&pool.ID,
			&pool.Name,
			&domainPatternsJSON,
			&pool.Strategy,
			&pool.Enabled,
			&pool.CreatedAt,
			&pool.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan pool row: %w", err)
		}

		// Parse domain patterns JSON
		if len(domainPatternsJSON) > 0 {
			if err := json.Unmarshal(domainPatternsJSON, &pool.DomainPatterns); err != nil {
				log.Printf("Warning: Failed to parse domain patterns for pool %s: %v", pool.ID, err)
				continue
			}
		}

		// Load workspaces for this pool
		workspaces, err := pm.loadPoolWorkspaces(ctx, pool.ID)
		if err != nil {
			log.Printf("Warning: Failed to load workspaces for pool %s: %v", pool.ID, err)
			continue
		}
		
		pool.Workspaces = workspaces
		pools = append(pools, &pool)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating pool rows: %w", err)
	}

	return pools, nil
}

// loadPoolWorkspaces loads workspace configurations for a specific pool
func (pm *PoolManager) loadPoolWorkspaces(ctx context.Context, poolID string) ([]PoolWorkspace, error) {
	query := `
		SELECT workspace_id, weight, enabled
		FROM pool_workspaces
		WHERE pool_id = ? AND enabled = true
		ORDER BY workspace_id`

	rows, err := pm.db.QueryxContext(ctx, query, poolID)
	if err != nil {
		return nil, fmt.Errorf("failed to query pool workspaces: %w", err)
	}
	defer rows.Close()

	var workspaces []PoolWorkspace
	
	for rows.Next() {
		var ws PoolWorkspace
		err := rows.Scan(&ws.WorkspaceID, &ws.Weight, &ws.Enabled)
		if err != nil {
			return nil, fmt.Errorf("failed to scan workspace row: %w", err)
		}
		
		workspaces = append(workspaces, ws)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating workspace rows: %w", err)
	}

	return workspaces, nil
}

// GetPoolsForDomain returns all pools that can handle the given domain
func (pm *PoolManager) GetPoolsForDomain(domain string) ([]*LoadBalancingPool, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	poolIDs, exists := pm.domainToPools[domain]
	if !exists || len(poolIDs) == 0 {
		return nil, fmt.Errorf("no pools found for domain: %s", domain)
	}

	var pools []*LoadBalancingPool
	for _, poolID := range poolIDs {
		if pool, exists := pm.pools[poolID]; exists && pool.Enabled {
			pools = append(pools, pool)
		}
	}

	if len(pools) == 0 {
		return nil, fmt.Errorf("no enabled pools found for domain: %s", domain)
	}

	return pools, nil
}

// GetAllPools returns all pools
func (pm *PoolManager) GetAllPools() map[string]*LoadBalancingPool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	// Return a copy to prevent external modification
	result := make(map[string]*LoadBalancingPool)
	for k, v := range pm.pools {
		result[k] = v
	}
	return result
}

// GetPool returns a specific pool by ID
func (pm *PoolManager) GetPool(poolID string) (*LoadBalancingPool, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	pool, exists := pm.pools[poolID]
	if !exists {
		return nil, fmt.Errorf("pool not found: %s", poolID)
	}

	// Return a copy to prevent external modification
	poolCopy := *pool
	poolCopy.Workspaces = make([]PoolWorkspace, len(pool.Workspaces))
	copy(poolCopy.Workspaces, pool.Workspaces)
	
	return &poolCopy, nil
}


// CreatePool creates a new load balancing pool in the database
func (pm *PoolManager) CreatePool(ctx context.Context, pool *LoadBalancingPool) error {
	if pool.ID == "" {
		return fmt.Errorf("pool ID is required")
	}
	
	if pool.Name == "" {
		return fmt.Errorf("pool name is required")
	}
	
	if len(pool.DomainPatterns) == 0 {
		return fmt.Errorf("at least one domain pattern is required")
	}
	
	if len(pool.Workspaces) == 0 {
		return fmt.Errorf("at least one workspace is required")
	}

	// Validate strategy
	if !pm.isValidStrategy(pool.Strategy) {
		return fmt.Errorf("invalid selection strategy: %s", pool.Strategy)
	}

	// Start transaction
	tx, err := pm.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Marshal domain patterns to JSON
	domainPatternsJSON, err := json.Marshal(pool.DomainPatterns)
	if err != nil {
		return fmt.Errorf("failed to marshal domain patterns: %w", err)
	}

	// Insert pool
	poolQuery := `
		INSERT INTO load_balancing_pools (id, name, domain_patterns, strategy, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, NOW(), NOW())`

	_, err = tx.ExecContext(ctx, poolQuery, pool.ID, pool.Name, domainPatternsJSON, string(pool.Strategy), pool.Enabled)
	if err != nil {
		return fmt.Errorf("failed to insert pool: %w", err)
	}

	// Insert pool workspaces
	for _, ws := range pool.Workspaces {
		wsQuery := `
			INSERT INTO pool_workspaces (pool_id, workspace_id, weight, enabled)
			VALUES (?, ?, ?, ?)`
		
		_, err = tx.ExecContext(ctx, wsQuery, pool.ID, ws.WorkspaceID, ws.Weight, ws.Enabled)
		if err != nil {
			return fmt.Errorf("failed to insert workspace %s for pool %s: %w", ws.WorkspaceID, pool.ID, err)
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Reload pools to update in-memory cache
	if err := pm.LoadPools(ctx); err != nil {
		log.Printf("Warning: Failed to reload pools after creation: %v", err)
	}

	log.Printf("Successfully created pool '%s' with %d workspaces", pool.ID, len(pool.Workspaces))
	return nil
}

// UpdatePool updates an existing pool's configuration
func (pm *PoolManager) UpdatePool(ctx context.Context, pool *LoadBalancingPool) error {
	if pool.ID == "" {
		return fmt.Errorf("pool ID is required")
	}

	// Validate strategy
	if !pm.isValidStrategy(pool.Strategy) {
		return fmt.Errorf("invalid selection strategy: %s", pool.Strategy)
	}

	// Start transaction
	tx, err := pm.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Marshal domain patterns to JSON
	domainPatternsJSON, err := json.Marshal(pool.DomainPatterns)
	if err != nil {
		return fmt.Errorf("failed to marshal domain patterns: %w", err)
	}

	// Update pool
	poolQuery := `
		UPDATE load_balancing_pools 
		SET name = ?, domain_patterns = ?, strategy = ?, enabled = ?, updated_at = NOW()
		WHERE id = ?`

	result, err := tx.ExecContext(ctx, poolQuery, pool.Name, domainPatternsJSON, string(pool.Strategy), pool.Enabled, pool.ID)
	if err != nil {
		return fmt.Errorf("failed to update pool: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	
	if rowsAffected == 0 {
		return fmt.Errorf("pool not found: %s", pool.ID)
	}

	// Delete existing workspaces
	_, err = tx.ExecContext(ctx, "DELETE FROM pool_workspaces WHERE pool_id = ?", pool.ID)
	if err != nil {
		return fmt.Errorf("failed to delete existing workspaces: %w", err)
	}

	// Insert updated workspaces
	for _, ws := range pool.Workspaces {
		wsQuery := `
			INSERT INTO pool_workspaces (pool_id, workspace_id, weight, enabled)
			VALUES (?, ?, ?, ?)`
		
		_, err = tx.ExecContext(ctx, wsQuery, pool.ID, ws.WorkspaceID, ws.Weight, ws.Enabled)
		if err != nil {
			return fmt.Errorf("failed to insert workspace %s for pool %s: %w", ws.WorkspaceID, pool.ID, err)
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Reload pools to update in-memory cache
	if err := pm.LoadPools(ctx); err != nil {
		log.Printf("Warning: Failed to reload pools after update: %v", err)
	}

	log.Printf("Successfully updated pool '%s'", pool.ID)
	return nil
}

// DeletePool removes a pool and all its workspace associations
func (pm *PoolManager) DeletePool(ctx context.Context, poolID string) error {
	if poolID == "" {
		return fmt.Errorf("pool ID is required")
	}

	// Start transaction
	tx, err := pm.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Delete pool workspaces (foreign key constraint will handle this, but explicit for clarity)
	_, err = tx.ExecContext(ctx, "DELETE FROM pool_workspaces WHERE pool_id = ?", poolID)
	if err != nil {
		return fmt.Errorf("failed to delete pool workspaces: %w", err)
	}

	// Delete pool
	result, err := tx.ExecContext(ctx, "DELETE FROM load_balancing_pools WHERE id = ?", poolID)
	if err != nil {
		return fmt.Errorf("failed to delete pool: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	
	if rowsAffected == 0 {
		return fmt.Errorf("pool not found: %s", poolID)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Reload pools to update in-memory cache
	if err := pm.LoadPools(ctx); err != nil {
		log.Printf("Warning: Failed to reload pools after deletion: %v", err)
	}

	log.Printf("Successfully deleted pool '%s'", poolID)
	return nil
}

// RecordSelection records a pool selection decision for analytics
func (pm *PoolManager) RecordSelection(ctx context.Context, selection *LoadBalancingSelection) error {
	if selection == nil {
		return fmt.Errorf("selection is nil")
	}

	query := `
		INSERT INTO load_balancing_selections (pool_id, workspace_id, sender_email, selected_at, success, capacity_score)
		VALUES (?, ?, ?, ?, ?, ?)`

	_, err := pm.db.ExecContext(ctx, query, 
		selection.PoolID, 
		selection.WorkspaceID, 
		selection.SenderEmail, 
		selection.SelectedAt, 
		selection.Success, 
		selection.CapacityScore)

	if err != nil {
		return fmt.Errorf("failed to record selection: %w", err)
	}

	return nil
}

// GetSelectionHistory returns recent selection history for a pool
func (pm *PoolManager) GetSelectionHistory(ctx context.Context, poolID string, limit int) ([]*LoadBalancingSelection, error) {
	if limit <= 0 {
		limit = 100
	}

	query := `
		SELECT id, pool_id, workspace_id, sender_email, selected_at, success, capacity_score
		FROM load_balancing_selections
		WHERE pool_id = ?
		ORDER BY selected_at DESC
		LIMIT ?`

	rows, err := pm.db.QueryxContext(ctx, query, poolID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query selection history: %w", err)
	}
	defer rows.Close()

	var selections []*LoadBalancingSelection
	
	for rows.Next() {
		var selection LoadBalancingSelection
		err := rows.Scan(
			&selection.ID,
			&selection.PoolID,
			&selection.WorkspaceID,
			&selection.SenderEmail,
			&selection.SelectedAt,
			&selection.Success,
			&selection.CapacityScore,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan selection row: %w", err)
		}
		
		selections = append(selections, &selection)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating selection rows: %w", err)
	}

	return selections, nil
}

// GetPoolStats returns statistics for a specific pool
func (pm *PoolManager) GetPoolStats(ctx context.Context, poolID string, hours int) (map[string]interface{}, error) {
	if hours <= 0 {
		hours = 24
	}

	query := `
		SELECT 
			COUNT(*) as total_selections,
			SUM(CASE WHEN success = true THEN 1 ELSE 0 END) as successful_selections,
			AVG(capacity_score) as avg_capacity_score,
			COUNT(DISTINCT workspace_id) as workspaces_used,
			COUNT(DISTINCT sender_email) as unique_senders
		FROM load_balancing_selections
		WHERE pool_id = ? AND selected_at >= DATE_SUB(NOW(), INTERVAL ? HOUR)`

	var stats struct {
		TotalSelections      int     `db:"total_selections"`
		SuccessfulSelections int     `db:"successful_selections"`
		AvgCapacityScore     float64 `db:"avg_capacity_score"`
		WorkspacesUsed       int     `db:"workspaces_used"`
		UniqueSenders        int     `db:"unique_senders"`
	}

	err := pm.db.GetContext(ctx, &stats, query, poolID, hours)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to query pool stats: %w", err)
	}

	result := map[string]interface{}{
		"pool_id":               poolID,
		"hours":                 hours,
		"total_selections":      stats.TotalSelections,
		"successful_selections": stats.SuccessfulSelections,
		"failed_selections":     stats.TotalSelections - stats.SuccessfulSelections,
		"success_rate":          0.0,
		"avg_capacity_score":    stats.AvgCapacityScore,
		"workspaces_used":       stats.WorkspacesUsed,
		"unique_senders":        stats.UniqueSenders,
	}

	if stats.TotalSelections > 0 {
		result["success_rate"] = float64(stats.SuccessfulSelections) / float64(stats.TotalSelections)
	}

	return result, nil
}

// isValidStrategy checks if the given strategy is supported
func (pm *PoolManager) isValidStrategy(strategy SelectionStrategy) bool {
	switch strategy {
	case StrategyCapacityWeighted, StrategyRoundRobin, StrategyLeastUsed, StrategyRandomWeighted:
		return true
	default:
		return false
	}
}

// ValidatePoolConfiguration validates a pool configuration before saving
func (pm *PoolManager) ValidatePoolConfiguration(pool *LoadBalancingPool) error {
	if pool.ID == "" {
		return fmt.Errorf("pool ID is required")
	}

	if pool.Name == "" {
		return fmt.Errorf("pool name is required")
	}

	if len(pool.DomainPatterns) == 0 {
		return fmt.Errorf("at least one domain pattern is required")
	}

	if len(pool.Workspaces) == 0 {
		return fmt.Errorf("at least one workspace is required")
	}

	if !pm.isValidStrategy(pool.Strategy) {
		return fmt.Errorf("invalid selection strategy: %s", pool.Strategy)
	}

	// Validate workspaces
	for i, ws := range pool.Workspaces {
		if ws.WorkspaceID == "" {
			return fmt.Errorf("workspace %d: workspace ID is required", i)
		}

		if ws.Weight <= 0 {
			return fmt.Errorf("workspace %d: weight must be positive", i)
		}

		if ws.MinCapacityThreshold < 0 || ws.MinCapacityThreshold > 1 {
			return fmt.Errorf("workspace %d: minimum capacity threshold must be between 0 and 1", i)
		}
	}

	// Validate domain patterns
	for i, pattern := range pool.DomainPatterns {
		if pattern == "" {
			return fmt.Errorf("domain pattern %d: pattern cannot be empty", i)
		}

		// Clean pattern and basic validation
		cleanPattern := strings.TrimPrefix(strings.TrimSpace(pattern), "@")
		if cleanPattern == "" {
			return fmt.Errorf("domain pattern %d: invalid pattern '%s'", i, pattern)
		}
	}

	return nil
}

// Shutdown gracefully shuts down the pool manager
func (pm *PoolManager) Shutdown(ctx context.Context) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Clear in-memory data
	pm.pools = make(map[string]*LoadBalancingPool)
	pm.domainToPools = make(map[string][]string)

	log.Println("Pool manager shutdown complete")
	return nil
}