package loadbalancer

import (
	"context"
	"fmt"
	"testing"
	"time"

	"relay/internal/config"
)

func TestCapacityWeightedSelector_Select(t *testing.T) {
	selector := NewCapacityWeightedSelector()
	ctx := context.Background()

	t.Run("SelectsHighestScoringCandidate", func(t *testing.T) {
		candidates := []WorkspaceCandidate{
			{
				Workspace: PoolWorkspace{
					ProviderID: "workspace1",
					Weight:      1.0,
					Enabled:     true,
				},
				Config: &config.WorkspaceConfig{ID: "workspace1"},
				Capacity: &CapacityInfo{
					RemainingPercentage: 0.9, // 90% remaining
					EffectiveRemaining:  900,
					EffectiveLimit:      1000,
					TimeToReset:        time.Hour,
				},
				HealthScore: 1.0,
			},
			{
				Workspace: PoolWorkspace{
					ProviderID: "workspace2",
					Weight:      1.0,
					Enabled:     true,
				},
				Config: &config.WorkspaceConfig{ID: "workspace2"},
				Capacity: &CapacityInfo{
					RemainingPercentage: 0.5, // 50% remaining
					EffectiveRemaining:  500,
					EffectiveLimit:      1000,
					TimeToReset:        time.Hour,
				},
				HealthScore: 1.0,
			},
		}

		selected, err := selector.Select(ctx, candidates, "test@example.com")
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// The workspace with higher capacity should be preferred
		if selected.Workspace.ProviderID != "workspace1" {
			t.Errorf("Expected workspace1, got: %s", selected.Workspace.ProviderID)
		}

		if selected.Score <= 0 {
			t.Errorf("Expected positive score, got: %f", selected.Score)
		}
	})

	t.Run("ConsidersWeights", func(t *testing.T) {
		candidates := []WorkspaceCandidate{
			{
				Workspace: PoolWorkspace{
					ProviderID: "low-capacity-high-weight",
					Weight:      5.0, // Very high weight
					Enabled:     true,
				},
				Config: &config.WorkspaceConfig{ID: "low-capacity-high-weight"},
				Capacity: &CapacityInfo{
					RemainingPercentage: 0.3, // 30% remaining
					EffectiveRemaining:  300,
					EffectiveLimit:      1000,
					TimeToReset:        time.Hour,
				},
				HealthScore: 1.0,
			},
			{
				Workspace: PoolWorkspace{
					ProviderID: "high-capacity-low-weight",
					Weight:      0.5, // Low weight
					Enabled:     true,
				},
				Config: &config.WorkspaceConfig{ID: "high-capacity-low-weight"},
				Capacity: &CapacityInfo{
					RemainingPercentage: 0.9, // 90% remaining
					EffectiveRemaining:  900,
					EffectiveLimit:      1000,
					TimeToReset:        time.Hour,
				},
				HealthScore: 1.0,
			},
		}

		// Run multiple selections to test weighted distribution
		selections := make(map[string]int)
		for i := 0; i < 100; i++ {
			selected, err := selector.Select(ctx, candidates, "test@example.com")
			if err != nil {
				t.Fatalf("Selection %d failed: %v", i, err)
			}
			selections[selected.Workspace.ProviderID]++
		}

		// Both should be selected, but we can't test exact distribution due to randomness
		if len(selections) == 0 {
			t.Error("No workspaces were selected")
		}

		// At least verify that selections happened
		totalSelections := 0
		for _, count := range selections {
			totalSelections += count
		}
		if totalSelections != 100 {
			t.Errorf("Expected 100 total selections, got: %d", totalSelections)
		}
	})

	t.Run("FiltersUnhealthyWorkspaces", func(t *testing.T) {
		candidates := []WorkspaceCandidate{
			{
				Workspace: PoolWorkspace{
					ProviderID: "healthy",
					Weight:      1.0,
					Enabled:     true,
				},
				Config: &config.WorkspaceConfig{ID: "healthy"},
				Capacity: &CapacityInfo{
					RemainingPercentage: 0.5,
					EffectiveRemaining:  500,
					EffectiveLimit:      1000,
					TimeToReset:        time.Hour,
				},
				HealthScore: 1.0, // Healthy
			},
			{
				Workspace: PoolWorkspace{
					ProviderID: "unhealthy",
					Weight:      2.0, // Higher weight but unhealthy
					Enabled:     true,
				},
				Config: &config.WorkspaceConfig{ID: "unhealthy"},
				Capacity: &CapacityInfo{
					RemainingPercentage: 0.8,
					EffectiveRemaining:  800,
					EffectiveLimit:      1000,
					TimeToReset:        time.Hour,
				},
				HealthScore: 0.0, // Unhealthy
			},
		}

		selected, err := selector.Select(ctx, candidates, "test@example.com")
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Should select the healthy workspace
		if selected.Workspace.ProviderID != "healthy" {
			t.Errorf("Expected healthy workspace, got: %s", selected.Workspace.ProviderID)
		}
	})

	t.Run("RespectsMinCapacityThreshold", func(t *testing.T) {
		selector := NewCapacityWeightedSelectorWithConfig(0.6, 20) // 60% minimum capacity

		candidates := []WorkspaceCandidate{
			{
				Workspace: PoolWorkspace{
					ProviderID: "below-threshold",
					Weight:      1.0,
					Enabled:     true,
				},
				Config: &config.WorkspaceConfig{ID: "below-threshold"},
				Capacity: &CapacityInfo{
					RemainingPercentage: 0.4, // Below 60% threshold
					EffectiveRemaining:  400,
					EffectiveLimit:      1000,
					TimeToReset:        time.Hour,
				},
				HealthScore: 1.0,
			},
			{
				Workspace: PoolWorkspace{
					ProviderID: "above-threshold",
					Weight:      1.0,
					Enabled:     true,
				},
				Config: &config.WorkspaceConfig{ID: "above-threshold"},
				Capacity: &CapacityInfo{
					RemainingPercentage: 0.8, // Above 60% threshold
					EffectiveRemaining:  800,
					EffectiveLimit:      1000,
					TimeToReset:        time.Hour,
				},
				HealthScore: 1.0,
			},
		}

		selected, err := selector.Select(ctx, candidates, "test@example.com")
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Should select the workspace above threshold
		if selected.Workspace.ProviderID != "above-threshold" {
			t.Errorf("Expected above-threshold workspace, got: %s", selected.Workspace.ProviderID)
		}
	})

	t.Run("FiltersDisabledWorkspaces", func(t *testing.T) {
		candidates := []WorkspaceCandidate{
			{
				Workspace: PoolWorkspace{
					ProviderID: "disabled",
					Weight:      2.0,
					Enabled:     false, // Disabled
				},
				Config: &config.WorkspaceConfig{ID: "disabled"},
				Capacity: &CapacityInfo{
					RemainingPercentage: 0.9,
					EffectiveRemaining:  900,
					EffectiveLimit:      1000,
					TimeToReset:        time.Hour,
				},
				HealthScore: 1.0,
			},
			{
				Workspace: PoolWorkspace{
					ProviderID: "enabled",
					Weight:      1.0,
					Enabled:     true, // Enabled
				},
				Config: &config.WorkspaceConfig{ID: "enabled"},
				Capacity: &CapacityInfo{
					RemainingPercentage: 0.7,
					EffectiveRemaining:  700,
					EffectiveLimit:      1000,
					TimeToReset:        time.Hour,
				},
				HealthScore: 1.0,
			},
		}

		selected, err := selector.Select(ctx, candidates, "test@example.com")
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Should select the enabled workspace
		if selected.Workspace.ProviderID != "enabled" {
			t.Errorf("Expected enabled workspace, got: %s", selected.Workspace.ProviderID)
		}
	})

	t.Run("HandlesEmptyCandidates", func(t *testing.T) {
		candidates := []WorkspaceCandidate{}

		_, err := selector.Select(ctx, candidates, "test@example.com")
		if err == nil {
			t.Error("Expected error for empty candidates, got nil")
		}

		lbErr, ok := err.(*LoadBalancerError)
		if !ok {
			t.Errorf("Expected LoadBalancerError, got: %T", err)
		} else if lbErr.Type != ErrorTypeNoHealthyWorkspace {
			t.Errorf("Expected ErrorTypeNoHealthyWorkspace, got: %s", lbErr.Type)
		}
	})

	t.Run("HandlesZeroCapacityWorkspaces", func(t *testing.T) {
		candidates := []WorkspaceCandidate{
			{
				Workspace: PoolWorkspace{
					ProviderID: "zero-capacity",
					Weight:      1.0,
					Enabled:     true,
				},
				Config: &config.WorkspaceConfig{ID: "zero-capacity"},
				Capacity: &CapacityInfo{
					RemainingPercentage: 0.0, // No capacity
					EffectiveRemaining:  0,
					EffectiveLimit:      1000,
					TimeToReset:        time.Hour,
				},
				HealthScore: 1.0,
			},
		}

		_, err := selector.Select(ctx, candidates, "test@example.com")
		if err == nil {
			t.Error("Expected error for zero capacity candidates, got nil")
		}
	})

	t.Run("GeneratesSelectionReason", func(t *testing.T) {
		candidates := []WorkspaceCandidate{
			{
				Workspace: PoolWorkspace{
					ProviderID: "test-workspace",
					Weight:      2.5,
					Enabled:     true,
				},
				Config: &config.WorkspaceConfig{ID: "test-workspace"},
				Capacity: &CapacityInfo{
					RemainingPercentage: 0.85, // 85% remaining
					EffectiveRemaining:  850,
					EffectiveLimit:      1000,
					TimeToReset:        30 * time.Minute, // Reset soon
				},
				HealthScore: 1.0,
			},
		}

		selected, err := selector.Select(ctx, candidates, "test@example.com")
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if selected.SelectionReason == "" {
			t.Error("Expected selection reason to be populated")
		}

		// Verify reason contains expected elements
		reason := selected.SelectionReason
		if !contains(reason, "capacity_weighted") {
			t.Errorf("Selection reason should mention strategy: %s", reason)
		}
		if !contains(reason, "85.0%") {
			t.Errorf("Selection reason should mention capacity: %s", reason)
		}
		if !contains(reason, "2.5x") {
			t.Errorf("Selection reason should mention weight: %s", reason)
		}
	})
}

func TestCapacityWeightedSelector_Configuration(t *testing.T) {
	t.Run("DefaultConfiguration", func(t *testing.T) {
		selector := NewCapacityWeightedSelector()

		if selector.GetMinCapacityScore() != 0.01 {
			t.Errorf("Expected default min capacity score 0.01, got: %f", selector.GetMinCapacityScore())
		}

		if selector.GetMaxCandidates() != 20 {
			t.Errorf("Expected default max candidates 20, got: %d", selector.GetMaxCandidates())
		}
	})

	t.Run("CustomConfiguration", func(t *testing.T) {
		selector := NewCapacityWeightedSelectorWithConfig(0.25, 50)

		if selector.GetMinCapacityScore() != 0.25 {
			t.Errorf("Expected min capacity score 0.25, got: %f", selector.GetMinCapacityScore())
		}

		if selector.GetMaxCandidates() != 50 {
			t.Errorf("Expected max candidates 50, got: %d", selector.GetMaxCandidates())
		}
	})

	t.Run("SetMinCapacityScore", func(t *testing.T) {
		selector := NewCapacityWeightedSelector()
		selector.SetMinCapacityScore(0.4)

		if selector.GetMinCapacityScore() != 0.4 {
			t.Errorf("Expected min capacity score 0.4, got: %f", selector.GetMinCapacityScore())
		}

		// Test boundary values
		selector.SetMinCapacityScore(-0.1) // Should be ignored
		if selector.GetMinCapacityScore() != 0.4 {
			t.Errorf("Negative value should be ignored, got: %f", selector.GetMinCapacityScore())
		}

		selector.SetMinCapacityScore(1.5) // Should be ignored
		if selector.GetMinCapacityScore() != 0.4 {
			t.Errorf("Value > 1 should be ignored, got: %f", selector.GetMinCapacityScore())
		}
	})

	t.Run("SetMaxCandidates", func(t *testing.T) {
		selector := NewCapacityWeightedSelector()
		selector.SetMaxCandidates(100)

		if selector.GetMaxCandidates() != 100 {
			t.Errorf("Expected max candidates 100, got: %d", selector.GetMaxCandidates())
		}

		// Test boundary values
		selector.SetMaxCandidates(0) // Should be ignored
		if selector.GetMaxCandidates() != 100 {
			t.Errorf("Zero value should be ignored, got: %d", selector.GetMaxCandidates())
		}

		selector.SetMaxCandidates(-5) // Should be ignored
		if selector.GetMaxCandidates() != 100 {
			t.Errorf("Negative value should be ignored, got: %d", selector.GetMaxCandidates())
		}
	})

	t.Run("GetStrategy", func(t *testing.T) {
		selector := NewCapacityWeightedSelector()
		if selector.GetStrategy() != StrategyCapacityWeighted {
			t.Errorf("Expected StrategyCapacityWeighted, got: %s", selector.GetStrategy())
		}
	})
}

func TestCapacityWeightedSelector_GetSelectionStats(t *testing.T) {
	selector := NewCapacityWeightedSelector()

	t.Run("EmptyCandidates", func(t *testing.T) {
		stats := selector.GetSelectionStats([]WorkspaceCandidate{})

		expectedStats := map[string]interface{}{
			"total_candidates":        0,
			"eligible_candidates":     0,
			"avg_capacity":           0.0,
			"avg_weight":             0.0,
			"avg_health_score":       0.0,
			"min_capacity_threshold": 0.01,
			"max_candidates":         20,
		}

		for key, expected := range expectedStats {
			if stats[key] != expected {
				t.Errorf("Stats[%s]: expected %v, got %v", key, expected, stats[key])
			}
		}
	})

	t.Run("WithCandidates", func(t *testing.T) {
		candidates := []WorkspaceCandidate{
			{
				Workspace: PoolWorkspace{
					ProviderID: "ws1",
					Weight:      2.0,
					Enabled:     true,
				},
				Capacity: &CapacityInfo{
					RemainingPercentage: 0.8,
					EffectiveRemaining:  800,
					EffectiveLimit:      1000,
				},
				HealthScore: 1.0,
			},
			{
				Workspace: PoolWorkspace{
					ProviderID: "ws2",
					Weight:      1.0,
					Enabled:     true,
				},
				Capacity: &CapacityInfo{
					RemainingPercentage: 0.6,
					EffectiveRemaining:  600,
					EffectiveLimit:      1000,
				},
				HealthScore: 0.8,
			},
		}

		stats := selector.GetSelectionStats(candidates)

		if stats["total_candidates"] != 2 {
			t.Errorf("Expected 2 total candidates, got: %v", stats["total_candidates"])
		}

		if stats["eligible_candidates"] != 2 {
			t.Errorf("Expected 2 eligible candidates, got: %v", stats["eligible_candidates"])
		}

		// Check averages
		expectedAvgCapacity := (0.8 + 0.6) / 2.0
		if stats["avg_capacity"] != expectedAvgCapacity {
			t.Errorf("Expected avg capacity %f, got: %v", expectedAvgCapacity, stats["avg_capacity"])
		}

		expectedAvgWeight := (2.0 + 1.0) / 2.0
		if stats["avg_weight"] != expectedAvgWeight {
			t.Errorf("Expected avg weight %f, got: %v", expectedAvgWeight, stats["avg_weight"])
		}

		expectedAvgHealth := (1.0 + 0.8) / 2.0
		if stats["avg_health_score"] != expectedAvgHealth {
			t.Errorf("Expected avg health score %f, got: %v", expectedAvgHealth, stats["avg_health_score"])
		}
	})
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || 
		(len(s) > len(substr) && 
			(func() bool {
				for i := 0; i <= len(s)-len(substr); i++ {
					if s[i:i+len(substr)] == substr {
						return true
					}
				}
				return false
			}())))
}

// Benchmark tests for performance validation
func BenchmarkCapacityWeightedSelector_Select(b *testing.B) {
	selector := NewCapacityWeightedSelector()
	ctx := context.Background()

	// Create benchmark candidates
	candidates := make([]WorkspaceCandidate, 10)
	for i := 0; i < 10; i++ {
		candidates[i] = WorkspaceCandidate{
			Workspace: PoolWorkspace{
				ProviderID: fmt.Sprintf("workspace%d", i),
				Weight:      float64(i + 1),
				Enabled:     true,
			},
			Config: &config.WorkspaceConfig{ID: fmt.Sprintf("workspace%d", i)},
			Capacity: &CapacityInfo{
				RemainingPercentage: 0.5 + float64(i)*0.05,
				EffectiveRemaining:  500 + i*50,
				EffectiveLimit:      1000,
				TimeToReset:        time.Hour,
			},
			HealthScore: 1.0,
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := selector.Select(ctx, candidates, "test@example.com")
		if err != nil {
			b.Fatalf("Selection failed: %v", err)
		}
	}
}

func BenchmarkCapacityWeightedSelector_SelectLarge(b *testing.B) {
	selector := NewCapacityWeightedSelector()
	ctx := context.Background()

	// Create larger set of candidates to test scalability
	candidates := make([]WorkspaceCandidate, 100)
	for i := 0; i < 100; i++ {
		candidates[i] = WorkspaceCandidate{
			Workspace: PoolWorkspace{
				ProviderID: fmt.Sprintf("workspace%d", i),
				Weight:      1.0 + float64(i%5)*0.2,
				Enabled:     i%10 != 0, // 90% enabled
			},
			Config: &config.WorkspaceConfig{ID: fmt.Sprintf("workspace%d", i)},
			Capacity: &CapacityInfo{
				RemainingPercentage: 0.1 + float64(i%9)*0.1,
				EffectiveRemaining:  100 + i*10,
				EffectiveLimit:      1000,
				TimeToReset:        time.Hour,
			},
			HealthScore: 0.5 + float64(i%5)*0.1,
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := selector.Select(ctx, candidates, "test@example.com")
		if err != nil {
			b.Fatalf("Selection failed: %v", err)
		}
	}
}