package ratelimit

import (
	"fmt"
	"sync"
	"time"

	"relay/internal/config"
	"relay/internal/gateway"
	"relay/internal/queue"
)

// MultiGatewayRateLimiter manages rate limiting across multiple gateways
type MultiGatewayRateLimiter struct {
	mu sync.RWMutex

	// Gateway-specific limiters
	gatewayLimiters map[string]*GatewayRateLimiter

	// Global limiter for system-wide limits
	globalLimiter *SystemRateLimiter

	// Configuration
	globalDefaults *config.GlobalGatewayDefaults

	// Queue interface for historical data
	queue queue.Queue

	// Metrics
	metrics MultiGatewayMetrics
}

// GatewayRateLimiter handles rate limiting for a specific gateway
type GatewayRateLimiter struct {
	gatewayID   string
	gatewayType gateway.GatewayType
	config      config.GatewayRateLimitConfig

	// Per-user tracking
	userLimiters map[string]*UserRateLimiter
	mu           sync.RWMutex

	// Gateway-level metrics
	totalSent int64
	resetTime time.Time

	// Inheritance from global defaults
	globalDefaults *config.GlobalGatewayDefaults
}

// UserRateLimiter tracks rate limiting for a specific user within a gateway
type UserRateLimiter struct {
	userEmail string
	gatewayID string

	// Rate limit configuration
	dailyLimit  int
	hourlyLimit int
	burstLimit  int

	// Tracking
	dailySent   int
	hourlySent  int
	burstTokens int

	// Time tracking
	lastReset     time.Time
	lastHourReset time.Time

	mu sync.RWMutex
}

// SystemRateLimiter provides system-wide rate limiting across all gateways
type SystemRateLimiter struct {
	systemDailyLimit  int
	systemHourlyLimit int

	totalSentToday int
	totalSentHour  int

	lastReset     time.Time
	lastHourReset time.Time

	mu sync.RWMutex
}

// RateLimitResult represents the result of a rate limit check
type RateLimitResult struct {
	Allowed         bool
	Gateway         string
	User            string
	Reason          string
	RetryAfter      *time.Duration
	RemainingDaily  int
	RemainingHourly int
}

// MultiGatewayMetrics provides aggregated metrics across all gateways
type MultiGatewayMetrics struct {
	TotalGateways       int
	ActiveGateways      int
	TotalSent           int64
	TotalRateLimited    int64
	SystemUtilization   float64
	GatewayUtilizations map[string]float64
	LastReset           time.Time
}

// NewMultiGatewayRateLimiter creates a new multi-gateway rate limiter
func NewMultiGatewayRateLimiter(globalDefaults *config.GlobalGatewayDefaults, q queue.Queue) *MultiGatewayRateLimiter {
	limiter := &MultiGatewayRateLimiter{
		gatewayLimiters: make(map[string]*GatewayRateLimiter),
		globalDefaults:  globalDefaults,
		queue:           q,
		globalLimiter: &SystemRateLimiter{
			systemDailyLimit:  100000, // Default system limit
			systemHourlyLimit: 10000,
			lastReset:         time.Now().Truncate(24 * time.Hour),
			lastHourReset:     time.Now().Truncate(time.Hour),
		},
		metrics: MultiGatewayMetrics{
			GatewayUtilizations: make(map[string]float64),
			LastReset:           time.Now(),
		},
	}

	return limiter
}

// RegisterGateway registers a new gateway with the rate limiter
func (mgl *MultiGatewayRateLimiter) RegisterGateway(gatewayID string, gatewayType gateway.GatewayType, config config.GatewayRateLimitConfig) error {
	mgl.mu.Lock()
	defer mgl.mu.Unlock()

	if _, exists := mgl.gatewayLimiters[gatewayID]; exists {
		return fmt.Errorf("gateway %s already registered", gatewayID)
	}

	limiter := &GatewayRateLimiter{
		gatewayID:      gatewayID,
		gatewayType:    gatewayType,
		config:         config,
		userLimiters:   make(map[string]*UserRateLimiter),
		resetTime:      time.Now().Truncate(24 * time.Hour),
		globalDefaults: mgl.globalDefaults,
	}

	mgl.gatewayLimiters[gatewayID] = limiter
	mgl.metrics.TotalGateways++

	return nil
}

// UnregisterGateway removes a gateway from the rate limiter
func (mgl *MultiGatewayRateLimiter) UnregisterGateway(gatewayID string) error {
	mgl.mu.Lock()
	defer mgl.mu.Unlock()

	if _, exists := mgl.gatewayLimiters[gatewayID]; !exists {
		return fmt.Errorf("gateway %s not registered", gatewayID)
	}

	delete(mgl.gatewayLimiters, gatewayID)
	delete(mgl.metrics.GatewayUtilizations, gatewayID)
	mgl.metrics.TotalGateways--

	return nil
}

// Allow checks if a send is allowed for a specific gateway and user
func (mgl *MultiGatewayRateLimiter) Allow(gatewayID, userEmail string) RateLimitResult {
	mgl.mu.RLock()
	gatewayLimiter, exists := mgl.gatewayLimiters[gatewayID]
	mgl.mu.RUnlock()

	if !exists {
		return RateLimitResult{
			Allowed: false,
			Gateway: gatewayID,
			User:    userEmail,
			Reason:  "gateway not registered",
		}
	}

	// Check system-wide limits first
	if systemResult := mgl.globalLimiter.Allow(); !systemResult.Allowed {
		return RateLimitResult{
			Allowed:    false,
			Gateway:    gatewayID,
			User:       userEmail,
			Reason:     "system rate limit exceeded",
			RetryAfter: systemResult.RetryAfter,
		}
	}

	// Check gateway-specific limits
	return gatewayLimiter.Allow(userEmail)
}

// RecordSend records a successful send for rate limiting purposes
func (mgl *MultiGatewayRateLimiter) RecordSend(gatewayID, userEmail string) error {
	mgl.mu.RLock()
	gatewayLimiter, exists := mgl.gatewayLimiters[gatewayID]
	mgl.mu.RUnlock()

	if !exists {
		return fmt.Errorf("gateway %s not registered", gatewayID)
	}

	// Record at system level
	mgl.globalLimiter.RecordSend()

	// Record at gateway level
	gatewayLimiter.RecordSend(userEmail)

	// Update metrics
	mgl.mu.Lock()
	mgl.metrics.TotalSent++
	mgl.mu.Unlock()

	return nil
}

// GetStatus returns rate limiting status for a gateway and user
func (mgl *MultiGatewayRateLimiter) GetStatus(gatewayID, userEmail string) (sent int, remainingDaily int, remainingHourly int, resetTime time.Time) {
	mgl.mu.RLock()
	gatewayLimiter, exists := mgl.gatewayLimiters[gatewayID]
	mgl.mu.RUnlock()

	if !exists {
		return 0, 0, 0, time.Time{}
	}

	return gatewayLimiter.GetUserStatus(userEmail)
}

// GetGatewayMetrics returns metrics for all gateways
func (mgl *MultiGatewayRateLimiter) GetGatewayMetrics() map[string]GatewayRateLimitMetrics {
	mgl.mu.RLock()
	defer mgl.mu.RUnlock()

	metrics := make(map[string]GatewayRateLimitMetrics)
	for id, limiter := range mgl.gatewayLimiters {
		metrics[id] = limiter.GetMetrics()
	}

	return metrics
}

// InitializeFromQueue initializes rate limiter state from historical queue data
func (mgl *MultiGatewayRateLimiter) InitializeFromQueue() error {
	if mgl.queue == nil {
		return nil
	}

	// This would query the queue for recent sends to initialize current counters
	// Implementation depends on the queue interface
	return nil
}

// Allow checks if a send is allowed for this gateway
func (gl *GatewayRateLimiter) Allow(userEmail string) RateLimitResult {
	now := time.Now()

	// Check if we need to reset daily counters
	if now.Sub(gl.resetTime) >= 24*time.Hour {
		gl.resetDaily()
	}

	// Get effective rate limits for this user
	effectiveLimits := gl.getEffectiveUserLimits(userEmail)

	// Get or create user limiter
	userLimiter := gl.getUserLimiter(userEmail, effectiveLimits)

	return userLimiter.Allow()
}

// RecordSend records a successful send
func (gl *GatewayRateLimiter) RecordSend(userEmail string) {
	now := time.Now()

	// Check if we need to reset daily counters
	if now.Sub(gl.resetTime) >= 24*time.Hour {
		gl.resetDaily()
	}

	gl.mu.Lock()
	gl.totalSent++
	gl.mu.Unlock()

	// Get effective rate limits for this user
	effectiveLimits := gl.getEffectiveUserLimits(userEmail)

	// Get or create user limiter and record send
	userLimiter := gl.getUserLimiter(userEmail, effectiveLimits)
	userLimiter.RecordSend()
}

// getEffectiveUserLimits returns the effective rate limits for a user
func (gl *GatewayRateLimiter) getEffectiveUserLimits(userEmail string) UserLimits {
	limits := UserLimits{
		DailyLimit:  gl.config.PerUserDaily,
		HourlyLimit: gl.config.PerHour,
		BurstLimit:  gl.config.BurstLimit,
	}

	// Check for custom user limits
	if customLimit, exists := gl.config.CustomUserLimits[userEmail]; exists {
		limits.DailyLimit = customLimit
	}

	// Apply global defaults if not set and inheritance is enabled
	if gl.config.InheritGlobalLimits && gl.globalDefaults != nil {
		if limits.DailyLimit == 0 {
			limits.DailyLimit = gl.globalDefaults.RateLimits.PerUserDaily
		}
		if limits.HourlyLimit == 0 {
			limits.HourlyLimit = gl.globalDefaults.RateLimits.PerHour
		}
		if limits.BurstLimit == 0 {
			limits.BurstLimit = gl.globalDefaults.RateLimits.BurstLimit
		}
	}

	return limits
}

// getUserLimiter gets or creates a user-specific rate limiter
func (gl *GatewayRateLimiter) getUserLimiter(userEmail string, limits UserLimits) *UserRateLimiter {
	gl.mu.Lock()
	defer gl.mu.Unlock()

	if limiter, exists := gl.userLimiters[userEmail]; exists {
		// Update limits in case they changed
		limiter.mu.Lock()
		limiter.dailyLimit = limits.DailyLimit
		limiter.hourlyLimit = limits.HourlyLimit
		limiter.burstLimit = limits.BurstLimit
		limiter.mu.Unlock()
		return limiter
	}

	// Create new user limiter
	limiter := &UserRateLimiter{
		userEmail:     userEmail,
		gatewayID:     gl.gatewayID,
		dailyLimit:    limits.DailyLimit,
		hourlyLimit:   limits.HourlyLimit,
		burstLimit:    limits.BurstLimit,
		burstTokens:   limits.BurstLimit, // Start with full burst capacity
		lastReset:     time.Now().Truncate(24 * time.Hour),
		lastHourReset: time.Now().Truncate(time.Hour),
	}

	gl.userLimiters[userEmail] = limiter
	return limiter
}

// resetDaily resets daily counters for the gateway
func (gl *GatewayRateLimiter) resetDaily() {
	gl.mu.Lock()
	defer gl.mu.Unlock()

	gl.totalSent = 0
	gl.resetTime = time.Now().Truncate(24 * time.Hour)

	// Reset all user limiters
	for _, userLimiter := range gl.userLimiters {
		userLimiter.resetDaily()
	}
}

// GetUserStatus returns status for a specific user
func (gl *GatewayRateLimiter) GetUserStatus(userEmail string) (sent int, remainingDaily int, remainingHourly int, resetTime time.Time) {
	gl.mu.RLock()
	userLimiter, exists := gl.userLimiters[userEmail]
	gl.mu.RUnlock()

	if !exists {
		effectiveLimits := gl.getEffectiveUserLimits(userEmail)
		return 0, effectiveLimits.DailyLimit, effectiveLimits.HourlyLimit, gl.resetTime
	}

	return userLimiter.GetStatus()
}

// GetMetrics returns metrics for this gateway
func (gl *GatewayRateLimiter) GetMetrics() GatewayRateLimitMetrics {
	gl.mu.RLock()
	defer gl.mu.RUnlock()

	return GatewayRateLimitMetrics{
		GatewayID:   gl.gatewayID,
		GatewayType: string(gl.gatewayType),
		TotalSent:   gl.totalSent,
		ActiveUsers: len(gl.userLimiters),
		DailyLimit:  gl.config.WorkspaceDaily,
		Utilization: gl.getUtilization(),
		LastReset:   gl.resetTime,
	}
}

// getUtilization calculates the current utilization percentage
func (gl *GatewayRateLimiter) getUtilization() float64 {
	if gl.config.WorkspaceDaily == 0 {
		return 0.0
	}
	return float64(gl.totalSent) / float64(gl.config.WorkspaceDaily) * 100
}

// Allow checks if this user can send
func (ul *UserRateLimiter) Allow() RateLimitResult {
	ul.mu.Lock()
	defer ul.mu.Unlock()

	now := time.Now()

	// Reset counters if needed
	if now.Sub(ul.lastReset) >= 24*time.Hour {
		ul.resetDailyUnsafe()
	}
	if now.Sub(ul.lastHourReset) >= time.Hour {
		ul.resetHourlyUnsafe()
	}

	// Check daily limit
	if ul.dailyLimit > 0 && ul.dailySent >= ul.dailyLimit {
		nextReset := ul.lastReset.Add(24 * time.Hour)
		retryAfter := time.Until(nextReset)
		return RateLimitResult{
			Allowed:         false,
			Gateway:         ul.gatewayID,
			User:            ul.userEmail,
			Reason:          "daily limit exceeded",
			RetryAfter:      &retryAfter,
			RemainingDaily:  0,
			RemainingHourly: ul.getRemainingHourly(),
		}
	}

	// Check hourly limit
	if ul.hourlyLimit > 0 && ul.hourlySent >= ul.hourlyLimit {
		nextReset := ul.lastHourReset.Add(time.Hour)
		retryAfter := time.Until(nextReset)
		return RateLimitResult{
			Allowed:         false,
			Gateway:         ul.gatewayID,
			User:            ul.userEmail,
			Reason:          "hourly limit exceeded",
			RetryAfter:      &retryAfter,
			RemainingDaily:  ul.getRemainingDaily(),
			RemainingHourly: 0,
		}
	}

	// Check burst limit
	if ul.burstLimit > 0 && ul.burstTokens <= 0 {
		retryAfter := time.Minute // Burst tokens typically refill quickly
		return RateLimitResult{
			Allowed:         false,
			Gateway:         ul.gatewayID,
			User:            ul.userEmail,
			Reason:          "burst limit exceeded",
			RetryAfter:      &retryAfter,
			RemainingDaily:  ul.getRemainingDaily(),
			RemainingHourly: ul.getRemainingHourly(),
		}
	}

	// All limits passed
	return RateLimitResult{
		Allowed:         true,
		Gateway:         ul.gatewayID,
		User:            ul.userEmail,
		RemainingDaily:  ul.getRemainingDaily() - 1, // Account for this send
		RemainingHourly: ul.getRemainingHourly() - 1,
	}
}

// RecordSend records a successful send
func (ul *UserRateLimiter) RecordSend() {
	ul.mu.Lock()
	defer ul.mu.Unlock()

	ul.dailySent++
	ul.hourlySent++

	if ul.burstTokens > 0 {
		ul.burstTokens--
	}
}

// GetStatus returns current status for this user
func (ul *UserRateLimiter) GetStatus() (sent int, remainingDaily int, remainingHourly int, resetTime time.Time) {
	ul.mu.RLock()
	defer ul.mu.RUnlock()

	return ul.dailySent, ul.getRemainingDaily(), ul.getRemainingHourly(), ul.lastReset
}

// Helper methods
func (ul *UserRateLimiter) getRemainingDaily() int {
	if ul.dailyLimit == 0 {
		return -1 // Unlimited
	}
	remaining := ul.dailyLimit - ul.dailySent
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (ul *UserRateLimiter) getRemainingHourly() int {
	if ul.hourlyLimit == 0 {
		return -1 // Unlimited
	}
	remaining := ul.hourlyLimit - ul.hourlySent
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (ul *UserRateLimiter) resetDaily() {
	ul.mu.Lock()
	defer ul.mu.Unlock()
	ul.resetDailyUnsafe()
}

func (ul *UserRateLimiter) resetDailyUnsafe() {
	ul.dailySent = 0
	ul.lastReset = time.Now().Truncate(24 * time.Hour)
	ul.burstTokens = ul.burstLimit // Reset burst tokens
}

func (ul *UserRateLimiter) resetHourlyUnsafe() {
	ul.hourlySent = 0
	ul.lastHourReset = time.Now().Truncate(time.Hour)
}

// Allow checks system-wide rate limits
func (sl *SystemRateLimiter) Allow() RateLimitResult {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	now := time.Now()

	// Reset counters if needed
	if now.Sub(sl.lastReset) >= 24*time.Hour {
		sl.totalSentToday = 0
		sl.lastReset = now.Truncate(24 * time.Hour)
	}
	if now.Sub(sl.lastHourReset) >= time.Hour {
		sl.totalSentHour = 0
		sl.lastHourReset = now.Truncate(time.Hour)
	}

	// Check daily system limit
	if sl.systemDailyLimit > 0 && sl.totalSentToday >= sl.systemDailyLimit {
		nextReset := sl.lastReset.Add(24 * time.Hour)
		retryAfter := time.Until(nextReset)
		return RateLimitResult{
			Allowed:    false,
			Reason:     "system daily limit exceeded",
			RetryAfter: &retryAfter,
		}
	}

	// Check hourly system limit
	if sl.systemHourlyLimit > 0 && sl.totalSentHour >= sl.systemHourlyLimit {
		nextReset := sl.lastHourReset.Add(time.Hour)
		retryAfter := time.Until(nextReset)
		return RateLimitResult{
			Allowed:    false,
			Reason:     "system hourly limit exceeded",
			RetryAfter: &retryAfter,
		}
	}

	return RateLimitResult{Allowed: true}
}

// RecordSend records a send at system level
func (sl *SystemRateLimiter) RecordSend() {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	sl.totalSentToday++
	sl.totalSentHour++
}

// Supporting types
type UserLimits struct {
	DailyLimit  int
	HourlyLimit int
	BurstLimit  int
}

type GatewayRateLimitMetrics struct {
	GatewayID   string    `json:"gateway_id"`
	GatewayType string    `json:"gateway_type"`
	TotalSent   int64     `json:"total_sent"`
	ActiveUsers int       `json:"active_users"`
	DailyLimit  int       `json:"daily_limit"`
	Utilization float64   `json:"utilization"`
	LastReset   time.Time `json:"last_reset"`
}
