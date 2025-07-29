package queue

import (
	"sync"
	"time"
)

// PerSenderRateLimiter tracks rate limits per sender email address
type PerSenderRateLimiter struct {
	mu       sync.RWMutex
	limiters map[string]*RateLimiter
	limit    int
}

// RateLimiter tracks rate limit for a single sender
type RateLimiter struct {
	mu        sync.RWMutex
	limit     int
	window    time.Duration
	sentTimes []time.Time
}

func NewPerSenderRateLimiter(dailyLimit int) *PerSenderRateLimiter {
	return &PerSenderRateLimiter{
		limiters: make(map[string]*RateLimiter),
		limit:    dailyLimit,
	}
}

func (psr *PerSenderRateLimiter) Allow(senderEmail string) bool {
	limiter := psr.getLimiterForSender(senderEmail)
	return limiter.Allow()
}

func (psr *PerSenderRateLimiter) Record(senderEmail string, count int) {
	limiter := psr.getLimiterForSender(senderEmail)
	limiter.Record(count)
}

func (psr *PerSenderRateLimiter) GetStatus(senderEmail string) (sent int, remaining int, resetTime time.Time) {
	limiter := psr.getLimiterForSender(senderEmail)
	return limiter.GetStatus()
}

// GetGlobalStatus returns aggregate status across all senders
func (psr *PerSenderRateLimiter) GetGlobalStatus() (totalSent int, senderCount int, senders map[string]SenderStats) {
	psr.mu.RLock()
	defer psr.mu.RUnlock()

	senders = make(map[string]SenderStats)
	totalSent = 0
	senderCount = len(psr.limiters)

	for email, limiter := range psr.limiters {
		sent, remaining, resetTime := limiter.GetStatus()
		totalSent += sent
		senders[email] = SenderStats{
			Email:     email,
			Sent:      sent,
			Remaining: remaining,
			Limit:     psr.limit,
			ResetTime: resetTime,
		}
	}

	return
}

func (psr *PerSenderRateLimiter) getLimiterForSender(senderEmail string) *RateLimiter {
	psr.mu.RLock()
	if limiter, exists := psr.limiters[senderEmail]; exists {
		psr.mu.RUnlock()
		return limiter
	}
	psr.mu.RUnlock()

	// Need to create new limiter
	psr.mu.Lock()
	defer psr.mu.Unlock()

	// Double-check in case another goroutine created it
	if limiter, exists := psr.limiters[senderEmail]; exists {
		return limiter
	}

	// Create new limiter for this sender
	limiter := NewRateLimiter(psr.limit)
	psr.limiters[senderEmail] = limiter
	return limiter
}

func NewRateLimiter(dailyLimit int) *RateLimiter {
	return &RateLimiter{
		limit:     dailyLimit,
		window:    24 * time.Hour,
		sentTimes: make([]time.Time, 0),
	}
}

func (r *RateLimiter) Allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-r.window)

	// Remove entries older than 24 hours
	validTimes := make([]time.Time, 0)
	for _, t := range r.sentTimes {
		if t.After(cutoff) {
			validTimes = append(validTimes, t)
		}
	}
	r.sentTimes = validTimes

	// Check if we're under the limit
	if len(r.sentTimes) < r.limit {
		r.sentTimes = append(r.sentTimes, now)
		return true
	}

	return false
}

func (r *RateLimiter) Record(count int) {
	if count <= 0 {
		return
	}
	
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	
	// For large counts, pre-allocate slice to avoid multiple reallocations
	if len(r.sentTimes) + count > cap(r.sentTimes) {
		newSlice := make([]time.Time, len(r.sentTimes), len(r.sentTimes) + count)
		copy(newSlice, r.sentTimes)
		r.sentTimes = newSlice
	}
	
	// Add count entries with the same timestamp
	for i := 0; i < count; i++ {
		r.sentTimes = append(r.sentTimes, now)
	}
}

func (r *RateLimiter) GetStatus() (sent int, remaining int, resetTime time.Time) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	now := time.Now()
	cutoff := now.Add(-r.window)

	// Count valid entries
	valid := 0
	earliest := now
	for _, t := range r.sentTimes {
		if t.After(cutoff) {
			valid++
			if t.Before(earliest) {
				earliest = t
			}
		}
	}

	sent = valid
	remaining = r.limit - valid
	if remaining < 0 {
		remaining = 0
	}
	
	// Reset time is 24 hours after the earliest sent email
	if valid > 0 {
		resetTime = earliest.Add(r.window)
	} else {
		resetTime = now
	}

	return
}

// SenderStats represents rate limit statistics for a specific sender
type SenderStats struct {
	Email     string    `json:"email"`
	Sent      int       `json:"sent"`
	Remaining int       `json:"remaining"`
	Limit     int       `json:"limit"`
	ResetTime time.Time `json:"reset_time"`
}