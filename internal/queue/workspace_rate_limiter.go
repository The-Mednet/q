package queue

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"relay/internal/config"
)

// WorkspaceAwareRateLimiter manages rate limits across multiple workspaces
type WorkspaceAwareRateLimiter struct {
	mu                sync.RWMutex
	workspaceConfigs  map[string]*config.WorkspaceConfig
	limiters          map[string]*RateLimiter // key: "workspaceID:senderEmail"
	workspaceLimiters map[string]*RateLimiter // key: workspaceID for workspace-level limits
	globalDefault     int
}

func NewWorkspaceAwareRateLimiter(workspaces map[string]*config.WorkspaceConfig, globalDefault int) *WorkspaceAwareRateLimiter {
	return &WorkspaceAwareRateLimiter{
		workspaceConfigs:  workspaces,
		limiters:          make(map[string]*RateLimiter),
		workspaceLimiters: make(map[string]*RateLimiter),
		globalDefault:     globalDefault,
	}
}

func (warl *WorkspaceAwareRateLimiter) Allow(workspaceID, senderEmail string) bool {
	warl.mu.RLock()
	workspace, exists := warl.workspaceConfigs[workspaceID]
	warl.mu.RUnlock()

	if !exists {
		// Fallback to global limit if workspace not found
		limiter := warl.getLimiterForSender("global", senderEmail, warl.globalDefault)
		return limiter.Allow()
	}

	// Check workspace-level limit first (if configured)
	if workspace.RateLimits.WorkspaceDaily > 0 {
		workspaceLimiter := warl.getWorkspaceLimiter(workspaceID, workspace.RateLimits.WorkspaceDaily)
		if !workspaceLimiter.Allow() {
			return false // Workspace limit exceeded
		}
	}

	// Check user-level limit
	userLimit := warl.getUserLimit(workspace, senderEmail)
	userLimiter := warl.getLimiterForSender(workspaceID, senderEmail, userLimit)
	return userLimiter.Allow()
}

func (warl *WorkspaceAwareRateLimiter) Record(workspaceID, senderEmail string, count int) {
	warl.mu.RLock()
	workspace, exists := warl.workspaceConfigs[workspaceID]
	warl.mu.RUnlock()

	if !exists {
		// Fallback to global limit if workspace not found
		limiter := warl.getLimiterForSender("global", senderEmail, warl.globalDefault)
		limiter.Record(count)
		return
	}

	// Record for workspace-level limit (if configured)
	if workspace.RateLimits.WorkspaceDaily > 0 {
		workspaceLimiter := warl.getWorkspaceLimiter(workspaceID, workspace.RateLimits.WorkspaceDaily)
		workspaceLimiter.Record(count)
	}

	// Record for user-level limit
	userLimit := warl.getUserLimit(workspace, senderEmail)
	userLimiter := warl.getLimiterForSender(workspaceID, senderEmail, userLimit)
	userLimiter.Record(count)
}

func (warl *WorkspaceAwareRateLimiter) GetStatus(workspaceID, senderEmail string) (sent int, remaining int, resetTime time.Time) {
	warl.mu.RLock()
	workspace, exists := warl.workspaceConfigs[workspaceID]
	warl.mu.RUnlock()

	if !exists {
		limiter := warl.getLimiterForSender("global", senderEmail, warl.globalDefault)
		return limiter.GetStatus()
	}

	userLimit := warl.getUserLimit(workspace, senderEmail)
	userLimiter := warl.getLimiterForSender(workspaceID, senderEmail, userLimit)
	return userLimiter.GetStatus()
}

func (warl *WorkspaceAwareRateLimiter) GetWorkspaceStatus(workspaceID string) (sent int, remaining int, resetTime time.Time) {
	warl.mu.RLock()
	workspace, exists := warl.workspaceConfigs[workspaceID]
	warl.mu.RUnlock()

	if !exists || workspace.RateLimits.WorkspaceDaily <= 0 {
		return 0, 0, time.Now() // No workspace limit configured
	}

	workspaceLimiter := warl.getWorkspaceLimiter(workspaceID, workspace.RateLimits.WorkspaceDaily)
	return workspaceLimiter.GetStatus()
}

// RecordSend records a successful send for rate limit tracking
func (warl *WorkspaceAwareRateLimiter) RecordSend(workspaceID, senderEmail string) {
	warl.mu.RLock()
	workspace, exists := warl.workspaceConfigs[workspaceID]
	warl.mu.RUnlock()

	if !exists {
		// Fallback to global tracking if workspace not found
		limiter := warl.getLimiterForSender("global", senderEmail, warl.globalDefault)
		limiter.Record(1)
		return
	}

	// Record workspace-level send if configured
	if workspace.RateLimits.WorkspaceDaily > 0 {
		workspaceLimiter := warl.getWorkspaceLimiter(workspaceID, workspace.RateLimits.WorkspaceDaily)
		workspaceLimiter.Record(1)
	}

	// Record user-level send
	userLimit := warl.getUserLimit(workspace, senderEmail)
	limiter := warl.getLimiterForSender(workspaceID, senderEmail, userLimit)
	limiter.Record(1)
}

// InitializeFromQueue initializes rate limiters with historical data from the queue
func (warl *WorkspaceAwareRateLimiter) InitializeFromQueue(queue Queue) error {
	if queue == nil {
		return fmt.Errorf("queue is nil")
	}

	counts, err := queue.GetSentCountsByWorkspaceAndSender()
	if err != nil {
		return fmt.Errorf("failed to get counts: %v", err)
	}

	log.Printf("DEBUG: Retrieved counts from queue: %+v", counts)

	totalInitialized := 0
	log.Printf("Processing %d workspace groups from database", len(counts))

	for workspaceID, senderCounts := range counts {
		log.Printf("Processing workspace '%s' with %d senders", workspaceID, len(senderCounts))

		// Handle empty workspace_id by mapping from email domain
		if workspaceID == "" {
			log.Printf("Empty workspace_id found, mapping by email domain")
			for senderEmail, count := range senderCounts {
				mappedWorkspaceID := warl.MapEmailToWorkspace(senderEmail)
				if mappedWorkspaceID != "" {
					log.Printf("Mapped %s to workspace %s", senderEmail, mappedWorkspaceID)
					warl.initializeSenderCount(mappedWorkspaceID, senderEmail, count)
					totalInitialized += count
				} else {
					log.Printf("Warning: Could not map email %s to any workspace", senderEmail)
				}
			}
			continue
		}

		// Check if workspace exists (with lock protection)
		warl.mu.RLock()
		_, exists := warl.workspaceConfigs[workspaceID]
		warl.mu.RUnlock()

		if !exists {
			log.Printf("Warning: Workspace %s not found in configs, available: %+v", workspaceID, warl.getAvailableWorkspaceIds())
			continue // Skip unknown workspaces
		}

		// Initialize user-level limiters (no lock held here)
		for senderEmail, count := range senderCounts {
			if count > 0 {
				log.Printf("Initializing rate limiter for %s in workspace %s with count %d", senderEmail, workspaceID, count)
				warl.initializeSenderCount(workspaceID, senderEmail, count)
				totalInitialized += count
			}
		}
	}

	log.Printf("DEBUG: Total messages initialized in rate limiter: %d", totalInitialized)
	return nil
}

func (warl *WorkspaceAwareRateLimiter) getAvailableWorkspaceIds() []string {
	warl.mu.RLock()
	defer warl.mu.RUnlock()

	ids := make([]string, 0, len(warl.workspaceConfigs))
	for id := range warl.workspaceConfigs {
		ids = append(ids, id)
	}
	return ids
}

// MapEmailToWorkspace maps an email address to a workspace ID based on domain
func (warl *WorkspaceAwareRateLimiter) MapEmailToWorkspace(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return ""
	}
	domain := parts[1]

	warl.mu.RLock()
	defer warl.mu.RUnlock()

	// Find workspace by domain
	for workspaceID, workspace := range warl.workspaceConfigs {
		if workspace.Domain == domain {
			return workspaceID
		}
	}
	return ""
}

// initializeSenderCount initializes rate limit counters for a specific sender
func (warl *WorkspaceAwareRateLimiter) initializeSenderCount(workspaceID, senderEmail string, count int) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("ERROR: Panic during rate limiter initialization for %s:%s - %v", workspaceID, senderEmail, r)
		}
	}()

	// Get workspace config with lock protection
	warl.mu.RLock()
	workspace, exists := warl.workspaceConfigs[workspaceID]
	warl.mu.RUnlock()

	if !exists {
		log.Printf("Warning: Cannot initialize rate limiter - workspace %s not found", workspaceID)
		return
	}

	if workspace == nil {
		log.Printf("Error: Workspace config for %s is nil", workspaceID)
		return
	}

	log.Printf("STEP 1: Getting user limit for %s", senderEmail)
	// Safety check: Cap initialization to prevent memory issues
	// Limit to 2x the daily limit to handle edge cases
	userLimit := warl.getUserLimit(workspace, senderEmail)
	log.Printf("STEP 1 COMPLETE: User limit is %d", userLimit)

	if userLimit <= 0 {
		log.Printf("Warning: User limit for %s in workspace %s is %d, using default", senderEmail, workspaceID, userLimit)
		userLimit = warl.globalDefault
	}

	maxInitCount := userLimit * 2
	if count > maxInitCount {
		log.Printf("Warning: Capping initialization count from %d to %d for %s in workspace %s", count, maxInitCount, senderEmail, workspaceID)
		count = maxInitCount
	}

	log.Printf("STEP 2: Checking workspace-level limiter (WorkspaceDaily=%d)", workspace.RateLimits.WorkspaceDaily)
	// Initialize workspace-level limiter if configured
	if workspace.RateLimits.WorkspaceDaily > 0 {
		log.Printf("STEP 2a: Getting workspace limiter...")
		workspaceLimiter := warl.getWorkspaceLimiter(workspaceID, workspace.RateLimits.WorkspaceDaily)
		log.Printf("STEP 2b: Got workspace limiter, recording...")
		// Cap workspace count to workspace limit * 2
		wsCount := count
		if wsCount > workspace.RateLimits.WorkspaceDaily*2 {
			wsCount = workspace.RateLimits.WorkspaceDaily * 2
		}
		workspaceLimiter.Record(wsCount)
		log.Printf("STEP 2c: Workspace limiter record complete")
	}

	log.Printf("STEP 3: Getting user-level limiter...")
	// Initialize user-level limiter
	limiter := warl.getLimiterForSender(workspaceID, senderEmail, userLimit)
	log.Printf("STEP 4: Recording %d messages for user limiter...", count)
	limiter.Record(count)
	log.Printf("STEP 4 COMPLETE: User limiter record complete")

	log.Printf("Successfully initialized rate limiter for %s:%s with %d messages", workspaceID, senderEmail, count)
}

func (warl *WorkspaceAwareRateLimiter) GetGlobalStatus() (totalSent int, workspaceCount int, workspaces map[string]WorkspaceStats) {
	warl.mu.RLock()
	defer warl.mu.RUnlock()

	workspaces = make(map[string]WorkspaceStats)
	totalSent = 0
	workspaceCount = len(warl.workspaceConfigs)

	for workspaceID, workspace := range warl.workspaceConfigs {
		stats := WorkspaceStats{
			WorkspaceID: workspaceID,
			DisplayName: workspace.DisplayName,
			Domain:      workspace.Domain,
			Users:       make(map[string]SenderStats),
		}

		// Get workspace-level stats if configured
		if workspace.RateLimits.WorkspaceDaily > 0 {
			if limiter, exists := warl.workspaceLimiters[workspaceID]; exists {
				sent, remaining, resetTime := limiter.GetStatus()
				stats.WorkspaceSent = sent
				stats.WorkspaceRemaining = remaining
				stats.WorkspaceLimit = workspace.RateLimits.WorkspaceDaily
				stats.WorkspaceResetTime = resetTime
				totalSent += sent
			}
		}

		// Get per-user stats for this workspace
		for key, limiter := range warl.limiters {
			if strings.HasPrefix(key, workspaceID+":") {
				email := strings.TrimPrefix(key, workspaceID+":")
				sent, remaining, resetTime := limiter.GetStatus()

				userLimit := warl.getUserLimit(workspace, email)
				stats.Users[email] = SenderStats{
					Email:     email,
					Sent:      sent,
					Remaining: remaining,
					Limit:     userLimit,
					ResetTime: resetTime,
				}

				if workspace.RateLimits.WorkspaceDaily <= 0 {
					totalSent += sent // Only add if not already counted at workspace level
				}
			}
		}

		workspaces[workspaceID] = stats
	}

	return
}

func (warl *WorkspaceAwareRateLimiter) getUserLimit(workspace *config.WorkspaceConfig, senderEmail string) int {
	// Priority order: Custom user limit > Per-user workspace limit > Global default

	// Check custom user limit first
	if workspace.RateLimits.CustomUserLimits != nil {
		if customLimit, exists := workspace.RateLimits.CustomUserLimits[senderEmail]; exists && customLimit > 0 {
			return customLimit
		}
	}

	// Check per-user workspace limit
	if workspace.RateLimits.PerUserDaily > 0 {
		return workspace.RateLimits.PerUserDaily
	}

	// Fall back to global default
	return warl.globalDefault
}

func (warl *WorkspaceAwareRateLimiter) getLimiterForSender(workspaceID, senderEmail string, limit int) *RateLimiter {
	key := fmt.Sprintf("%s:%s", workspaceID, senderEmail)

	warl.mu.RLock()
	if limiter, exists := warl.limiters[key]; exists {
		warl.mu.RUnlock()
		return limiter
	}
	warl.mu.RUnlock()

	// Need to create new limiter
	warl.mu.Lock()
	defer warl.mu.Unlock()

	// Double-check in case another goroutine created it
	if limiter, exists := warl.limiters[key]; exists {
		return limiter
	}

	// Create new limiter for this sender with the determined limit
	limiter := NewRateLimiter(limit)
	warl.limiters[key] = limiter
	return limiter
}

func (warl *WorkspaceAwareRateLimiter) getWorkspaceLimiter(workspaceID string, limit int) *RateLimiter {
	warl.mu.RLock()
	if limiter, exists := warl.workspaceLimiters[workspaceID]; exists {
		warl.mu.RUnlock()
		return limiter
	}
	warl.mu.RUnlock()

	// Need to create new workspace limiter
	warl.mu.Lock()
	defer warl.mu.Unlock()

	// Double-check in case another goroutine created it
	if limiter, exists := warl.workspaceLimiters[workspaceID]; exists {
		return limiter
	}

	// Create new workspace limiter
	limiter := NewRateLimiter(limit)
	warl.workspaceLimiters[workspaceID] = limiter
	return limiter
}

// WorkspaceStats represents rate limit statistics for a workspace
type WorkspaceStats struct {
	WorkspaceID        string                 `json:"workspace_id"`
	DisplayName        string                 `json:"display_name"`
	Domain             string                 `json:"domain"`
	WorkspaceSent      int                    `json:"workspace_sent"`
	WorkspaceRemaining int                    `json:"workspace_remaining"`
	WorkspaceLimit     int                    `json:"workspace_limit"`
	WorkspaceResetTime time.Time              `json:"workspace_reset_time"`
	Users              map[string]SenderStats `json:"users"`
}
