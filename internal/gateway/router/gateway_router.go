package router

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"relay/internal/config"
	"relay/internal/gateway"
	"relay/internal/gateway/reliability"
	"relay/pkg/models"
)

// GatewayRouterImpl implements the GatewayRouter interface
type GatewayRouterImpl struct {
	mu                    sync.RWMutex
	gateways              map[string]gateway.GatewayInterface
	gatewayConfigs        map[string]*config.GatewayConfig
	strategy              gateway.RoutingStrategy
	circuitBreakerManager *reliability.CircuitBreakerManager

	// Health tracking
	healthStatus     map[string]gateway.GatewayStatus
	healthUpdateTime map[string]time.Time

	// Routing state
	roundRobinIndex int
	rand            *rand.Rand

	// Configuration
	healthCheckRequired    bool
	circuitBreakerRequired bool
}

// NewGatewayRouter creates a new gateway router
func NewGatewayRouter(strategy gateway.RoutingStrategy, cbManager *reliability.CircuitBreakerManager) *GatewayRouterImpl {
	return &GatewayRouterImpl{
		gateways:               make(map[string]gateway.GatewayInterface),
		gatewayConfigs:         make(map[string]*config.GatewayConfig),
		strategy:               strategy,
		circuitBreakerManager:  cbManager,
		healthStatus:           make(map[string]gateway.GatewayStatus),
		healthUpdateTime:       make(map[string]time.Time),
		rand:                   rand.New(rand.NewSource(time.Now().UnixNano())),
		healthCheckRequired:    true,
		circuitBreakerRequired: true,
	}
}

// RegisterGateway registers a gateway with the router
func (gr *GatewayRouterImpl) RegisterGateway(gw gateway.GatewayInterface, config *config.GatewayConfig) error {
	gr.mu.Lock()
	defer gr.mu.Unlock()

	gatewayID := gw.GetID()
	if _, exists := gr.gateways[gatewayID]; exists {
		return fmt.Errorf("gateway %s already registered", gatewayID)
	}

	gr.gateways[gatewayID] = gw
	if config != nil {
		gr.gatewayConfigs[gatewayID] = config
	}
	gr.healthStatus[gatewayID] = gateway.GatewayStatusHealthy
	gr.healthUpdateTime[gatewayID] = time.Now()

	return nil
}

// UnregisterGateway removes a gateway from the router
func (gr *GatewayRouterImpl) UnregisterGateway(gatewayID string) error {
	gr.mu.Lock()
	defer gr.mu.Unlock()

	if _, exists := gr.gateways[gatewayID]; !exists {
		return fmt.Errorf("gateway %s not registered", gatewayID)
	}

	delete(gr.gateways, gatewayID)
	delete(gr.gatewayConfigs, gatewayID)
	delete(gr.healthStatus, gatewayID)
	delete(gr.healthUpdateTime, gatewayID)

	return nil
}

// RouteMessage routes a message to the best available gateway
func (gr *GatewayRouterImpl) RouteMessage(ctx context.Context, msg *models.Message) (gateway.GatewayInterface, error) {
	// Get available gateways for this sender
	candidates := gr.GetAvailableGateways(msg.From)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no available gateways for sender %s", msg.From)
	}

	// Apply routing strategy
	selectedGateway, err := gr.selectGateway(candidates, msg)
	if err != nil {
		return nil, fmt.Errorf("failed to select gateway: %w", err)
	}

	// Try to send with failover if configured
	return gr.tryWithFailover(ctx, selectedGateway, msg)
}

// GetAvailableGateways returns all available gateways for a sender
func (gr *GatewayRouterImpl) GetAvailableGateways(senderEmail string) []gateway.GatewayInterface {
	gr.mu.RLock()
	defer gr.mu.RUnlock()

	var available []gateway.GatewayInterface

	for gatewayID, gw := range gr.gateways {
		log.Printf("DEBUG: Checking gateway %s for sender %s", gatewayID, senderEmail)

		// Check if gateway is enabled
		if config, exists := gr.gatewayConfigs[gatewayID]; exists && !config.Enabled {
			log.Printf("DEBUG: Gateway %s is disabled", gatewayID)
			continue
		}

		// Check if gateway can route this sender
		if !gw.CanRoute(senderEmail) {
			log.Printf("DEBUG: Gateway %s cannot route sender %s", gatewayID, senderEmail)
			continue
		}

		// Check routing patterns if configured
		if config, exists := gr.gatewayConfigs[gatewayID]; exists {
			if !gr.matchesRoutingRules(senderEmail, config.Routing) {
				log.Printf("DEBUG: Gateway %s routing rules failed for sender %s, patterns: %v", gatewayID, senderEmail, config.Routing.CanRoute)
				continue
			}
		}

		// Check health status if required
		if gr.healthCheckRequired {
			if status, exists := gr.healthStatus[gatewayID]; exists {
				if status == gateway.GatewayStatusUnhealthy || status == gateway.GatewayStatusDisabled {
					continue
				}
			}
		}

		// Check circuit breaker status if required
		if gr.circuitBreakerRequired && gr.circuitBreakerManager != nil {
			cb, _ := gr.circuitBreakerManager.GetOrCreateCircuitBreaker(gatewayID, nil)
			if cb != nil && !cb.IsHealthy() {
				continue
			}
		}

		log.Printf("DEBUG: Gateway %s passed all checks, adding to available list", gatewayID)
		available = append(available, gw)
	}

	log.Printf("DEBUG: Found %d available gateways for sender %s", len(available), senderEmail)
	return available
}

// matchesRoutingRules checks if a sender matches the routing rules
func (gr *GatewayRouterImpl) matchesRoutingRules(senderEmail string, routing config.GatewayRoutingConfig) bool {
	// Check exclude patterns first
	for _, pattern := range routing.ExcludePatterns {
		if gr.matchesPattern(senderEmail, pattern) {
			return false
		}
	}

	// Check include patterns
	if len(routing.CanRoute) == 0 {
		return true // No restrictions
	}

	for _, pattern := range routing.CanRoute {
		if gr.matchesPattern(senderEmail, pattern) {
			return true
		}
	}

	return false
}

// matchesPattern checks if an email matches a pattern
func (gr *GatewayRouterImpl) matchesPattern(email, pattern string) bool {
	if pattern == "*" {
		return true
	}

	if strings.HasPrefix(pattern, "@") {
		// Domain pattern
		domain := pattern[1:]
		emailParts := strings.Split(email, "@")
		if len(emailParts) == 2 {
			return strings.EqualFold(emailParts[1], domain)
		}
	}

	// Exact match or wildcard match
	return strings.EqualFold(email, pattern) || strings.Contains(strings.ToLower(email), strings.ToLower(pattern))
}

// selectGateway selects a gateway based on the routing strategy
func (gr *GatewayRouterImpl) selectGateway(candidates []gateway.GatewayInterface, msg *models.Message) (gateway.GatewayInterface, error) {
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no candidate gateways")
	}

	if len(candidates) == 1 {
		return candidates[0], nil
	}

	switch gr.strategy {
	case gateway.StrategyRoundRobin:
		return gr.selectRoundRobin(candidates), nil

	case gateway.StrategyWeighted:
		return gr.selectWeighted(candidates), nil

	case gateway.StrategyPriority:
		return gr.selectPriority(candidates), nil

	case gateway.StrategyFailover:
		return gr.selectFailover(candidates), nil

	case gateway.StrategyLeastLoaded:
		return gr.selectLeastLoaded(candidates), nil

	case gateway.StrategyDomainBased:
		return gr.selectDomainBased(candidates, msg.From), nil

	default:
		// Default to priority-based selection
		return gr.selectPriority(candidates), nil
	}
}

// selectRoundRobin implements round-robin selection
func (gr *GatewayRouterImpl) selectRoundRobin(candidates []gateway.GatewayInterface) gateway.GatewayInterface {
	gr.mu.Lock()
	defer gr.mu.Unlock()

	if gr.roundRobinIndex >= len(candidates) {
		gr.roundRobinIndex = 0
	}

	selected := candidates[gr.roundRobinIndex]
	gr.roundRobinIndex++

	return selected
}

// selectWeighted implements weighted selection
func (gr *GatewayRouterImpl) selectWeighted(candidates []gateway.GatewayInterface) gateway.GatewayInterface {
	totalWeight := 0
	for _, gw := range candidates {
		totalWeight += gw.GetWeight()
	}

	if totalWeight == 0 {
		// Fall back to random selection if no weights
		return candidates[gr.rand.Intn(len(candidates))]
	}

	target := gr.rand.Intn(totalWeight)
	current := 0

	for _, gw := range candidates {
		current += gw.GetWeight()
		if current > target {
			return gw
		}
	}

	// Fallback (should not reach here)
	return candidates[len(candidates)-1]
}

// selectPriority implements priority-based selection
func (gr *GatewayRouterImpl) selectPriority(candidates []gateway.GatewayInterface) gateway.GatewayInterface {
	// Sort by priority (lower number = higher priority)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].GetPriority() < candidates[j].GetPriority()
	})

	return candidates[0]
}

// selectFailover implements failover selection (highest priority healthy gateway)
func (gr *GatewayRouterImpl) selectFailover(candidates []gateway.GatewayInterface) gateway.GatewayInterface {
	// Same as priority for now, but could implement more sophisticated failover logic
	return gr.selectPriority(candidates)
}

// selectLeastLoaded implements least-loaded selection
func (gr *GatewayRouterImpl) selectLeastLoaded(candidates []gateway.GatewayInterface) gateway.GatewayInterface {
	var bestGateway gateway.GatewayInterface
	bestUtilization := float64(100.0)

	for _, gw := range candidates {
		metrics := gw.GetMetrics()
		// Calculate utilization based on success rate and recent activity
		utilization := (100.0 - metrics.SuccessRate) // Higher failure rate = higher utilization
		if utilization < bestUtilization {
			bestUtilization = utilization
			bestGateway = gw
		}
	}

	if bestGateway == nil {
		return candidates[0] // Fallback
	}

	return bestGateway
}

// selectDomainBased implements domain-based selection
func (gr *GatewayRouterImpl) selectDomainBased(candidates []gateway.GatewayInterface, senderEmail string) gateway.GatewayInterface {
	emailParts := strings.Split(senderEmail, "@")
	if len(emailParts) != 2 {
		return candidates[0] // Fallback for invalid email
	}

	domain := emailParts[1]

	// Try to find a gateway that can handle this domain specifically
	for _, gw := range candidates {
		if config, exists := gr.gatewayConfigs[gw.GetID()]; exists {
			for _, pattern := range config.Routing.CanRoute {
				if strings.EqualFold(pattern, "@"+domain) {
					return gw
				}
			}
		}
	}

	// Fall back to priority selection
	return gr.selectPriority(candidates)
}

// tryWithFailover attempts to use a gateway with failover support
func (gr *GatewayRouterImpl) tryWithFailover(ctx context.Context, primary gateway.GatewayInterface, msg *models.Message) (gateway.GatewayInterface, error) {
	// Check if failover is configured for this gateway
	config, exists := gr.gatewayConfigs[primary.GetID()]
	if !exists || len(config.Routing.FailoverTo) == 0 {
		// No failover configured, return primary
		return primary, nil
	}

	// Try primary gateway first
	if gr.isGatewayUsable(primary) {
		return primary, nil
	}

	// Primary is not usable, try failover gateways
	for _, failoverID := range config.Routing.FailoverTo {
		if failoverGw, exists := gr.gateways[failoverID]; exists {
			if gr.isGatewayUsable(failoverGw) && failoverGw.CanRoute(msg.From) {
				return failoverGw, nil
			}
		}
	}

	// No failover gateways available, return primary anyway
	// The caller will handle the failure
	return primary, nil
}

// isGatewayUsable checks if a gateway is currently usable
func (gr *GatewayRouterImpl) isGatewayUsable(gw gateway.GatewayInterface) bool {
	gatewayID := gw.GetID()

	// Check health status
	if status, exists := gr.healthStatus[gatewayID]; exists {
		if status == gateway.GatewayStatusUnhealthy || status == gateway.GatewayStatusDisabled {
			return false
		}
	}

	// Check circuit breaker
	if gr.circuitBreakerManager != nil {
		cb, _ := gr.circuitBreakerManager.GetOrCreateCircuitBreaker(gatewayID, nil)
		if cb != nil && !cb.IsHealthy() {
			return false
		}
	}

	return true
}

// UpdateGatewayHealth updates the health status of a gateway
func (gr *GatewayRouterImpl) UpdateGatewayHealth(gatewayID string, status gateway.GatewayStatus, err error) {
	gr.mu.Lock()
	defer gr.mu.Unlock()

	gr.healthStatus[gatewayID] = status
	gr.healthUpdateTime[gatewayID] = time.Now()

	// Log the health change (would integrate with actual logging system)
	if err != nil {
		fmt.Printf("Gateway %s health changed to %s: %v\n", gatewayID, status, err)
	} else {
		fmt.Printf("Gateway %s health changed to %s\n", gatewayID, status)
	}
}

// GetHealthyGateways returns all currently healthy gateways
func (gr *GatewayRouterImpl) GetHealthyGateways() []gateway.GatewayInterface {
	gr.mu.RLock()
	defer gr.mu.RUnlock()

	var healthy []gateway.GatewayInterface
	for gatewayID, gw := range gr.gateways {
		if status, exists := gr.healthStatus[gatewayID]; exists {
			if status == gateway.GatewayStatusHealthy || status == gateway.GatewayStatusDegraded {
				healthy = append(healthy, gw)
			}
		}
	}

	return healthy
}

// GetRoutingStrategy returns the current routing strategy
func (gr *GatewayRouterImpl) GetRoutingStrategy() gateway.RoutingStrategy {
	gr.mu.RLock()
	defer gr.mu.RUnlock()
	return gr.strategy
}

// SetRoutingStrategy sets the routing strategy
func (gr *GatewayRouterImpl) SetRoutingStrategy(strategy gateway.RoutingStrategy) {
	gr.mu.Lock()
	defer gr.mu.Unlock()
	gr.strategy = strategy
}

// GetGatewayStats returns statistics about gateway routing
func (gr *GatewayRouterImpl) GetGatewayStats() map[string]interface{} {
	gr.mu.RLock()
	defer gr.mu.RUnlock()

	stats := make(map[string]interface{})
	stats["total_gateways"] = len(gr.gateways)
	stats["routing_strategy"] = string(gr.strategy)
	stats["health_check_required"] = gr.healthCheckRequired
	stats["circuit_breaker_required"] = gr.circuitBreakerRequired

	// Health status summary
	healthSummary := make(map[string]int)
	for _, status := range gr.healthStatus {
		healthSummary[string(status)]++
	}
	stats["health_summary"] = healthSummary

	// Gateway details
	gatewayDetails := make(map[string]interface{})
	for id, gw := range gr.gateways {
		details := map[string]interface{}{
			"type":     string(gw.GetType()),
			"priority": gw.GetPriority(),
			"weight":   gw.GetWeight(),
			"status":   string(gr.healthStatus[id]),
		}

		if updateTime, exists := gr.healthUpdateTime[id]; exists {
			details["last_health_update"] = updateTime
		}

		gatewayDetails[id] = details
	}
	stats["gateways"] = gatewayDetails

	return stats
}

// SetHealthCheckRequired sets whether health checks are required
func (gr *GatewayRouterImpl) SetHealthCheckRequired(required bool) {
	gr.mu.Lock()
	defer gr.mu.Unlock()
	gr.healthCheckRequired = required
}

// SetCircuitBreakerRequired sets whether circuit breakers are required
func (gr *GatewayRouterImpl) SetCircuitBreakerRequired(required bool) {
	gr.mu.Lock()
	defer gr.mu.Unlock()
	gr.circuitBreakerRequired = required
}
