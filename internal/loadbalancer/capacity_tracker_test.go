package loadbalancer

import (
	"fmt"
	"testing"
	"time"

	"relay/internal/config"
)

// MockRateLimiter implements the interface needed for testing
type MockRateLimiter struct {
	workspaceStatus map[string]struct {
		sent      int
		remaining int
		resetTime time.Time
	}
	userStatus map[string]struct {
		sent      int
		remaining int
		resetTime time.Time
	}
}

func NewMockRateLimiter() *MockRateLimiter {
	return &MockRateLimiter{
		workspaceStatus: make(map[string]struct {
			sent      int
			remaining int
			resetTime time.Time
		}),
		userStatus: make(map[string]struct {
			sent      int
			remaining int
			resetTime time.Time
		}),
	}
}

func (m *MockRateLimiter) GetWorkspaceStatus(workspaceID string) (sent int, remaining int, resetTime time.Time) {
	if status, exists := m.workspaceStatus[workspaceID]; exists {
		return status.sent, status.remaining, status.resetTime
	}
	return 0, 0, time.Time{}
}

func (m *MockRateLimiter) GetStatus(workspaceID, senderEmail string) (sent int, remaining int, resetTime time.Time) {
	key := workspaceID + ":" + senderEmail
	if status, exists := m.userStatus[key]; exists {
		return status.sent, status.remaining, status.resetTime
	}
	return 0, 0, time.Time{}
}

func (m *MockRateLimiter) SetWorkspaceStatus(workspaceID string, sent, remaining int, resetTime time.Time) {
	m.workspaceStatus[workspaceID] = struct {
		sent      int
		remaining int
		resetTime time.Time
	}{sent, remaining, resetTime}
}

func (m *MockRateLimiter) SetUserStatus(workspaceID, senderEmail string, sent, remaining int, resetTime time.Time) {
	key := workspaceID + ":" + senderEmail
	m.userStatus[key] = struct {
		sent      int
		remaining int
		resetTime time.Time
	}{sent, remaining, resetTime}
}

// MockWorkspaceProvider implements the WorkspaceProvider interface
type MockWorkspaceProvider struct {
	workspaces map[string]*config.WorkspaceConfig
}

func NewMockWorkspaceProvider() *MockWorkspaceProvider {
	return &MockWorkspaceProvider{
		workspaces: make(map[string]*config.WorkspaceConfig),
	}
}

func (m *MockWorkspaceProvider) GetWorkspaceByID(workspaceID string) (*config.WorkspaceConfig, error) {
	if ws, exists := m.workspaces[workspaceID]; exists {
		return ws, nil
	}
	return nil, fmt.Errorf("workspace not found: %s", workspaceID)
}

func (m *MockWorkspaceProvider) GetAllWorkspaces() map[string]*config.WorkspaceConfig {
	result := make(map[string]*config.WorkspaceConfig)
	for k, v := range m.workspaces {
		result[k] = v
	}
	return result
}

func (m *MockWorkspaceProvider) AddWorkspace(ws *config.WorkspaceConfig) {
	m.workspaces[ws.ID] = ws
}

// Adapter to convert MockRateLimiter to the interface expected by CapacityTracker
type RateLimiterAdapter struct {
	mock *MockRateLimiter
}

func (r *RateLimiterAdapter) GetWorkspaceStatus(workspaceID string) (sent int, remaining int, resetTime time.Time) {
	return r.mock.GetWorkspaceStatus(workspaceID)
}

func (r *RateLimiterAdapter) GetStatus(workspaceID, senderEmail string) (sent int, remaining int, resetTime time.Time) {
	return r.mock.GetStatus(workspaceID, senderEmail)
}

func TestCapacityTracker_GetWorkspaceCapacity(t *testing.T) {
	rateLimiter := NewMockRateLimiter()
	workspaceProvider := NewMockWorkspaceProvider()
	adapter := &RateLimiterAdapter{mock: rateLimiter}

	// Create a mock workspace configuration that satisfies the expected interface
	workspace := &config.WorkspaceConfig{
		ID: "test-workspace",
		RateLimits: config.WorkspaceRateLimitConfig{
			WorkspaceDaily: 1000,
			PerUserDaily:   100,
		},
	}
	workspaceProvider.AddWorkspace(workspace)

	tracker := NewCapacityTracker(adapter, workspaceProvider)

	t.Run("BasicCapacityCalculation", func(t *testing.T) {
		// Set up mock data
		resetTime := time.Now().Add(time.Hour)
		rateLimiter.SetWorkspaceStatus("test-workspace", 200, 800, resetTime)
		rateLimiter.SetUserStatus("test-workspace", "user@example.com", 20, 80, resetTime)

		capacity, err := tracker.GetWorkspaceCapacity("test-workspace", "user@example.com")
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if capacity == nil {
			t.Fatal("Expected capacity info, got nil")
		}

		// User limit is more restrictive (80 remaining vs 800 workspace remaining)
		expectedRemaining := 80
		if capacity.EffectiveRemaining != expectedRemaining {
			t.Errorf("Expected effective remaining %d, got: %d", expectedRemaining, capacity.EffectiveRemaining)
		}

		expectedPercentage := 80.0 / 100.0 // 80 remaining out of 100 user limit
		if capacity.RemainingPercentage != expectedPercentage {
			t.Errorf("Expected remaining percentage %.2f, got: %.2f", expectedPercentage, capacity.RemainingPercentage)
		}

		if capacity.WorkspaceRemaining != 800 {
			t.Errorf("Expected workspace remaining 800, got: %d", capacity.WorkspaceRemaining)
		}

		if capacity.UserRemaining != 80 {
			t.Errorf("Expected user remaining 80, got: %d", capacity.UserRemaining)
		}
	})

	t.Run("WorkspaceLimitMoreRestrictive", func(t *testing.T) {
		// Set up scenario where workspace limit is more restrictive
		resetTime := time.Now().Add(time.Hour)
		rateLimiter.SetWorkspaceStatus("test-workspace", 950, 50, resetTime) // Only 50 remaining at workspace level
		rateLimiter.SetUserStatus("test-workspace", "user@example.com", 10, 90, resetTime) // 90 remaining at user level

		capacity, err := tracker.GetWorkspaceCapacity("test-workspace", "user@example.com")
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Workspace limit is more restrictive (50 remaining vs 90 user remaining)
		expectedRemaining := 50
		if capacity.EffectiveRemaining != expectedRemaining {
			t.Errorf("Expected effective remaining %d, got: %d", expectedRemaining, capacity.EffectiveRemaining)
		}

		expectedPercentage := 50.0 / 1000.0 // 50 remaining out of 1000 workspace limit
		if capacity.RemainingPercentage != expectedPercentage {
			t.Errorf("Expected remaining percentage %.3f, got: %.3f", expectedPercentage, capacity.RemainingPercentage)
		}
	})

	t.Run("EmptyWorkspace", func(t *testing.T) {
		_, err := tracker.GetWorkspaceCapacity("", "user@example.com")
		if err == nil {
			t.Error("Expected error for empty workspace ID, got nil")
		}
	})

	t.Run("EmptySenderEmail", func(t *testing.T) {
		_, err := tracker.GetWorkspaceCapacity("test-workspace", "")
		if err == nil {
			t.Error("Expected error for empty sender email, got nil")
		}
	})

	t.Run("NonExistentWorkspace", func(t *testing.T) {
		_, err := tracker.GetWorkspaceCapacity("non-existent", "user@example.com")
		if err == nil {
			t.Error("Expected error for non-existent workspace, got nil")
		}
	})

	t.Run("ZeroLimitsHandling", func(t *testing.T) {
		// Create workspace with no configured limits
		zeroLimitWorkspace := &config.WorkspaceConfig{
			ID: "zero-limits-workspace",
			RateLimits: config.WorkspaceRateLimitConfig{
				// No limits configured
			},
		}
		workspaceProvider.AddWorkspace(zeroLimitWorkspace)

		// No status in rate limiter (simulates fresh workspace)
		capacity, err := tracker.GetWorkspaceCapacity("zero-limits-workspace", "user@example.com")
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Should have some default capacity
		if capacity.EffectiveRemaining <= 0 {
			t.Errorf("Expected positive effective remaining for zero-limits workspace, got: %d", capacity.EffectiveRemaining)
		}
	})

	t.Run("CustomUserLimits", func(t *testing.T) {
		// Create workspace with custom user limits
		customLimitWorkspace := &config.WorkspaceConfig{
			ID: "custom-limits-workspace",
			RateLimits: config.WorkspaceRateLimitConfig{
				WorkspaceDaily: 2000,
				PerUserDaily:   50,
				CustomUserLimits: map[string]int{
					"vip@example.com": 500, // VIP user gets higher limit
				},
			},
		}
		workspaceProvider.AddWorkspace(customLimitWorkspace)

		resetTime := time.Now().Add(time.Hour)
		rateLimiter.SetWorkspaceStatus("custom-limits-workspace", 100, 1900, resetTime)

		// Test VIP user
		rateLimiter.SetUserStatus("custom-limits-workspace", "vip@example.com", 100, 400, resetTime)
		capacity, err := tracker.GetWorkspaceCapacity("custom-limits-workspace", "vip@example.com")
		if err != nil {
			t.Fatalf("Expected no error for VIP user, got: %v", err)
		}

		if capacity.UserLimit != 500 {
			t.Errorf("Expected VIP user limit 500, got: %d", capacity.UserLimit)
		}

		// Test regular user
		rateLimiter.SetUserStatus("custom-limits-workspace", "regular@example.com", 10, 40, resetTime)
		capacity, err = tracker.GetWorkspaceCapacity("custom-limits-workspace", "regular@example.com")
		if err != nil {
			t.Fatalf("Expected no error for regular user, got: %v", err)
		}

		if capacity.UserLimit != 50 {
			t.Errorf("Expected regular user limit 50, got: %d", capacity.UserLimit)
		}
	})
}

func TestCapacityTracker_Caching(t *testing.T) {
	rateLimiter := NewMockRateLimiter()
	workspaceProvider := NewMockWorkspaceProvider()
	adapter := &RateLimiterAdapter{mock: rateLimiter}

	workspace := &config.WorkspaceConfig{
		ID: "cache-test-workspace",
		RateLimits: config.WorkspaceRateLimitConfig{
			WorkspaceDaily: 1000,
			PerUserDaily:   100,
		},
	}
	workspaceProvider.AddWorkspace(workspace)

	// Create tracker with very short cache TTL for testing
	tracker := NewCapacityTrackerWithCache(adapter, workspaceProvider, 100*time.Millisecond)

	t.Run("CacheHit", func(t *testing.T) {
		resetTime := time.Now().Add(time.Hour)
		rateLimiter.SetWorkspaceStatus("cache-test-workspace", 200, 800, resetTime)
		rateLimiter.SetUserStatus("cache-test-workspace", "user@example.com", 20, 80, resetTime)

		// First call - should populate cache
		capacity1, err1 := tracker.GetWorkspaceCapacity("cache-test-workspace", "user@example.com")
		if err1 != nil {
			t.Fatalf("First call failed: %v", err1)
		}

		// Second call immediately - should use cache
		capacity2, err2 := tracker.GetWorkspaceCapacity("cache-test-workspace", "user@example.com")
		if err2 != nil {
			t.Fatalf("Second call failed: %v", err2)
		}

		// Results should be identical
		if capacity1.EffectiveRemaining != capacity2.EffectiveRemaining {
			t.Errorf("Cache miss detected: %d vs %d", capacity1.EffectiveRemaining, capacity2.EffectiveRemaining)
		}
	})

	t.Run("CacheExpiry", func(t *testing.T) {
		resetTime := time.Now().Add(time.Hour)
		rateLimiter.SetWorkspaceStatus("cache-test-workspace", 200, 800, resetTime)
		rateLimiter.SetUserStatus("cache-test-workspace", "user2@example.com", 20, 80, resetTime)

		// First call - should populate cache
		_, err1 := tracker.GetWorkspaceCapacity("cache-test-workspace", "user2@example.com")
		if err1 != nil {
			t.Fatalf("First call failed: %v", err1)
		}

		// Wait for cache to expire
		time.Sleep(150 * time.Millisecond)

		// Update the underlying data
		rateLimiter.SetUserStatus("cache-test-workspace", "user2@example.com", 30, 70, resetTime)

		// Second call after cache expiry - should get updated data
		capacity2, err2 := tracker.GetWorkspaceCapacity("cache-test-workspace", "user2@example.com")
		if err2 != nil {
			t.Fatalf("Second call failed: %v", err2)
		}

		// Should see updated data (70 remaining instead of 80)
		if capacity2.UserRemaining != 70 {
			t.Errorf("Cache didn't expire, expected 70 remaining, got: %d", capacity2.UserRemaining)
		}
	})

	t.Run("ClearCache", func(t *testing.T) {
		resetTime := time.Now().Add(time.Hour)
		rateLimiter.SetWorkspaceStatus("cache-test-workspace", 200, 800, resetTime)
		rateLimiter.SetUserStatus("cache-test-workspace", "user3@example.com", 20, 80, resetTime)

		// Populate cache
		_, err := tracker.GetWorkspaceCapacity("cache-test-workspace", "user3@example.com")
		if err != nil {
			t.Fatalf("Initial call failed: %v", err)
		}

		// Clear cache
		tracker.ClearCache()

		// Update underlying data
		rateLimiter.SetUserStatus("cache-test-workspace", "user3@example.com", 40, 60, resetTime)

		// Call should get updated data immediately
		capacity, err := tracker.GetWorkspaceCapacity("cache-test-workspace", "user3@example.com")
		if err != nil {
			t.Fatalf("Call after clear failed: %v", err)
		}

		if capacity.UserRemaining != 60 {
			t.Errorf("Clear cache didn't work, expected 60 remaining, got: %d", capacity.UserRemaining)
		}
	})

	t.Run("CacheStats", func(t *testing.T) {
		tracker.ClearCache()
		
		stats := tracker.GetCacheStats()
		if stats["total_entries"] != 0 {
			t.Errorf("Expected 0 total entries after clear, got: %v", stats["total_entries"])
		}

		// Add some cache entries
		resetTime := time.Now().Add(time.Hour)
		rateLimiter.SetWorkspaceStatus("cache-test-workspace", 200, 800, resetTime)
		rateLimiter.SetUserStatus("cache-test-workspace", "user4@example.com", 20, 80, resetTime)
		rateLimiter.SetUserStatus("cache-test-workspace", "user5@example.com", 30, 70, resetTime)

		_, _ = tracker.GetWorkspaceCapacity("cache-test-workspace", "user4@example.com")
		_, _ = tracker.GetWorkspaceCapacity("cache-test-workspace", "user5@example.com")

		stats = tracker.GetCacheStats()
		if stats["total_entries"] != 2 {
			t.Errorf("Expected 2 total entries, got: %v", stats["total_entries"])
		}

		if stats["valid_entries"] != 2 {
			t.Errorf("Expected 2 valid entries, got: %v", stats["valid_entries"])
		}

		if stats["expired_entries"] != 0 {
			t.Errorf("Expected 0 expired entries, got: %v", stats["expired_entries"])
		}
	})
}

func TestCapacityTracker_MultipleWorkspaces(t *testing.T) {
	rateLimiter := NewMockRateLimiter()
	workspaceProvider := NewMockWorkspaceProvider()
	adapter := &RateLimiterAdapter{mock: rateLimiter}

	// Add multiple workspaces
	for i := 1; i <= 3; i++ {
		workspace := &config.WorkspaceConfig{
			ID: fmt.Sprintf("workspace%d", i),
			RateLimits: config.WorkspaceRateLimitConfig{
				WorkspaceDaily: 1000 * i,
				PerUserDaily:   100 * i,
			},
		}
		workspaceProvider.AddWorkspace(workspace)
	}

	tracker := NewCapacityTracker(adapter, workspaceProvider)

	t.Run("GetCapacityForMultipleWorkspaces", func(t *testing.T) {
		resetTime := time.Now().Add(time.Hour)
		
		// Set up data for each workspace
		for i := 1; i <= 3; i++ {
			workspaceID := fmt.Sprintf("workspace%d", i)
			rateLimiter.SetWorkspaceStatus(workspaceID, i*100, 1000*i-i*100, resetTime)
			rateLimiter.SetUserStatus(workspaceID, "user@example.com", i*10, 100*i-i*10, resetTime)
		}

		workspaceIDs := []string{"workspace1", "workspace2", "workspace3"}
		capacities, err := tracker.GetCapacityForMultipleWorkspaces(workspaceIDs, "user@example.com")
		if err != nil {
			t.Fatalf("GetCapacityForMultipleWorkspaces failed: %v", err)
		}

		if len(capacities) != 3 {
			t.Errorf("Expected 3 capacity results, got: %d", len(capacities))
		}

		// Verify each workspace has correct capacity
		for i := 1; i <= 3; i++ {
			workspaceID := fmt.Sprintf("workspace%d", i)
			capacity, exists := capacities[workspaceID]
			if !exists {
				t.Errorf("Missing capacity for %s", workspaceID)
				continue
			}

			expectedUserRemaining := 100*i - i*10
			if capacity.UserRemaining != expectedUserRemaining {
				t.Errorf("Workspace %s: expected user remaining %d, got: %d", 
					workspaceID, expectedUserRemaining, capacity.UserRemaining)
			}
		}
	})

	t.Run("HandleNonExistentWorkspace", func(t *testing.T) {
		workspaceIDs := []string{"workspace1", "non-existent", "workspace2"}
		capacities, err := tracker.GetCapacityForMultipleWorkspaces(workspaceIDs, "user@example.com")
		if err != nil {
			t.Fatalf("GetCapacityForMultipleWorkspaces failed: %v", err)
		}

		if len(capacities) != 3 {
			t.Errorf("Expected 3 capacity results, got: %d", len(capacities))
		}

		// Non-existent workspace should have zero capacity
		nonExistentCapacity, exists := capacities["non-existent"]
		if !exists {
			t.Error("Expected entry for non-existent workspace")
		} else {
			if nonExistentCapacity.EffectiveRemaining != 0 {
				t.Errorf("Expected zero capacity for non-existent workspace, got: %d", 
					nonExistentCapacity.EffectiveRemaining)
			}
		}
	})
}

func TestCapacityTracker_UtilityMethods(t *testing.T) {
	rateLimiter := NewMockRateLimiter()
	workspaceProvider := NewMockWorkspaceProvider()
	adapter := &RateLimiterAdapter{mock: rateLimiter}

	workspace := &config.WorkspaceConfig{
		ID: "utility-test-workspace",
		RateLimits: config.WorkspaceRateLimitConfig{
			WorkspaceDaily: 1000,
			PerUserDaily:   100,
		},
	}
	workspaceProvider.AddWorkspace(workspace)

	tracker := NewCapacityTracker(adapter, workspaceProvider)

	t.Run("IsWorkspaceAtCapacity", func(t *testing.T) {
		resetTime := time.Now().Add(time.Hour)
		
		// Set workspace at 95% capacity (50 remaining out of 1000)
		rateLimiter.SetWorkspaceStatus("utility-test-workspace", 950, 50, resetTime)
		rateLimiter.SetUserStatus("utility-test-workspace", "user@example.com", 90, 10, resetTime)

		// User is at 90% capacity (10 remaining out of 100), which is more restrictive
		atCapacity, err := tracker.IsWorkspaceAtCapacity("utility-test-workspace", "user@example.com", 0.8) // 80% threshold
		if err != nil {
			t.Fatalf("IsWorkspaceAtCapacity failed: %v", err)
		}

		if !atCapacity {
			t.Error("Expected workspace to be at capacity (above 80% usage)")
		}

		// Test with higher threshold
		atCapacity, err = tracker.IsWorkspaceAtCapacity("utility-test-workspace", "user@example.com", 0.95) // 95% threshold
		if err != nil {
			t.Fatalf("IsWorkspaceAtCapacity failed: %v", err)
		}

		if atCapacity {
			t.Error("Expected workspace not to be at capacity (below 95% usage)")
		}
	})

	t.Run("PredictTimeToCapacity", func(t *testing.T) {
		resetTime := time.Now().Add(time.Hour)
		
		// Set user with 50 emails remaining
		rateLimiter.SetWorkspaceStatus("utility-test-workspace", 500, 500, resetTime)
		rateLimiter.SetUserStatus("utility-test-workspace", "user@example.com", 50, 50, resetTime)

		// Predict at 10 emails per hour
		timeToCapacity, err := tracker.PredictTimeToCapacity("utility-test-workspace", "user@example.com", 10.0)
		if err != nil {
			t.Fatalf("PredictTimeToCapacity failed: %v", err)
		}

		expectedTime := 5 * time.Hour // 50 remaining / 10 per hour
		if timeToCapacity != expectedTime {
			t.Errorf("Expected time to capacity %s, got: %s", expectedTime, timeToCapacity)
		}

		// Test zero send rate
		timeToCapacity, err = tracker.PredictTimeToCapacity("utility-test-workspace", "user@example.com", 0.0)
		if err != nil {
			t.Fatalf("PredictTimeToCapacity with zero rate failed: %v", err)
		}

		// Should return max duration for zero rate
		if timeToCapacity != time.Duration(int64(^uint64(0)>>1)) {
			t.Error("Expected max duration for zero send rate")
		}
	})

	t.Run("GetWorkspaceUtilization", func(t *testing.T) {
		resetTime := time.Now().Add(2 * time.Hour)
		
		// Set workspace utilization data
		rateLimiter.SetWorkspaceStatus("utility-test-workspace", 300, 700, resetTime)

		utilization, err := tracker.GetWorkspaceUtilization("utility-test-workspace")
		if err != nil {
			t.Fatalf("GetWorkspaceUtilization failed: %v", err)
		}

		if utilization["workspace_id"] != "utility-test-workspace" {
			t.Errorf("Expected workspace_id 'utility-test-workspace', got: %v", utilization["workspace_id"])
		}

		if utilization["sent"] != 300 {
			t.Errorf("Expected sent 300, got: %v", utilization["sent"])
		}

		if utilization["remaining"] != 700 {
			t.Errorf("Expected remaining 700, got: %v", utilization["remaining"])
		}

		expectedUtilization := 300.0 / 1000.0 // 300 sent out of 1000 limit
		if utilization["utilization"] != expectedUtilization {
			t.Errorf("Expected utilization %.3f, got: %v", expectedUtilization, utilization["utilization"])
		}

		expectedUtilizationPct := expectedUtilization * 100
		if utilization["utilization_pct"] != expectedUtilizationPct {
			t.Errorf("Expected utilization_pct %.1f, got: %v", expectedUtilizationPct, utilization["utilization_pct"])
		}
	})
}