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
		config = DefaultLoadBalancerConfig()
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
	var candidates []WorkspaceCandidate

	for _, poolWorkspace := range pool.Workspaces {
		if !poolWorkspace.Enabled {
			continue
		}

		// Get workspace configuration
		workspaceConfig, err := lb.workspaceProvider.GetWorkspaceByID(poolWorkspace.WorkspaceID)
		if err != nil {
			log.Printf("Warning: Failed to get workspace %s: %v", poolWorkspace.WorkspaceID, err)
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
	pool, err := lb.poolManager.GetPool(poolID)
	if err != nil {
		return nil, err
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
	lb.refreshTicker = time.NewTicker(lb.config.HealthCheckInterval)
	
	go func() {
		for {
			select {
			case <-lb.refreshTicker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				if err := lb.RefreshPools(ctx); err != nil {
					log.Printf("Background pool refresh failed: %v", err)
				}
				cancel()
			case <-lb.shutdownChan:
				return
			}
		}
	}()

	log.Printf("Started background pool refresh every %s", lb.config.HealthCheckInterval)
}

// Shutdown gracefully shuts down the load balancer
func (lb *LoadBalancerImpl) Shutdown(ctx context.Context) error {
	close(lb.shutdownChan)

	if lb.refreshTicker != nil {
		lb.refreshTicker.Stop()
	}

	// Shutdown pool manager
	if err := lb.poolManager.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown pool manager: %w", err)
	}

	log.Println("Load balancer shutdown complete")
	return nil
}

// GetAllPools returns all configured pools
func (lb *LoadBalancerImpl) GetAllPools() []*LoadBalancingPool {
	return lb.poolManager.GetAllPools()
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
	// For now, just check if the workspace exists and is configured properly
	workspace, err := shc.workspaceProvider.GetWorkspaceByID(workspaceID)
	if err != nil {
		return false, err
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