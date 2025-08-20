package reliability

import (
	"context"
	"fmt"
	"sync"
	"time"

	"relay/internal/config"
	"relay/internal/gateway"
)

// CircuitBreaker implements the circuit breaker pattern for gateway reliability
type CircuitBreaker struct {
	name   string
	config config.CircuitBreakerConfig

	mu              sync.RWMutex
	state           gateway.CircuitBreakerState
	failureCount    int64
	successCount    int64
	lastFailureTime *time.Time
	lastSuccessTime *time.Time
	nextAttempt     *time.Time

	// Callbacks
	onStateChange func(from, to gateway.CircuitBreakerState)
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(name string, config config.CircuitBreakerConfig) (*CircuitBreaker, error) {
	if !config.Enabled {
		return nil, nil // Return nil if circuit breaker is disabled
	}

	timeout, err := config.ParseTimeout()
	if err != nil {
		return nil, fmt.Errorf("invalid circuit breaker timeout: %w", err)
	}

	if timeout <= 0 {
		return nil, fmt.Errorf("circuit breaker timeout must be positive")
	}

	cb := &CircuitBreaker{
		name:   name,
		config: config,
		state:  gateway.CircuitBreakerClosed,
	}

	return cb, nil
}

// Execute executes a function with circuit breaker protection
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func() error) error {
	if cb == nil {
		// Circuit breaker is disabled, execute directly
		return fn()
	}

	// Check if we can execute
	if !cb.canExecute() {
		return fmt.Errorf("circuit breaker %s is open", cb.name)
	}

	// Execute the function
	err := fn()

	// Record the result
	if err != nil {
		cb.recordFailure()
	} else {
		cb.recordSuccess()
	}

	return err
}

// canExecute determines if the circuit breaker allows execution
func (cb *CircuitBreaker) canExecute() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()

	switch cb.state {
	case gateway.CircuitBreakerClosed:
		return true

	case gateway.CircuitBreakerOpen:
		// Check if we should transition to half-open
		if cb.nextAttempt != nil && now.After(*cb.nextAttempt) {
			cb.setState(gateway.CircuitBreakerHalfOpen)
			return true
		}
		return false

	case gateway.CircuitBreakerHalfOpen:
		// In half-open state, allow limited requests
		if cb.config.MaxRequests > 0 {
			// If MaxRequests is configured, check if we're under the limit
			// This would require tracking concurrent requests, simplified here
			return true
		}
		return true

	default:
		return false
	}
}

// recordFailure records a failure and updates circuit breaker state
func (cb *CircuitBreaker) recordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount++
	now := time.Now()
	cb.lastFailureTime = &now

	switch cb.state {
	case gateway.CircuitBreakerClosed:
		// Check if we should open the circuit
		if cb.failureCount >= int64(cb.config.FailureThreshold) {
			timeout, _ := cb.config.ParseTimeout()
			nextAttempt := now.Add(timeout)
			cb.nextAttempt = &nextAttempt
			cb.setState(gateway.CircuitBreakerOpen)
		}

	case gateway.CircuitBreakerHalfOpen:
		// Any failure in half-open state opens the circuit
		timeout, _ := cb.config.ParseTimeout()
		nextAttempt := now.Add(timeout)
		cb.nextAttempt = &nextAttempt
		cb.setState(gateway.CircuitBreakerOpen)
	}
}

// recordSuccess records a success and updates circuit breaker state
func (cb *CircuitBreaker) recordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.successCount++
	now := time.Now()
	cb.lastSuccessTime = &now

	switch cb.state {
	case gateway.CircuitBreakerHalfOpen:
		// Check if we should close the circuit
		if cb.successCount >= int64(cb.config.SuccessThreshold) {
			cb.failureCount = 0 // Reset failure count
			cb.nextAttempt = nil
			cb.setState(gateway.CircuitBreakerClosed)
		}
	}
}

// setState changes the circuit breaker state and notifies listeners
func (cb *CircuitBreaker) setState(newState gateway.CircuitBreakerState) {
	oldState := cb.state
	cb.state = newState

	if cb.onStateChange != nil && oldState != newState {
		// Call callback outside of lock to avoid deadlocks
		go cb.onStateChange(oldState, newState)
	}
}

// GetState returns the current circuit breaker state
func (cb *CircuitBreaker) GetState() gateway.CircuitBreakerState {
	if cb == nil {
		return gateway.CircuitBreakerClosed // Disabled circuit breaker is always closed
	}

	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// GetMetrics returns current circuit breaker metrics
func (cb *CircuitBreaker) GetMetrics() gateway.CircuitBreakerMetrics {
	if cb == nil {
		return gateway.CircuitBreakerMetrics{
			State: gateway.CircuitBreakerClosed,
		}
	}

	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return gateway.CircuitBreakerMetrics{
		State:           cb.state,
		FailureCount:    cb.failureCount,
		SuccessCount:    cb.successCount,
		LastFailureTime: cb.lastFailureTime,
		LastSuccessTime: cb.lastSuccessTime,
		NextAttemptTime: cb.nextAttempt,
	}
}

// Reset resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	if cb == nil {
		return
	}

	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount = 0
	cb.successCount = 0
	cb.lastFailureTime = nil
	cb.lastSuccessTime = nil
	cb.nextAttempt = nil
	cb.setState(gateway.CircuitBreakerClosed)
}

// SetStateChangeCallback sets a callback for state changes
func (cb *CircuitBreaker) SetStateChangeCallback(callback func(from, to gateway.CircuitBreakerState)) {
	if cb == nil {
		return
	}

	cb.mu.Lock()
	cb.onStateChange = callback
	cb.mu.Unlock()
}

// IsHealthy returns true if the circuit breaker is allowing requests
func (cb *CircuitBreaker) IsHealthy() bool {
	if cb == nil {
		return true // Disabled circuit breaker is always healthy
	}

	return cb.GetState() != gateway.CircuitBreakerOpen
}

// GetFailureRate returns the current failure rate (failures per total attempts)
func (cb *CircuitBreaker) GetFailureRate() float64 {
	if cb == nil {
		return 0.0
	}

	cb.mu.RLock()
	defer cb.mu.RUnlock()

	total := cb.failureCount + cb.successCount
	if total == 0 {
		return 0.0
	}

	return float64(cb.failureCount) / float64(total)
}

// CircuitBreakerManager manages multiple circuit breakers
type CircuitBreakerManager struct {
	mu              sync.RWMutex
	circuitBreakers map[string]*CircuitBreaker
	defaultConfig   config.CircuitBreakerConfig
}

// NewCircuitBreakerManager creates a new circuit breaker manager
func NewCircuitBreakerManager(defaultConfig config.CircuitBreakerConfig) *CircuitBreakerManager {
	return &CircuitBreakerManager{
		circuitBreakers: make(map[string]*CircuitBreaker),
		defaultConfig:   defaultConfig,
	}
}

// GetOrCreateCircuitBreaker gets or creates a circuit breaker for a gateway
func (cbm *CircuitBreakerManager) GetOrCreateCircuitBreaker(gatewayID string, config *config.CircuitBreakerConfig) (*CircuitBreaker, error) {
	cbm.mu.Lock()
	defer cbm.mu.Unlock()

	if cb, exists := cbm.circuitBreakers[gatewayID]; exists {
		return cb, nil
	}

	// Use provided config or default
	cbConfig := cbm.defaultConfig
	if config != nil {
		cbConfig = *config
	}

	cb, err := NewCircuitBreaker(gatewayID, cbConfig)
	if err != nil {
		return nil, err
	}

	if cb != nil {
		// Set up state change logging
		cb.SetStateChangeCallback(func(from, to gateway.CircuitBreakerState) {
			// This would integrate with your logging system
			fmt.Printf("Circuit breaker %s state changed from %s to %s\n", gatewayID, from, to)
		})

		cbm.circuitBreakers[gatewayID] = cb
	}

	return cb, nil
}

// RemoveCircuitBreaker removes a circuit breaker
func (cbm *CircuitBreakerManager) RemoveCircuitBreaker(gatewayID string) {
	cbm.mu.Lock()
	defer cbm.mu.Unlock()

	delete(cbm.circuitBreakers, gatewayID)
}

// GetAllMetrics returns metrics for all circuit breakers
func (cbm *CircuitBreakerManager) GetAllMetrics() map[string]gateway.CircuitBreakerMetrics {
	cbm.mu.RLock()
	defer cbm.mu.RUnlock()

	metrics := make(map[string]gateway.CircuitBreakerMetrics)
	for id, cb := range cbm.circuitBreakers {
		if cb != nil {
			metrics[id] = cb.GetMetrics()
		}
	}

	return metrics
}

// GetHealthyGateways returns IDs of gateways with healthy circuit breakers
func (cbm *CircuitBreakerManager) GetHealthyGateways() []string {
	cbm.mu.RLock()
	defer cbm.mu.RUnlock()

	var healthy []string
	for id, cb := range cbm.circuitBreakers {
		if cb == nil || cb.IsHealthy() {
			healthy = append(healthy, id)
		}
	}

	return healthy
}

// ResetAllCircuitBreakers resets all circuit breakers to closed state
func (cbm *CircuitBreakerManager) ResetAllCircuitBreakers() {
	cbm.mu.RLock()
	defer cbm.mu.RUnlock()

	for _, cb := range cbm.circuitBreakers {
		if cb != nil {
			cb.Reset()
		}
	}
}
