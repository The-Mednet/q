package loadbalancer

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"relay/internal/config"

	"github.com/jmoiron/sqlx"
)

// LoadBalancerImpl implements the LoadBalancer interface
type LoadBalancerImpl struct {
	poolManager       *PoolManager
	capacityTracker   *CapacityTracker
	workspaceProvider WorkspaceProvider
	healthChecker     HealthChecker
	selector          Selector
	db                *sqlx.DB
	config            *LoadBalancerConfig
	
	// Caching and state management
	mu            sync.RWMutex
	lastRefresh   time.Time
	refreshTicker *time.Ticker
	shutdownChan  chan struct{}
}

// NewLoadBalancer creates a new load balancer with all dependencies
func NewLoadBalancer(
	db *sqlx.DB,
	workspaceProvider WorkspaceProvider,
	capacityTracker *CapacityTracker,
	config *LoadBalancerConfig,
) (*LoadBalancerImpl, error) {
	// Defensive programming: validate all required inputs
	if db == nil {
		return nil, fmt.Errorf("database connection is required")
	}
	if workspaceProvider == nil {
		return nil, fmt.Errorf("workspace provider is required")
	}
	if capacityTracker == nil {
		return nil, fmt.Errorf("capacity tracker is required")
	}
	if config == nil {
		log.Printf("Warning: LoadBalancer config is nil, using defaults")
		config = DefaultLoadBalancerConfig()
		if config == nil {
			return nil, fmt.Errorf("failed to create default config")
		}
	}

	// Create pool manager
	poolManager := NewPoolManager(db, config)

	// Create default selector (capacity-weighted)
	selector := NewCapacityWeightedSelector()

	// Create health checker (simplified for now)
	healthChecker := &SimpleHealthChecker{workspaceProvider: workspaceProvider}

	// Create load balancer
	lb := &LoadBalancerImpl{
		poolManager:       poolManager,
		capacityTracker:   capacityTracker,
		workspaceProvider: workspaceProvider,
		healthChecker:     healthChecker,
		selector:          selector,
		db:                db,
		config:            config,
		shutdownChan:      make(chan struct{}),
	}

	// Load pools initially
	if err := lb.RefreshPools(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to load initial pools: %w", err)
	}

	// Start background refresh if enabled
	if config.EnableCaching && config.HealthCheckInterval > 0 {
		lb.startBackgroundRefresh()
	}

	log.Printf("Load balancer initialized with %d pools", len(lb.poolManager.GetAllPools()))
	return lb, nil
}

// SelectWorkspace selects the optimal workspace from available pools for the given sender
func (lb *LoadBalancerImpl) SelectWorkspace(ctx context.Context, senderEmail string) (*config.WorkspaceConfig, error) {
	// Defensive programming: validate load balancer state and inputs
	if lb == nil {
		return nil, NewLoadBalancerError(ErrorTypeInvalidConfig, "load balancer is nil", nil)
	}
	if lb.poolManager == nil {
		return nil, NewLoadBalancerError(ErrorTypeInvalidConfig, "pool manager is nil", nil)
	}
	if senderEmail == "" {
		return nil, NewLoadBalancerError(ErrorTypeInvalidConfig, "sender email is required", nil)
	}

	// Extract domain from sender email
	domain, err := lb.extractDomainFromEmail(senderEmail)
	if err != nil {
		return nil, NewLoadBalancerError(ErrorTypeInvalidConfig, 
			fmt.Sprintf("invalid sender email format: %s", senderEmail), err)
	}

	// Get pools for this domain
	pools, err := lb.poolManager.GetPoolsForDomain(domain)
	if err != nil {
		// No pools found for domain - this is not an error, fall back to direct domain mapping
		log.Printf("No load balancing pools found for domain %s, using direct workspace mapping", domain)
		// Since WorkspaceProvider doesn't have GetWorkspaceByDomain, we'll return nil
		// to indicate that load balancing doesn't apply to this domain
		return nil, nil
	}

	// Select the best pool (for now, use the first enabled pool)
	var selectedPool *LoadBalancingPool
	for _, pool := range pools {
		if pool.Enabled {
			selectedPool = pool
			break
		}
	}

	if selectedPool == nil {
		return nil, NewLoadBalancerError(ErrorTypePoolNotFound, 
			fmt.Sprintf("no enabled pools found for domain: %s", domain), nil)
	}

	// Defensive check for nil selected pool
	if selectedPool == nil {
		return nil, NewLoadBalancerError(ErrorTypePoolNotFound, 
			fmt.Sprintf("selected pool is nil for domain: %s", domain), nil)
	}
	
	// Build candidates from pool workspaces
	candidates, err := lb.buildCandidates(ctx, selectedPool, senderEmail)
	if err != nil {
		return nil, fmt.Errorf("failed to build candidates: %w", err)
	}

	if len(candidates) == 0 {
		return nil, NewLoadBalancerError(ErrorTypeNoHealthyWorkspace, 
			fmt.Sprintf("no eligible workspaces in pool %s", selectedPool.ID), nil)
	}

	// Select workspace using configured algorithm
	selected, err := lb.selector.Select(ctx, candidates, senderEmail)
	if err != nil {
		return nil, fmt.Errorf("failed to select workspace: %w", err)
	}

	// Record the selection for analytics (TODO: implement analytics storage)
	_ = &LoadBalancingSelection{
		PoolID:        selectedPool.ID,
		WorkspaceID:   selected.Workspace.WorkspaceID,
		SenderEmail:   senderEmail,
		SelectedAt:    time.Now(),
		Success:       true, // Assume success for now
		CapacityScore: selected.Capacity.RemainingPercentage,
		SelectionReason: selected.SelectionReason,
	}

	if err := lb.RecordSelection(ctx, selectedPool.ID, selected.Workspace.WorkspaceID, senderEmail, true, selected.Score); err != nil {
		log.Printf("Warning: Failed to record selection: %v", err)
	}

	log.Printf("Selected workspace %s for %s from pool %s (score=%.4f, capacity=%.2f%%)",
		selected.Workspace.WorkspaceID, senderEmail, selectedPool.ID, 
		selected.Score, selected.Capacity.RemainingPercentage*100)

	return selected.Config, nil
}

// buildCandidates creates workspace candidates from a pool
func (lb *LoadBalancerImpl) buildCandidates(ctx context.Context, pool *LoadBalancingPool, senderEmail string) ([]WorkspaceCandidate, error) {
	// Defensive programming: validate inputs
	if lb == nil {
		return nil, fmt.Errorf("load balancer is nil")
	}
	if pool == nil {
		return nil, fmt.Errorf("pool is nil")
	}
	if lb.workspaceProvider == nil {
		return nil, fmt.Errorf("workspace provider is nil")
	}
	if lb.capacityTracker == nil {
		return nil, fmt.Errorf("capacity tracker is nil")
	}
	
	var candidates []WorkspaceCandidate

	for _, poolWorkspace := range pool.Workspaces {
		// Defensive check for empty workspace ID (since PoolWorkspace is a struct, not a pointer)
		if poolWorkspace.WorkspaceID == "" {
			log.Printf("Warning: Skipping workspace with empty ID in pool %s", pool.ID)
			continue
		}
		if !poolWorkspace.Enabled {
			continue
		}

		// Get workspace configuration
		workspaceConfig, err := lb.workspaceProvider.GetWorkspaceByID(poolWorkspace.WorkspaceID)
		if err != nil {
			log.Printf("Warning: Failed to get workspace %s: %v", poolWorkspace.WorkspaceID, err)
			continue
		}
		
		// Defensive check for nil workspace config
		if workspaceConfig == nil {
			log.Printf("Warning: Workspace config is nil for workspace %s", poolWorkspace.WorkspaceID)
			continue
		}

		// Get capacity information
		capacity, err := lb.capacityTracker.GetWorkspaceCapacity(poolWorkspace.WorkspaceID, senderEmail)
		if err != nil {
			log.Printf("Warning: Failed to get capacity for workspace %s: %v", poolWorkspace.WorkspaceID, err)
			continue
		}

		// Get health information
		healthScore := 1.0 // Default to healthy
		if lb.healthChecker != nil {
			healthy, healthErr := lb.healthChecker.IsWorkspaceHealthy(ctx, poolWorkspace.WorkspaceID)
			if healthErr != nil {
				log.Printf("Warning: Health check failed for workspace %s: %v", poolWorkspace.WorkspaceID, healthErr)
				healthScore = 0.5 // Partial health score on error
			} else if !healthy {
				healthScore = 0.0 // Unhealthy
			}
		}

		candidate := WorkspaceCandidate{
			Workspace:   poolWorkspace,
			Config:      workspaceConfig,
			Score:       0, // Will be calculated by selector
			Capacity:    capacity,
			HealthScore: healthScore,
		}

		candidates = append(candidates, candidate)
	}

	return candidates, nil
}

// RecordSelection records the outcome of a selection decision for analytics
func (lb *LoadBalancerImpl) RecordSelection(ctx context.Context, poolID, workspaceID, senderEmail string, success bool, capacityScore float64) error {
	selection := &LoadBalancingSelection{
		PoolID:        poolID,
		WorkspaceID:   workspaceID,
		SenderEmail:   senderEmail,
		SelectedAt:    time.Now(),
		Success:       success,
		CapacityScore: capacityScore,
	}

	return lb.poolManager.RecordSelection(ctx, selection)
}

// GetPoolStatus returns the current status and health of a pool
func (lb *LoadBalancerImpl) GetPoolStatus(poolID string) (*PoolStatus, error) {
	// Defensive programming: validate load balancer state
	if lb == nil {
		return nil, fmt.Errorf("load balancer is nil")
	}
	if lb.poolManager == nil {
		return nil, fmt.Errorf("pool manager is nil")
	}
	
	pool, err := lb.poolManager.GetPool(poolID)
	if err != nil {
		return nil, err
	}
	
	// Defensive check for nil pool
	if pool == nil {
		return nil, fmt.Errorf("pool %s is nil", poolID)
	}

	status := &PoolStatus{
		PoolID:           pool.ID,
		Name:             pool.Name,
		Strategy:         pool.Strategy,
		Enabled:          pool.Enabled,
		TotalWorkspaces:  len(pool.Workspaces),
		HealthyWorkspaces: 0,
		WorkspaceStatuses: make([]WorkspaceStatus, 0, len(pool.Workspaces)),
		DomainPatterns:   pool.DomainPatterns,
	}

	// Check health of each workspace
	ctx := context.Background()
	for _, ws := range pool.Workspaces {
		workspaceStatus := WorkspaceStatus{
			WorkspaceID: ws.WorkspaceID,
			Weight:      ws.Weight,
			Enabled:     ws.Enabled,
			Healthy:     true, // Default assumption
		}

		// Check health if workspace is enabled
		if ws.Enabled && lb.healthChecker != nil {
			healthy, healthErr := lb.healthChecker.IsWorkspaceHealthy(ctx, ws.WorkspaceID)
			workspaceStatus.Healthy = healthy
			if healthErr != nil {
				errStr := healthErr.Error()
				workspaceStatus.LastError = &errStr
			}
		}

		// Get capacity information (using a dummy sender email for status)
		capacity, capacityErr := lb.capacityTracker.GetWorkspaceCapacity(ws.WorkspaceID, "status@example.com")
		if capacityErr == nil {
			workspaceStatus.CapacityInfo = capacity
		}

		if workspaceStatus.Healthy {
			status.HealthyWorkspaces++
		}

		status.WorkspaceStatuses = append(status.WorkspaceStatuses, workspaceStatus)
	}

	return status, nil
}

// RefreshPools reloads pool configuration from the database
func (lb *LoadBalancerImpl) RefreshPools(ctx context.Context) error {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	err := lb.poolManager.LoadPools(ctx)
	if err != nil {
		return fmt.Errorf("failed to refresh pools: %w", err)
	}

	lb.lastRefresh = time.Now()
	log.Printf("Load balancing pools refreshed at %s", lb.lastRefresh.Format(time.RFC3339))
	return nil
}

// SelectFromDefaultPool selects a workspace from the default pool
func (lb *LoadBalancerImpl) SelectFromDefaultPool(ctx context.Context) (*config.WorkspaceConfig, error) {
	// Defensive programming: validate load balancer
	if lb == nil {
		return nil, fmt.Errorf("load balancer is nil")
	}
	
	// Get the default pool ID from configuration
	defaultPoolID := lb.getDefaultPoolID()
	if defaultPoolID == "" {
		return nil, fmt.Errorf("no default pool configured")
	}
	
	// Get the default pool
	pool, err := lb.poolManager.GetPool(defaultPoolID)
	if err != nil {
		return nil, fmt.Errorf("failed to get default pool %s: %w", defaultPoolID, err)
	}
	
	if pool == nil {
		return nil, fmt.Errorf("default pool %s not found", defaultPoolID)
	}
	
	if !pool.Enabled {
		return nil, fmt.Errorf("default pool %s is disabled", defaultPoolID)
	}
	
	// Build candidates from the default pool (use a dummy email since we're selecting from default pool)
	candidates, err := lb.buildCandidates(ctx, pool, "default@example.com")
	if err != nil {
		return nil, fmt.Errorf("failed to build candidates from default pool: %w", err)
	}
	
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no available workspaces in default pool %s", defaultPoolID)
	}
	
	// Select workspace using the pool's strategy
	selected, err := lb.selector.Select(ctx, candidates, string(pool.Strategy))
	if err != nil {
		return nil, fmt.Errorf("failed to select from default pool: %w", err)
	}
	
	if selected == nil {
		return nil, fmt.Errorf("no workspace selected from default pool")
	}
	
	// Get the actual workspace configuration
	workspace, err := lb.workspaceProvider.GetWorkspaceByID(selected.Workspace.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace %s: %w", selected.Workspace.WorkspaceID, err)
	}
	
	log.Printf("Selected workspace %s from default pool %s", selected.Workspace.WorkspaceID, defaultPoolID)
	return workspace, nil
}

// GetPoolManager returns the pool manager instance
func (lb *LoadBalancerImpl) GetPoolManager() *PoolManager {
	return lb.poolManager
}

// getDefaultPoolID retrieves the default pool ID from the database
func (lb *LoadBalancerImpl) getDefaultPoolID() string {
	// Query the database for the default pool
	var defaultPoolID string
	query := `SELECT id FROM load_balancing_pools WHERE is_default = TRUE AND enabled = TRUE LIMIT 1`
	
	err := lb.db.Get(&defaultPoolID, query)
	if err != nil {
		log.Printf("Failed to get default pool from database: %v", err)
		// Fallback to general-pool if database query fails
		return "general-pool"
	}
	
	return defaultPoolID
}

// extractDomainFromEmail extracts domain from an email address
func (lb *LoadBalancerImpl) extractDomainFromEmail(email string) (string, error) {
	atIndex := strings.LastIndex(email, "@")
	if atIndex == -1 || atIndex == len(email)-1 {
		return "", fmt.Errorf("invalid email format: %s", email)
	}

	domain := email[atIndex+1:]
	if domain == "" {
		return "", fmt.Errorf("empty domain in email: %s", email)
	}

	return domain, nil
}

// startBackgroundRefresh starts a goroutine to periodically refresh pools
func (lb *LoadBalancerImpl) startBackgroundRefresh() {
	// Defensive programming: validate state before starting background goroutine
	if lb.config == nil || lb.config.HealthCheckInterval <= 0 {
		log.Printf("Warning: Invalid health check interval, skipping background refresh")
		return
	}
	
	lb.refreshTicker = time.NewTicker(lb.config.HealthCheckInterval)
	
	go func() {
		defer func() {
			// Defensive programming: ensure ticker is stopped on goroutine exit
			if lb.refreshTicker != nil {
				lb.refreshTicker.Stop()
			}
			log.Printf("Background pool refresh goroutine exited")
		}()
		
		for {
			select {
			case <-lb.refreshTicker.C:
				// Create timeout context for each refresh operation
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				if err := lb.RefreshPools(ctx); err != nil {
					log.Printf("Background pool refresh failed: %v", err)
				}
				cancel()
				
			case <-lb.shutdownChan:
				log.Printf("Background pool refresh received shutdown signal")
				return
			}
		}
	}()

	log.Printf("Started background pool refresh every %s", lb.config.HealthCheckInterval)
}

// Shutdown gracefully shuts down the load balancer
func (lb *LoadBalancerImpl) Shutdown(ctx context.Context) error {
	// Defensive programming: validate load balancer state
	if lb == nil {
		return fmt.Errorf("load balancer is nil")
	}
	
	log.Printf("Starting load balancer shutdown...")
	
	// Signal shutdown to background goroutines
	select {
	case <-lb.shutdownChan:
		// Channel already closed
		log.Printf("Load balancer already shutting down")
	default:
		close(lb.shutdownChan)
	}

	// Stop the ticker with proper synchronization
	if lb.refreshTicker != nil {
		lb.refreshTicker.Stop()
		lb.refreshTicker = nil
	}

	// Shutdown pool manager with timeout
	if lb.poolManager != nil {
		if err := lb.poolManager.Shutdown(ctx); err != nil {
			log.Printf("Warning: Pool manager shutdown failed: %v", err)
			return fmt.Errorf("failed to shutdown pool manager: %w", err)
		}
	}

	log.Println("Load balancer shutdown complete")
	return nil
}

// GetAllPools returns all configured pools
func (lb *LoadBalancerImpl) GetAllPools() []*LoadBalancingPool {
	poolsMap := lb.poolManager.GetAllPools()
	pools := make([]*LoadBalancingPool, 0, len(poolsMap))
	for _, pool := range poolsMap {
		pools = append(pools, pool)
	}
	return pools
}

// GetPoolsForDomain returns pools that can handle the specified domain
func (lb *LoadBalancerImpl) GetPoolsForDomain(domain string) ([]*LoadBalancingPool, error) {
	return lb.poolManager.GetPoolsForDomain(domain)
}

// CreatePool creates a new load balancing pool
func (lb *LoadBalancerImpl) CreatePool(ctx context.Context, pool *LoadBalancingPool) error {
	return lb.poolManager.CreatePool(ctx, pool)
}

// UpdatePool updates an existing pool configuration
func (lb *LoadBalancerImpl) UpdatePool(ctx context.Context, pool *LoadBalancingPool) error {
	return lb.poolManager.UpdatePool(ctx, pool)
}

// DeletePool removes a pool
func (lb *LoadBalancerImpl) DeletePool(ctx context.Context, poolID string) error {
	return lb.poolManager.DeletePool(ctx, poolID)
}

// GetSelectionHistory returns recent selection history for a pool
func (lb *LoadBalancerImpl) GetSelectionHistory(ctx context.Context, poolID string, limit int) ([]*LoadBalancingSelection, error) {
	return lb.poolManager.GetSelectionHistory(ctx, poolID, limit)
}

// GetPoolStats returns statistics for a specific pool
func (lb *LoadBalancerImpl) GetPoolStats(ctx context.Context, poolID string, hours int) (map[string]interface{}, error) {
	return lb.poolManager.GetPoolStats(ctx, poolID, hours)
}

// GetLoadBalancerStats returns overall load balancer statistics
func (lb *LoadBalancerImpl) GetLoadBalancerStats(ctx context.Context) map[string]interface{} {
	pools := lb.poolManager.GetAllPools()
	
	totalPools := len(pools)
	enabledPools := 0
	totalWorkspaces := 0
	healthyWorkspaces := 0

	for _, pool := range pools {
		if pool.Enabled {
			enabledPools++
		}
		
		totalWorkspaces += len(pool.Workspaces)
		
		// Count healthy workspaces (simplified check)
		for _, ws := range pool.Workspaces {
			if ws.Enabled {
				// For stats purposes, assume enabled workspaces are healthy
				// In practice, you'd check actual health status
				healthyWorkspaces++
			}
		}
	}

	return map[string]interface{}{
		"total_pools":       totalPools,
		"enabled_pools":     enabledPools,
		"total_workspaces":  totalWorkspaces,
		"healthy_workspaces": healthyWorkspaces,
		"last_refresh":      lb.lastRefresh.Format(time.RFC3339),
		"config": map[string]interface{}{
			"cache_enabled":        lb.config.EnableCaching,
			"cache_ttl":           lb.config.CacheTTL.String(),
			"health_check_interval": lb.config.HealthCheckInterval.String(),
			"selection_timeout":   lb.config.SelectionTimeout.String(),
		},
	}
}

// SimpleHealthChecker provides basic health checking functionality
type SimpleHealthChecker struct {
	workspaceProvider WorkspaceProvider
}

// IsWorkspaceHealthy checks if a workspace is healthy (simplified implementation)
func (shc *SimpleHealthChecker) IsWorkspaceHealthy(ctx context.Context, workspaceID string) (bool, error) {
	// Defensive programming: validate health checker state
	if shc == nil {
		return false, fmt.Errorf("health checker is nil")
	}
	if shc.workspaceProvider == nil {
		return false, fmt.Errorf("workspace provider is nil")
	}
	
	// For now, just check if the workspace exists and is configured properly
	workspace, err := shc.workspaceProvider.GetWorkspaceByID(workspaceID)
	if err != nil {
		return false, err
	}
	
	// Defensive check for nil workspace
	if workspace == nil {
		return false, fmt.Errorf("workspace %s is nil", workspaceID)
	}

	// Check if at least one provider is enabled
	hasEnabledProvider := (workspace.Gmail != nil && workspace.Gmail.Enabled) ||
		(workspace.Mailgun != nil && workspace.Mailgun.Enabled) ||
		(workspace.Mandrill != nil && workspace.Mandrill.Enabled)

	return hasEnabledProvider, nil
}

// GetWorkspaceHealth returns detailed health information (simplified implementation)
func (shc *SimpleHealthChecker) GetWorkspaceHealth(ctx context.Context, workspaceID string) (*WorkspaceHealthInfo, error) {
	healthy, err := shc.IsWorkspaceHealthy(ctx, workspaceID)
	
	info := &WorkspaceHealthInfo{
		WorkspaceID:   workspaceID,
		Healthy:       healthy,
		LastCheckTime: time.Now(),
		ProviderStatus: "unknown",
	}

	if err != nil {
		errStr := err.Error()
		info.LastError = &errStr
	}

	return info, nil
}

// WorkspaceProviderAdapter adapts the workspace manager to the WorkspaceProvider interface
type WorkspaceProviderAdapter struct {
	manager interface {
		GetWorkspaceByID(string) (*config.WorkspaceConfig, error)
		GetWorkspaceByDomain(string) (*config.WorkspaceConfig, error)
		GetAllWorkspaces() map[string]*config.WorkspaceConfig
	}
}

// NewWorkspaceProviderAdapter creates an adapter for the workspace manager
func NewWorkspaceProviderAdapter(manager interface {
	GetWorkspaceByID(string) (*config.WorkspaceConfig, error)
	GetWorkspaceByDomain(string) (*config.WorkspaceConfig, error)
	GetAllWorkspaces() map[string]*config.WorkspaceConfig
}) *WorkspaceProviderAdapter {
	return &WorkspaceProviderAdapter{manager: manager}
}

// GetWorkspaceByID returns a workspace configuration by ID
func (wpa *WorkspaceProviderAdapter) GetWorkspaceByID(workspaceID string) (*config.WorkspaceConfig, error) {
	return wpa.manager.GetWorkspaceByID(workspaceID)
}

// GetAllWorkspaces returns all configured workspaces
func (wpa *WorkspaceProviderAdapter) GetAllWorkspaces() map[string]*config.WorkspaceConfig {
	return wpa.manager.GetAllWorkspaces()
}

// GetWorkspaceByDomain returns a workspace for the given domain
func (wpa *WorkspaceProviderAdapter) GetWorkspaceByDomain(domain string) (*config.WorkspaceConfig, error) {
	return wpa.manager.GetWorkspaceByDomain(domain)
}