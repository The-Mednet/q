package manager

import (
	"context"
	"fmt"
	"sync"
	"time"

	"relay/internal/gateway"
	"relay/internal/gateway/ratelimit"
	"relay/internal/gateway/reliability"
)

// GatewayManagerImpl implements the GatewayManager interface
type GatewayManagerImpl struct {
	mu              sync.RWMutex
	gateways        map[string]gateway.GatewayInterface
	router          gateway.GatewayRouter
	rateLimiter     *ratelimit.MultiGatewayRateLimiter
	circuitBreakers *reliability.CircuitBreakerManager

	// Health monitoring
	healthMonitorCtx    context.Context
	healthMonitorCancel context.CancelFunc
	healthInterval      time.Duration

	// Metrics
	metrics           gateway.AggregateMetrics
	metricsLastUpdate time.Time
}

// NewGatewayManager creates a new gateway manager
func NewGatewayManager(
	router gateway.GatewayRouter,
	rateLimiter *ratelimit.MultiGatewayRateLimiter,
	circuitBreakers *reliability.CircuitBreakerManager,
) *GatewayManagerImpl {
	return &GatewayManagerImpl{
		gateways:        make(map[string]gateway.GatewayInterface),
		router:          router,
		rateLimiter:     rateLimiter,
		circuitBreakers: circuitBreakers,
		healthInterval:  30 * time.Second,
		metrics: gateway.AggregateMetrics{
			GatewayStats: make(map[string]gateway.GatewayMetrics),
		},
	}
}

// RegisterGateway implements GatewayManager.RegisterGateway
func (gm *GatewayManagerImpl) RegisterGateway(gw gateway.GatewayInterface) error {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	gatewayID := gw.GetID()
	if _, exists := gm.gateways[gatewayID]; exists {
		return fmt.Errorf("gateway %s already registered", gatewayID)
	}

	gm.gateways[gatewayID] = gw

	// Initialize circuit breaker if available
	if gm.circuitBreakers != nil {
		cb, err := gm.circuitBreakers.GetOrCreateCircuitBreaker(gatewayID, nil)
		if err != nil {
			return fmt.Errorf("failed to create circuit breaker for gateway %s: %w", gatewayID, err)
		}

		// If the gateway supports circuit breakers, set it up
		if cbGateway, ok := gw.(CircuitBreakerGateway); ok {
			cbGateway.SetCircuitBreaker(cb)
		}
	}

	return nil
}

// CircuitBreakerGateway interface for gateways that support circuit breakers
type CircuitBreakerGateway interface {
	SetCircuitBreaker(cb gateway.CircuitBreakerInterface)
}

// UnregisterGateway implements GatewayManager.UnregisterGateway
func (gm *GatewayManagerImpl) UnregisterGateway(gatewayID string) error {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	if _, exists := gm.gateways[gatewayID]; !exists {
		return fmt.Errorf("gateway %s not registered", gatewayID)
	}

	delete(gm.gateways, gatewayID)

	// Clean up circuit breaker
	if gm.circuitBreakers != nil {
		gm.circuitBreakers.RemoveCircuitBreaker(gatewayID)
	}

	// Clean up rate limiter
	if gm.rateLimiter != nil {
		gm.rateLimiter.UnregisterGateway(gatewayID)
	}

	return nil
}

// GetGateway implements GatewayManager.GetGateway
func (gm *GatewayManagerImpl) GetGateway(gatewayID string) (gateway.GatewayInterface, error) {
	gm.mu.RLock()
	defer gm.mu.RUnlock()

	gw, exists := gm.gateways[gatewayID]
	if !exists {
		return nil, fmt.Errorf("gateway %s not found", gatewayID)
	}

	return gw, nil
}

// GetAllGateways implements GatewayManager.GetAllGateways
func (gm *GatewayManagerImpl) GetAllGateways() []gateway.GatewayInterface {
	gm.mu.RLock()
	defer gm.mu.RUnlock()

	gateways := make([]gateway.GatewayInterface, 0, len(gm.gateways))
	for _, gw := range gm.gateways {
		gateways = append(gateways, gw)
	}

	return gateways
}

// StartHealthMonitoring implements GatewayManager.StartHealthMonitoring
func (gm *GatewayManagerImpl) StartHealthMonitoring(ctx context.Context, interval time.Duration) {
	gm.mu.Lock()

	// Stop existing monitoring if running
	if gm.healthMonitorCancel != nil {
		gm.healthMonitorCancel()
	}

	// Create new context for health monitoring
	gm.healthMonitorCtx, gm.healthMonitorCancel = context.WithCancel(ctx)
	if interval > 0 {
		gm.healthInterval = interval
	}

	gm.mu.Unlock()

	// Start monitoring goroutine
	go gm.healthMonitorLoop()
}

// StopHealthMonitoring implements GatewayManager.StopHealthMonitoring
func (gm *GatewayManagerImpl) StopHealthMonitoring() {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	if gm.healthMonitorCancel != nil {
		gm.healthMonitorCancel()
		gm.healthMonitorCancel = nil
	}
}

// healthMonitorLoop runs the health monitoring loop
func (gm *GatewayManagerImpl) healthMonitorLoop() {
	ticker := time.NewTicker(gm.healthInterval)
	defer ticker.Stop()

	for {
		select {
		case <-gm.healthMonitorCtx.Done():
			return

		case <-ticker.C:
			gm.performHealthChecks()
		}
	}
}

// performHealthChecks performs health checks on all gateways
func (gm *GatewayManagerImpl) performHealthChecks() {
	gm.mu.RLock()
	gateways := make(map[string]gateway.GatewayInterface)
	for id, gw := range gm.gateways {
		gateways[id] = gw
	}
	gm.mu.RUnlock()

	// Perform health checks in parallel
	var wg sync.WaitGroup
	for gatewayID, gw := range gateways {
		wg.Add(1)
		go func(id string, g gateway.GatewayInterface) {
			defer wg.Done()
			gm.performSingleHealthCheck(id, g)
		}(gatewayID, gw)
	}

	wg.Wait()
}

// performSingleHealthCheck performs a health check on a single gateway
func (gm *GatewayManagerImpl) performSingleHealthCheck(gatewayID string, gw gateway.GatewayInterface) {
	// Create timeout context for health check
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Perform health check
	err := gw.HealthCheck(ctx)

	// Determine status
	var status gateway.GatewayStatus
	if err != nil {
		status = gateway.GatewayStatusUnhealthy
	} else {
		status = gateway.GatewayStatusHealthy
	}

	// Update router with health status
	if gm.router != nil {
		gm.router.UpdateGatewayHealth(gatewayID, status, err)
	}
}

// GetAggregateMetrics implements GatewayManager.GetAggregateMetrics
func (gm *GatewayManagerImpl) GetAggregateMetrics() gateway.AggregateMetrics {
	gm.mu.RLock()
	defer gm.mu.RUnlock()

	// Update metrics if stale
	if time.Since(gm.metricsLastUpdate) > 30*time.Second {
		gm.updateAggregateMetrics()
	}

	return gm.metrics
}

// GetGatewayStats implements GatewayManager.GetGatewayStats
func (gm *GatewayManagerImpl) GetGatewayStats() map[string]gateway.GatewayMetrics {
	gm.mu.RLock()
	defer gm.mu.RUnlock()

	stats := make(map[string]gateway.GatewayMetrics)
	for id, gw := range gm.gateways {
		stats[id] = gw.GetMetrics()
	}

	return stats
}

// updateAggregateMetrics updates the cached aggregate metrics
func (gm *GatewayManagerImpl) updateAggregateMetrics() {
	totalMessages := int64(0)
	totalSent := int64(0)
	totalFailed := int64(0)
	healthyGateways := 0
	totalGateways := len(gm.gateways)

	gatewayStats := make(map[string]gateway.GatewayMetrics)
	var lastProcessed *time.Time

	for id, gw := range gm.gateways {
		metrics := gw.GetMetrics()
		gatewayStats[id] = metrics

		totalSent += metrics.TotalSent
		totalFailed += metrics.TotalFailed
		totalMessages += metrics.TotalSent + metrics.TotalFailed

		if gw.GetStatus() == gateway.GatewayStatusHealthy {
			healthyGateways++
		}

		if metrics.LastSent != nil && (lastProcessed == nil || metrics.LastSent.After(*lastProcessed)) {
			lastProcessed = metrics.LastSent
		}
	}

	// Calculate overall success rate
	var overallSuccessRate float64
	if totalMessages > 0 {
		overallSuccessRate = float64(totalSent) / float64(totalMessages) * 100
	}

	gm.metrics = gateway.AggregateMetrics{
		TotalMessages:      totalMessages,
		TotalSent:          totalSent,
		TotalFailed:        totalFailed,
		OverallSuccessRate: overallSuccessRate,
		GatewayStats:       gatewayStats,
		HealthyGateways:    healthyGateways,
		TotalGateways:      totalGateways,
		LastProcessed:      lastProcessed,
	}

	gm.metricsLastUpdate = time.Now()
}

// GetGatewayHealth returns health status for all gateways
func (gm *GatewayManagerImpl) GetGatewayHealth() map[string]GatewayHealthStatus {
	gm.mu.RLock()
	defer gm.mu.RUnlock()

	health := make(map[string]GatewayHealthStatus)
	for id, gw := range gm.gateways {
		status := GatewayHealthStatus{
			GatewayID:   id,
			GatewayType: string(gw.GetType()),
			Status:      gw.GetStatus(),
			LastError:   gw.GetLastError(),
			Metrics:     gw.GetMetrics(),
		}

		// Add circuit breaker info if available
		if gm.circuitBreakers != nil {
			if cb, err := gm.circuitBreakers.GetOrCreateCircuitBreaker(id, nil); err == nil && cb != nil {
				status.CircuitBreakerState = cb.GetState()
				status.CircuitBreakerMetrics = cb.GetMetrics()
			}
		}

		health[id] = status
	}

	return health
}

// GetSystemHealth returns overall system health
func (gm *GatewayManagerImpl) GetSystemHealth() SystemHealthStatus {
	health := gm.GetGatewayHealth()

	healthyCount := 0
	degradedCount := 0
	unhealthyCount := 0

	for _, gwHealth := range health {
		switch gwHealth.Status {
		case gateway.GatewayStatusHealthy:
			healthyCount++
		case gateway.GatewayStatusDegraded:
			degradedCount++
		case gateway.GatewayStatusUnhealthy:
			unhealthyCount++
		}
	}

	totalGateways := len(health)
	var overallStatus gateway.GatewayStatus

	if unhealthyCount == totalGateways {
		overallStatus = gateway.GatewayStatusUnhealthy
	} else if unhealthyCount > 0 || degradedCount > 0 {
		overallStatus = gateway.GatewayStatusDegraded
	} else {
		overallStatus = gateway.GatewayStatusHealthy
	}

	return SystemHealthStatus{
		OverallStatus:     overallStatus,
		TotalGateways:     totalGateways,
		HealthyGateways:   healthyCount,
		DegradedGateways:  degradedCount,
		UnhealthyGateways: unhealthyCount,
		GatewayHealth:     health,
		LastUpdated:       time.Now(),
	}
}

// Supporting types

// GatewayHealthStatus represents the health status of a single gateway
type GatewayHealthStatus struct {
	GatewayID             string                        `json:"gateway_id"`
	GatewayType           string                        `json:"gateway_type"`
	Status                gateway.GatewayStatus         `json:"status"`
	LastError             error                         `json:"last_error,omitempty"`
	Metrics               gateway.GatewayMetrics        `json:"metrics"`
	CircuitBreakerState   gateway.CircuitBreakerState   `json:"circuit_breaker_state,omitempty"`
	CircuitBreakerMetrics gateway.CircuitBreakerMetrics `json:"circuit_breaker_metrics,omitempty"`
}

// SystemHealthStatus represents the overall system health
type SystemHealthStatus struct {
	OverallStatus     gateway.GatewayStatus          `json:"overall_status"`
	TotalGateways     int                            `json:"total_gateways"`
	HealthyGateways   int                            `json:"healthy_gateways"`
	DegradedGateways  int                            `json:"degraded_gateways"`
	UnhealthyGateways int                            `json:"unhealthy_gateways"`
	GatewayHealth     map[string]GatewayHealthStatus `json:"gateway_health"`
	LastUpdated       time.Time                      `json:"last_updated"`
}
