package loadbalancer

import (
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"relay/internal/config"
	"relay/internal/queue"
)

// RateLimiterInterface defines the methods needed from a rate limiter
type RateLimiterInterface interface {
	GetWorkspaceStatus(workspaceID string) (sent int, remaining int, resetTime time.Time)
	GetStatus(workspaceID, senderEmail string) (sent int, remaining int, resetTime time.Time)
}

// Compile-time check that WorkspaceAwareRateLimiter implements RateLimiterInterface
var _ RateLimiterInterface = (*queue.WorkspaceAwareRateLimiter)(nil)

// CapacityTracker implements the CapacityProvider interface using the existing rate limiter
type CapacityTracker struct {
	rateLimiter       RateLimiterInterface
	workspaceProvider WorkspaceProvider
	mu                sync.RWMutex
	capacityCache     map[string]*cachedCapacityInfo
	cacheTTL          time.Duration
}

// cachedCapacityInfo represents cached capacity information with expiry
type cachedCapacityInfo struct {
	info      *CapacityInfo
	expiry    time.Time
	workspaceID string
	senderEmail string
}

// NewCapacityTracker creates a new capacity tracker with the existing rate limiter
func NewCapacityTracker(rateLimiter RateLimiterInterface, workspaceProvider WorkspaceProvider) *CapacityTracker {
	if rateLimiter == nil {
		log.Fatal("CapacityTracker: rate limiter cannot be nil")
	}
	if workspaceProvider == nil {
		log.Fatal("CapacityTracker: workspace provider cannot be nil")
	}

	return &CapacityTracker{
		rateLimiter:       rateLimiter,
		workspaceProvider: workspaceProvider,
		capacityCache:     make(map[string]*cachedCapacityInfo),
		cacheTTL:          30 * time.Second, // Cache capacity info for 30 seconds
	}
}

// NewCapacityTrackerWithCache creates a capacity tracker with custom cache TTL
func NewCapacityTrackerWithCache(rateLimiter RateLimiterInterface, workspaceProvider WorkspaceProvider, cacheTTL time.Duration) *CapacityTracker {
	tracker := NewCapacityTracker(rateLimiter, workspaceProvider)
	if cacheTTL > 0 {
		tracker.cacheTTL = cacheTTL
	}
	return tracker
}

// GetWorkspaceCapacity returns capacity information for a workspace and specific sender
func (ct *CapacityTracker) GetWorkspaceCapacity(workspaceID, senderEmail string) (*CapacityInfo, error) {
	if workspaceID == "" {
		return nil, fmt.Errorf("workspace ID cannot be empty")
	}
	if senderEmail == "" {
		return nil, fmt.Errorf("sender email cannot be empty")
	}

	// Check cache first
	cacheKey := fmt.Sprintf("%s:%s", workspaceID, senderEmail)
	if cachedInfo := ct.getCachedCapacity(cacheKey); cachedInfo != nil {
		return cachedInfo, nil
	}

	// Get workspace configuration to determine limits
	workspace, err := ct.workspaceProvider.GetWorkspaceByID(workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace %s: %w", workspaceID, err)
	}

	// Get workspace-level status
	wsSent, wsRemaining, wsResetTime, err := ct.GetWorkspaceStatus(workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace status for %s: %w", workspaceID, err)
	}

	// Get user-level status
	userSent, userRemaining, userResetTime, err := ct.GetUserStatus(workspaceID, senderEmail)
	if err != nil {
		return nil, fmt.Errorf("failed to get user status for %s in %s: %w", senderEmail, workspaceID, err)
	}

	// Calculate capacity information
	capacityInfo := ct.calculateCapacityInfo(workspace, wsSent, wsRemaining, wsResetTime, userSent, userRemaining, userResetTime)

	// Cache the result
	ct.setCachedCapacity(cacheKey, capacityInfo)

	log.Printf("Calculated capacity for %s in %s: %.2f%% remaining (%d/%d effective)",
		senderEmail, workspaceID, capacityInfo.RemainingPercentage*100,
		capacityInfo.EffectiveRemaining, capacityInfo.EffectiveLimit)

	return capacityInfo, nil
}

// GetWorkspaceStatus returns workspace-level capacity information
func (ct *CapacityTracker) GetWorkspaceStatus(workspaceID string) (sent int, remaining int, resetTime time.Time, err error) {
	if workspaceID == "" {
		return 0, 0, time.Time{}, fmt.Errorf("workspace ID cannot be empty")
	}

	sent, remaining, resetTime = ct.rateLimiter.GetWorkspaceStatus(workspaceID)
	
	// Handle case where workspace-level limiting is not configured
	if sent == 0 && remaining == 0 {
		// Get workspace config to check if workspace-level limits exist
		workspace, wsErr := ct.workspaceProvider.GetWorkspaceByID(workspaceID)
		if wsErr != nil {
			return 0, 0, time.Time{}, fmt.Errorf("failed to get workspace config: %w", wsErr)
		}
		
		if workspace.RateLimits.WorkspaceDaily > 0 {
			// Workspace limits are configured but not yet tracked, return the full limit
			remaining = workspace.RateLimits.WorkspaceDaily
			resetTime = time.Now().Add(24 * time.Hour) // Reset tomorrow
		}
	}

	return sent, remaining, resetTime, nil
}

// GetUserStatus returns user-level capacity information within a workspace
func (ct *CapacityTracker) GetUserStatus(workspaceID, senderEmail string) (sent int, remaining int, resetTime time.Time, err error) {
	if workspaceID == "" {
		return 0, 0, time.Time{}, fmt.Errorf("workspace ID cannot be empty")
	}
	if senderEmail == "" {
		return 0, 0, time.Time{}, fmt.Errorf("sender email cannot be empty")
	}

	sent, remaining, resetTime = ct.rateLimiter.GetStatus(workspaceID, senderEmail)

	// Handle case where user hasn't sent emails yet
	if sent == 0 && remaining == 0 {
		// Get workspace config to determine user limits
		workspace, wsErr := ct.workspaceProvider.GetWorkspaceByID(workspaceID)
		if wsErr != nil {
			return 0, 0, time.Time{}, fmt.Errorf("failed to get workspace config: %w", wsErr)
		}

		// Determine user limit based on workspace configuration
		userLimit := ct.getUserLimit(workspace, senderEmail)
		remaining = userLimit
		resetTime = time.Now().Add(24 * time.Hour) // Reset tomorrow
	}

	return sent, remaining, resetTime, nil
}

// calculateCapacityInfo calculates comprehensive capacity information
func (ct *CapacityTracker) calculateCapacityInfo(
	workspace *config.WorkspaceConfig,
	wsSent, wsRemaining int, wsResetTime time.Time,
	userSent, userRemaining int, userResetTime time.Time,
) *CapacityInfo {

	// Calculate limits
	workspaceLimit := workspace.RateLimits.WorkspaceDaily
	if workspaceLimit <= 0 {
		workspaceLimit = wsRemaining + wsSent // Derive from current state
	}
	
	userLimit := ct.getUserLimit(workspace, "")
	if userLimit <= 0 {
		userLimit = userRemaining + userSent // Derive from current state
	}

	// Determine the more restrictive limit (effective limit)
	effectiveRemaining := userRemaining
	effectiveLimit := userLimit
	timeToReset := userResetTime.Sub(time.Now())

	// If workspace limits are configured and more restrictive, use those
	if workspaceLimit > 0 && wsRemaining < userRemaining {
		effectiveRemaining = wsRemaining
		effectiveLimit = workspaceLimit
		timeToReset = wsResetTime.Sub(time.Now())
	}

	// Handle case where both limits are at workspace level (both should be the same)
	if workspaceLimit > 0 && userLimit >= workspaceLimit {
		effectiveRemaining = wsRemaining
		effectiveLimit = workspaceLimit
		timeToReset = wsResetTime.Sub(time.Now())
	}

	// Calculate remaining percentage
	remainingPercentage := 0.0
	if effectiveLimit > 0 {
		remainingPercentage = float64(effectiveRemaining) / float64(effectiveLimit)
	}

	// Ensure values are non-negative
	if effectiveRemaining < 0 {
		effectiveRemaining = 0
		remainingPercentage = 0.0
	}
	if timeToReset < 0 {
		timeToReset = 0
	}

	// Cap remaining percentage at 1.0
	if remainingPercentage > 1.0 {
		remainingPercentage = 1.0
	}

	return &CapacityInfo{
		WorkspaceRemaining:  wsRemaining,
		UserRemaining:      userRemaining,
		WorkspaceLimit:     workspaceLimit,
		UserLimit:          userLimit,
		RemainingPercentage: remainingPercentage,
		TimeToReset:        timeToReset,
		EffectiveRemaining:  effectiveRemaining,
		EffectiveLimit:     effectiveLimit,
	}
}

// getUserLimit determines the effective user limit based on workspace configuration
func (ct *CapacityTracker) getUserLimit(workspace *config.WorkspaceConfig, senderEmail string) int {
	// Check custom user limits first
	if workspace.RateLimits.CustomUserLimits != nil && senderEmail != "" {
		if customLimit, exists := workspace.RateLimits.CustomUserLimits[senderEmail]; exists && customLimit > 0 {
			return customLimit
		}
	}

	// Check per-user workspace limit
	if workspace.RateLimits.PerUserDaily > 0 {
		return workspace.RateLimits.PerUserDaily
	}

	// If no specific user limits, use a fraction of workspace limit or default
	if workspace.RateLimits.WorkspaceDaily > 0 {
		// Default to 10% of workspace limit for individual users
		return int(math.Max(1, float64(workspace.RateLimits.WorkspaceDaily)*0.1))
	}

	// Fallback to a reasonable default
	return 200
}

// getCachedCapacity retrieves cached capacity information if still valid
func (ct *CapacityTracker) getCachedCapacity(cacheKey string) *CapacityInfo {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	cached, exists := ct.capacityCache[cacheKey]
	if !exists {
		return nil
	}

	// Check if cache has expired
	if time.Now().After(cached.expiry) {
		// Cache expired, remove it
		delete(ct.capacityCache, cacheKey)
		return nil
	}

	return cached.info
}

// setCachedCapacity stores capacity information in cache with expiry
func (ct *CapacityTracker) setCachedCapacity(cacheKey string, info *CapacityInfo) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	ct.capacityCache[cacheKey] = &cachedCapacityInfo{
		info:   info,
		expiry: time.Now().Add(ct.cacheTTL),
	}

	// Clean up expired entries periodically
	ct.cleanupExpiredCache()
}

// cleanupExpiredCache removes expired entries from cache (called with write lock)
func (ct *CapacityTracker) cleanupExpiredCache() {
	now := time.Now()
	for key, cached := range ct.capacityCache {
		if now.After(cached.expiry) {
			delete(ct.capacityCache, key)
		}
	}
}

// GetCapacityForMultipleWorkspaces gets capacity info for multiple workspaces efficiently
func (ct *CapacityTracker) GetCapacityForMultipleWorkspaces(workspaceIDs []string, senderEmail string) (map[string]*CapacityInfo, error) {
	if len(workspaceIDs) == 0 {
		return make(map[string]*CapacityInfo), nil
	}

	results := make(map[string]*CapacityInfo)
	
	for _, workspaceID := range workspaceIDs {
		capacity, err := ct.GetWorkspaceCapacity(workspaceID, senderEmail)
		if err != nil {
			log.Printf("Warning: Failed to get capacity for workspace %s: %v", workspaceID, err)
			// Create a zero-capacity entry to indicate unavailability
			results[workspaceID] = &CapacityInfo{
				WorkspaceRemaining:  0,
				UserRemaining:      0,
				WorkspaceLimit:     0,
				UserLimit:          0,
				RemainingPercentage: 0.0,
				TimeToReset:        24 * time.Hour,
				EffectiveRemaining:  0,
				EffectiveLimit:     0,
			}
			continue
		}
		results[workspaceID] = capacity
	}

	return results, nil
}

// RefreshCapacityCache forces a refresh of cached capacity information
func (ct *CapacityTracker) RefreshCapacityCache(workspaceID, senderEmail string) error {
	cacheKey := fmt.Sprintf("%s:%s", workspaceID, senderEmail)
	
	ct.mu.Lock()
	delete(ct.capacityCache, cacheKey)
	ct.mu.Unlock()

	// Fetch fresh capacity information
	_, err := ct.GetWorkspaceCapacity(workspaceID, senderEmail)
	return err
}

// ClearCache clears all cached capacity information
func (ct *CapacityTracker) ClearCache() {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	ct.capacityCache = make(map[string]*cachedCapacityInfo)
	log.Println("Capacity cache cleared")
}

// GetCacheStats returns statistics about the capacity cache
func (ct *CapacityTracker) GetCacheStats() map[string]interface{} {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	totalEntries := len(ct.capacityCache)
	expiredEntries := 0
	now := time.Now()

	for _, cached := range ct.capacityCache {
		if now.After(cached.expiry) {
			expiredEntries++
		}
	}

	return map[string]interface{}{
		"total_entries":   totalEntries,
		"expired_entries": expiredEntries,
		"valid_entries":   totalEntries - expiredEntries,
		"cache_ttl":       ct.cacheTTL.String(),
	}
}

// SetCacheTTL updates the cache time-to-live duration
func (ct *CapacityTracker) SetCacheTTL(ttl time.Duration) {
	if ttl > 0 {
		ct.cacheTTL = ttl
		log.Printf("Capacity cache TTL updated to %s", ttl)
	}
}

// GetCacheTTL returns the current cache TTL
func (ct *CapacityTracker) GetCacheTTL() time.Duration {
	return ct.cacheTTL
}

// IsWorkspaceAtCapacity checks if a workspace has reached capacity limits
func (ct *CapacityTracker) IsWorkspaceAtCapacity(workspaceID, senderEmail string, threshold float64) (bool, error) {
	if threshold <= 0 {
		threshold = 0.95 // Default to 95% capacity
	}
	if threshold > 1.0 {
		threshold = 1.0
	}

	capacity, err := ct.GetWorkspaceCapacity(workspaceID, senderEmail)
	if err != nil {
		return false, fmt.Errorf("failed to get capacity: %w", err)
	}

	// Check if remaining capacity is below threshold
	return capacity.RemainingPercentage < (1.0 - threshold), nil
}

// PredictTimeToCapacity estimates when a workspace will reach capacity based on current usage
func (ct *CapacityTracker) PredictTimeToCapacity(workspaceID, senderEmail string, sendRate float64) (time.Duration, error) {
	capacity, err := ct.GetWorkspaceCapacity(workspaceID, senderEmail)
	if err != nil {
		return 0, fmt.Errorf("failed to get capacity: %w", err)
	}

	if capacity.EffectiveRemaining <= 0 {
		return 0, nil // Already at capacity
	}

	if sendRate <= 0 {
		return time.Duration(math.MaxInt64), nil // Never reach capacity at zero rate
	}

	// Calculate time to exhaust remaining capacity
	hoursToCapacity := float64(capacity.EffectiveRemaining) / sendRate
	return time.Duration(hoursToCapacity * float64(time.Hour)), nil
}

// GetWorkspaceUtilization returns utilization statistics for a workspace
func (ct *CapacityTracker) GetWorkspaceUtilization(workspaceID string) (map[string]interface{}, error) {
	workspace, err := ct.workspaceProvider.GetWorkspaceByID(workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace: %w", err)
	}

	wsSent, wsRemaining, wsResetTime, err := ct.GetWorkspaceStatus(workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace status: %w", err)
	}

	workspaceLimit := workspace.RateLimits.WorkspaceDaily
	if workspaceLimit <= 0 {
		workspaceLimit = wsSent + wsRemaining
	}

	utilization := 0.0
	if workspaceLimit > 0 {
		utilization = float64(wsSent) / float64(workspaceLimit)
	}

	return map[string]interface{}{
		"provider_id":     workspaceID,
		"sent":            wsSent,
		"remaining":       wsRemaining,
		"limit":           workspaceLimit,
		"utilization":     utilization,
		"utilization_pct": utilization * 100,
		"time_to_reset":   wsResetTime.Sub(time.Now()).String(),
	}, nil
}