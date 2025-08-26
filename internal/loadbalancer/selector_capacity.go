package loadbalancer

import (
	"context"
	"fmt"
	"log"
	"math"
	"math/rand"
	"sort"
	"time"
)

// CapacityWeightedSelector implements capacity-weighted workspace selection
type CapacityWeightedSelector struct {
	rng              *rand.Rand
	minCapacityScore float64 // Minimum capacity score to be considered eligible
	maxCandidates    int     // Maximum number of candidates to consider
}

// NewCapacityWeightedSelector creates a new capacity-weighted selector
func NewCapacityWeightedSelector() *CapacityWeightedSelector {
	return &CapacityWeightedSelector{
		rng:              rand.New(rand.NewSource(time.Now().UnixNano())),
		minCapacityScore: 0.01, // 1% minimum capacity to be eligible
		maxCandidates:    20,    // Consider up to 20 candidates max
	}
}

// NewCapacityWeightedSelectorWithConfig creates a selector with custom configuration
func NewCapacityWeightedSelectorWithConfig(minCapacity float64, maxCandidates int) *CapacityWeightedSelector {
	if minCapacity < 0 {
		minCapacity = 0.01
	}
	if minCapacity > 1 {
		minCapacity = 1.0
	}
	if maxCandidates <= 0 {
		maxCandidates = 20
	}

	return &CapacityWeightedSelector{
		rng:              rand.New(rand.NewSource(time.Now().UnixNano())),
		minCapacityScore: minCapacity,
		maxCandidates:    maxCandidates,
	}
}

// Select chooses the best workspace from candidates using capacity-weighted algorithm
func (cws *CapacityWeightedSelector) Select(ctx context.Context, candidates []WorkspaceCandidate, senderEmail string) (*WorkspaceCandidate, error) {
	if len(candidates) == 0 {
		return nil, NewLoadBalancerError(ErrorTypeNoHealthyWorkspace, "no candidates available for selection", nil)
	}

	// Filter candidates by eligibility
	eligibleCandidates, err := cws.filterEligibleCandidates(candidates)
	if err != nil {
		return nil, fmt.Errorf("failed to filter candidates: %w", err)
	}

	if len(eligibleCandidates) == 0 {
		// If no candidates meet criteria, try with relaxed capacity requirements
		log.Printf("No candidates meet capacity requirements, relaxing criteria for sender %s", senderEmail)
		eligibleCandidates, err = cws.filterEligibleCandidatesRelaxed(candidates)
		if err != nil || len(eligibleCandidates) == 0 {
			return nil, NewLoadBalancerError(ErrorTypeNoHealthyWorkspace, 
				"no eligible candidates after filtering", nil)
		}
	}

	// Limit candidates if too many
	if len(eligibleCandidates) > cws.maxCandidates {
		// Sort by score and take top candidates
		sort.Slice(eligibleCandidates, func(i, j int) bool {
			return eligibleCandidates[i].Score > eligibleCandidates[j].Score
		})
		eligibleCandidates = eligibleCandidates[:cws.maxCandidates]
	}

	// Calculate capacity-weighted scores
	scoredCandidates, err := cws.calculateCapacityWeightedScores(eligibleCandidates, senderEmail)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate scores: %w", err)
	}

	// Select using weighted random selection
	selected, err := cws.weightedRandomSelect(scoredCandidates)
	if err != nil {
		return nil, fmt.Errorf("failed to select candidate: %w", err)
	}

	// Add selection reason for debugging
	selected.SelectionReason = cws.generateSelectionReason(selected, len(candidates), len(eligibleCandidates))

	log.Printf("Selected workspace %s for sender %s: score=%.4f, capacity=%.2f%%, weight=%.2f, reason=%s",
		selected.Workspace.ProviderID, senderEmail, selected.Score,
		selected.Capacity.RemainingPercentage*100, selected.Workspace.Weight, selected.SelectionReason)

	return selected, nil
}

// GetStrategy returns the strategy type this selector implements
func (cws *CapacityWeightedSelector) GetStrategy() SelectionStrategy {
	return StrategyCapacityWeighted
}

// filterEligibleCandidates filters candidates based on capacity and health requirements
func (cws *CapacityWeightedSelector) filterEligibleCandidates(candidates []WorkspaceCandidate) ([]WorkspaceCandidate, error) {
	var eligible []WorkspaceCandidate

	for _, candidate := range candidates {
		// Skip if workspace is not enabled
		if !candidate.Workspace.Enabled {
			continue
		}

		// Skip if capacity information is missing or invalid
		if candidate.Capacity == nil {
			log.Printf("Warning: Skipping candidate %s - missing capacity information", candidate.Workspace.ProviderID)
			continue
		}

		// Check minimum capacity threshold (workspace-specific or global)
		minCapacity := cws.minCapacityScore
		if candidate.Workspace.MinCapacityThreshold > 0 {
			minCapacity = candidate.Workspace.MinCapacityThreshold
		}

		if candidate.Capacity.RemainingPercentage < minCapacity {
			log.Printf("Candidate %s below capacity threshold: %.2f%% < %.2f%%", 
				candidate.Workspace.ProviderID, 
				candidate.Capacity.RemainingPercentage*100, 
				minCapacity*100)
			continue
		}

		// Check if workspace has any remaining capacity
		if candidate.Capacity.EffectiveRemaining <= 0 {
			log.Printf("Candidate %s has no remaining capacity: %d", 
				candidate.Workspace.ProviderID, candidate.Capacity.EffectiveRemaining)
			continue
		}

		// Check health score (should be > 0.5 for good health)
		if candidate.HealthScore < 0.5 {
			log.Printf("Candidate %s has poor health score: %.2f", 
				candidate.Workspace.ProviderID, candidate.HealthScore)
			continue
		}

		eligible = append(eligible, candidate)
	}

	return eligible, nil
}

// filterEligibleCandidatesRelaxed applies relaxed filtering when strict filtering yields no results
func (cws *CapacityWeightedSelector) filterEligibleCandidatesRelaxed(candidates []WorkspaceCandidate) ([]WorkspaceCandidate, error) {
	var eligible []WorkspaceCandidate

	for _, candidate := range candidates {
		// Only require that workspace is enabled
		if !candidate.Workspace.Enabled {
			continue
		}

		// Accept any workspace with capacity > 0, regardless of thresholds
		if candidate.Capacity != nil && candidate.Capacity.EffectiveRemaining > 0 {
			eligible = append(eligible, candidate)
		}
	}

	return eligible, nil
}

// calculateCapacityWeightedScores calculates final scores combining capacity and weights
func (cws *CapacityWeightedSelector) calculateCapacityWeightedScores(candidates []WorkspaceCandidate, senderEmail string) ([]WorkspaceCandidate, error) {
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no candidates to score")
	}

	scoredCandidates := make([]WorkspaceCandidate, len(candidates))
	
	for i, candidate := range candidates {
		score, err := cws.calculateCandidateScore(candidate, senderEmail)
		if err != nil {
			log.Printf("Warning: Failed to calculate score for candidate %s: %v", candidate.Workspace.ProviderID, err)
			score = 0.001 // Minimal score to avoid division by zero
		}

		scoredCandidates[i] = candidate
		scoredCandidates[i].Score = score
	}

	return scoredCandidates, nil
}

// calculateCandidateScore calculates a comprehensive score for a candidate workspace
func (cws *CapacityWeightedSelector) calculateCandidateScore(candidate WorkspaceCandidate, senderEmail string) (float64, error) {
	if candidate.Capacity == nil {
		return 0, fmt.Errorf("capacity information missing")
	}

	// Base score components
	capacityScore := candidate.Capacity.RemainingPercentage
	weightScore := candidate.Workspace.Weight
	healthScore := candidate.HealthScore

	// Normalize weight score (assume max weight of 10.0 for normalization)
	normalizedWeight := math.Min(weightScore/10.0, 1.0)

	// Calculate time-based urgency factor (higher score for workspaces with sooner reset times)
	urgencyFactor := cws.calculateUrgencyFactor(candidate.Capacity.TimeToReset)

	// Composite score calculation
	// Formula: (capacity * 0.4) + (weight * 0.3) + (health * 0.2) + (urgency * 0.1)
	score := (capacityScore * 0.4) + 
			 (normalizedWeight * 0.3) + 
			 (healthScore * 0.2) + 
			 (urgencyFactor * 0.1)

	// Apply bonus for workspaces with very high remaining capacity (>80%)
	if capacityScore > 0.8 {
		score *= 1.1 // 10% bonus
	}

	// Apply penalty for workspaces with low capacity (10-20%)
	if capacityScore < 0.2 && capacityScore >= 0.1 {
		score *= 0.9 // 10% penalty
	}

	// Ensure score is positive and reasonable
	if score <= 0 {
		score = 0.001
	}

	return score, nil
}

// calculateUrgencyFactor calculates urgency based on time to capacity reset
func (cws *CapacityWeightedSelector) calculateUrgencyFactor(timeToReset time.Duration) float64 {
	if timeToReset <= 0 {
		return 1.0 // Immediate reset available
	}

	hours := timeToReset.Hours()
	
	// Higher score for sooner resets
	switch {
	case hours <= 1:
		return 1.0 // Reset within 1 hour
	case hours <= 6:
		return 0.8 // Reset within 6 hours
	case hours <= 12:
		return 0.6 // Reset within 12 hours
	case hours <= 18:
		return 0.4 // Reset within 18 hours
	default:
		return 0.2 // Reset > 18 hours away
	}
}

// weightedRandomSelect selects a candidate using weighted random selection
func (cws *CapacityWeightedSelector) weightedRandomSelect(candidates []WorkspaceCandidate) (*WorkspaceCandidate, error) {
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no candidates available for weighted selection")
	}

	// If only one candidate, return it
	if len(candidates) == 1 {
		return &candidates[0], nil
	}

	// Calculate total weight
	totalWeight := 0.0
	for _, candidate := range candidates {
		totalWeight += candidate.Score
	}

	if totalWeight <= 0 {
		// All candidates have zero weight, select randomly
		selected := cws.rng.Intn(len(candidates))
		return &candidates[selected], nil
	}

	// Generate random value between 0 and totalWeight
	randomValue := cws.rng.Float64() * totalWeight
	
	// Find the candidate corresponding to the random value
	currentWeight := 0.0
	for i, candidate := range candidates {
		currentWeight += candidate.Score
		if currentWeight >= randomValue {
			return &candidates[i], nil
		}
	}

	// Fallback to last candidate (shouldn't happen due to floating point precision)
	return &candidates[len(candidates)-1], nil
}

// generateSelectionReason creates a human-readable reason for the selection
func (cws *CapacityWeightedSelector) generateSelectionReason(selected *WorkspaceCandidate, totalCandidates, eligibleCandidates int) string {
	capacity := selected.Capacity.RemainingPercentage * 100
	weight := selected.Workspace.Weight

	reason := fmt.Sprintf("capacity_weighted(%.1f%% capacity, %.1fx weight", capacity, weight)

	if totalCandidates > eligibleCandidates {
		reason += fmt.Sprintf(", %d/%d eligible", eligibleCandidates, totalCandidates)
	}

	if capacity > 80 {
		reason += ", high_capacity_bonus"
	} else if capacity < 20 {
		reason += ", low_capacity_penalty"
	}

	if selected.Capacity.TimeToReset < time.Hour {
		reason += ", reset_soon"
	}

	reason += ")"
	return reason
}

// SetMinCapacityScore sets the minimum capacity score required for eligibility
func (cws *CapacityWeightedSelector) SetMinCapacityScore(minScore float64) {
	if minScore >= 0 && minScore <= 1 {
		cws.minCapacityScore = minScore
	}
}

// GetMinCapacityScore returns the current minimum capacity score
func (cws *CapacityWeightedSelector) GetMinCapacityScore() float64 {
	return cws.minCapacityScore
}

// SetMaxCandidates sets the maximum number of candidates to consider
func (cws *CapacityWeightedSelector) SetMaxCandidates(maxCandidates int) {
	if maxCandidates > 0 {
		cws.maxCandidates = maxCandidates
	}
}

// GetMaxCandidates returns the current maximum number of candidates
func (cws *CapacityWeightedSelector) GetMaxCandidates() int {
	return cws.maxCandidates
}

// GetSelectionStats returns statistics about the last selection process
func (cws *CapacityWeightedSelector) GetSelectionStats(candidates []WorkspaceCandidate) map[string]interface{} {
	if len(candidates) == 0 {
		return map[string]interface{}{
			"total_candidates":    0,
			"eligible_candidates": 0,
			"avg_capacity":        0.0,
			"avg_weight":          0.0,
			"avg_health_score":    0.0,
		}
	}

	eligible, _ := cws.filterEligibleCandidates(candidates)

	var totalCapacity, totalWeight, totalHealth float64
	for _, candidate := range eligible {
		if candidate.Capacity != nil {
			totalCapacity += candidate.Capacity.RemainingPercentage
		}
		totalWeight += candidate.Workspace.Weight
		totalHealth += candidate.HealthScore
	}

	eligibleCount := len(eligible)
	avgCapacity := 0.0
	avgWeight := 0.0
	avgHealth := 0.0

	if eligibleCount > 0 {
		avgCapacity = totalCapacity / float64(eligibleCount)
		avgWeight = totalWeight / float64(eligibleCount)
		avgHealth = totalHealth / float64(eligibleCount)
	}

	return map[string]interface{}{
		"total_candidates":    len(candidates),
		"eligible_candidates": eligibleCount,
		"avg_capacity":        avgCapacity,
		"avg_weight":          avgWeight,
		"avg_health_score":    avgHealth,
		"min_capacity_threshold": cws.minCapacityScore,
		"max_candidates":         cws.maxCandidates,
	}
}