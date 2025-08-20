package processor

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"relay/internal/blaster"
	"relay/internal/config"
	"relay/internal/llm"
	"relay/internal/provider"
	"relay/internal/queue"
	"relay/internal/recipient"
	"relay/internal/variables"
	"relay/internal/webhook"
	"relay/internal/workspace"
	"relay/pkg/models"
)

// UnifiedProcessor handles email processing using the unified provider system
type UnifiedProcessor struct {
	// Core components
	queue            queue.Queue
	config           *config.Config
	providerRouter   *provider.Router
	workspaceManager *workspace.Manager
	
	// Optional services
	webhookClient    *webhook.Client
	personalizer     *llm.Personalizer
	variableReplacer *variables.VariableReplacer
	rateLimiter      *queue.WorkspaceAwareRateLimiter
	recipientService *recipient.Service
	
	// Processing control
	mu         sync.Mutex
	processing bool
	lastRun    time.Time
	stats      UnifiedProcessStats
	ctx        context.Context
	cancel     context.CancelFunc
}

// UnifiedProcessStats tracks processing statistics for the unified processor
type UnifiedProcessStats struct {
	TotalProcessed  int
	Sent            int
	Failed          int
	RateLimited     int
	LastProcessedAt time.Time
	ProviderStats   map[string]ProviderProcessStats
}

// ProviderProcessStats tracks stats per provider
type ProviderProcessStats struct {
	Sent   int
	Failed int
}

// NewUnifiedProcessor creates a new unified processor
func NewUnifiedProcessor(
	q queue.Queue,
	cfg *config.Config,
	workspaceManager *workspace.Manager,
	providerRouter *provider.Router,
	wc *webhook.Client,
	p *llm.Personalizer,
	rs *recipient.Service,
) *UnifiedProcessor {
	// Create workspace map for rate limiter
	workspaces := make(map[string]*config.WorkspaceConfig)
	allWorkspaces := workspaceManager.GetAllWorkspaces()
	
	log.Printf("Loading %d workspaces for unified processor rate limiter", len(allWorkspaces))
	for id, workspace := range allWorkspaces {
		log.Printf("Adding workspace: ID='%s', Domain='%s'", id, workspace.Domain)
		workspaces[id] = workspace
	}
	
	// Initialize variable replacer if blaster is configured
	var variableReplacer *variables.VariableReplacer
	if cfg.Blaster.BaseURL != "" && cfg.Blaster.APIKey != "" {
		trendingClient := blaster.NewTrendingClient(cfg.Blaster.BaseURL, cfg.Blaster.APIKey)
		variableReplacer = variables.NewVariableReplacer(trendingClient)
		log.Printf("Variable replacer initialized with blaster API at %s", cfg.Blaster.BaseURL)
	} else {
		log.Printf("Variable replacer disabled - blaster API not configured")
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	
	processor := &UnifiedProcessor{
		queue:            q,
		config:           cfg,
		providerRouter:   providerRouter,
		workspaceManager: workspaceManager,
		webhookClient:    wc,
		personalizer:     p,
		variableReplacer: variableReplacer,
		rateLimiter:      queue.NewWorkspaceAwareRateLimiter(workspaces, cfg.Queue.DailyRateLimit),
		recipientService: rs,
		ctx:              ctx,
		cancel:           cancel,
		stats: UnifiedProcessStats{
			ProviderStats: make(map[string]ProviderProcessStats),
		},
	}
	
	// Initialize rate limiter with historical data from the queue
	log.Printf("Initializing unified processor rate limiter with historical data...")
	if err := processor.rateLimiter.InitializeFromQueue(processor.queue); err != nil {
		log.Printf("Warning: Failed to initialize rate limiter: %v", err)
	} else {
		log.Printf("Unified processor rate limiter successfully initialized")
	}
	
	return processor
}

// Start begins the processing loop
func (p *UnifiedProcessor) Start() {
	log.Println("Starting unified message processor...")
	
	ticker := time.NewTicker(p.config.Queue.ProcessInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			if err := p.Process(); err != nil {
				log.Printf("Error during processing cycle: %v", err)
			}
		case <-p.ctx.Done():
			log.Println("Unified processor stopping...")
			return
		}
	}
}

// Stop gracefully stops the processor
func (p *UnifiedProcessor) Stop() {
	log.Println("Stopping unified processor...")
	if p.cancel != nil {
		p.cancel()
	}
}

// Process handles a batch of messages from the queue
func (p *UnifiedProcessor) Process() error {
	p.mu.Lock()
	if p.processing {
		p.mu.Unlock()
		return fmt.Errorf("unified processing already in progress")
	}
	p.processing = true
	p.lastRun = time.Now()
	p.mu.Unlock()
	
	defer func() {
		p.mu.Lock()
		p.processing = false
		p.mu.Unlock()
	}()
	
	log.Println("Starting unified queue processing...")
	
	// Reset stats for this run
	stats := UnifiedProcessStats{
		LastProcessedAt: time.Now(),
		ProviderStats:   make(map[string]ProviderProcessStats),
	}
	
	// Dequeue messages
	messages, err := p.queue.Dequeue(p.config.Queue.BatchSize)
	if err != nil {
		log.Printf("Error dequeuing messages: %v", err)
		return err
	}
	
	if len(messages) == 0 {
		return nil // No messages to process
	}
	
	log.Printf("Processing %d messages from queue", len(messages))
	
	// Process each message
	for _, msg := range messages {
		stats.TotalProcessed++
		
		// Process recipient information for this message (defensive programming - continue on error)
		if p.recipientService != nil {
			if err := p.recipientService.ProcessMessageRecipients(msg); err != nil {
				log.Printf("Warning: Failed to process recipients for message %s: %v", msg.ID, err)
				// Continue processing the message even if recipient tracking fails
			}
		}
		
		// Check rate limit for this sender (workspace-aware)
		if !p.rateLimiter.Allow(msg.WorkspaceID, msg.From) {
			log.Printf("Rate limit exceeded for sender %s in workspace %s (message %s)", msg.From, msg.WorkspaceID, msg.ID)
			stats.RateLimited++
			
			// Put back in queue as deferred
			p.queue.UpdateStatus(msg.ID, models.StatusQueued, fmt.Errorf("rate limit exceeded for sender %s in workspace %s", msg.From, msg.WorkspaceID))
			
			if p.webhookClient != nil {
				p.webhookClient.SendDeferredEvent(context.Background(), msg, fmt.Sprintf("Rate limit exceeded for %s in workspace %s", msg.From, msg.WorkspaceID))
			}
			
			// Log rate limit status for this sender
			sent, remaining, resetTime := p.rateLimiter.GetStatus(msg.WorkspaceID, msg.From)
			log.Printf("Rate limit status for %s in workspace %s: %d sent, %d remaining, resets at %s",
				msg.From, msg.WorkspaceID, sent, remaining, resetTime.Format(time.RFC3339))
			
			continue
		}
		
		// Process the message
		providerID, err := p.processMessage(msg)
		if err != nil {
			stats.Failed++
			if providerID != "" {
				providerStats := stats.ProviderStats[providerID]
				providerStats.Failed++
				stats.ProviderStats[providerID] = providerStats
			}
		} else {
			stats.Sent++
			if providerID != "" {
				providerStats := stats.ProviderStats[providerID]
				providerStats.Sent++
				stats.ProviderStats[providerID] = providerStats
			}
		}
	}
	
	// Update global stats
	p.mu.Lock()
	p.stats = stats
	p.mu.Unlock()
	
	log.Printf("Unified queue processing completed: %d total, %d sent, %d failed, %d rate limited",
		stats.TotalProcessed, stats.Sent, stats.Failed, stats.RateLimited)
	
	// Log provider-specific stats
	for providerID, providerStats := range stats.ProviderStats {
		log.Printf("Provider %s: %d sent, %d failed", providerID, providerStats.Sent, providerStats.Failed)
	}
	
	return nil
}

// processMessage processes a single message
func (p *UnifiedProcessor) processMessage(msg *models.Message) (string, error) {
	ctx := context.Background()
	
	// Apply variable replacement first (before personalization)
	if p.variableReplacer != nil {
		err := p.variableReplacer.ReplaceVariables(ctx, msg)
		if err != nil {
			log.Printf("Warning: Failed to replace variables in message %s: %v", msg.ID, err)
			// Continue with original message if variable replacement fails
		}
	}
	
	// Validate that all variables have been resolved
	if err := variables.ValidateNoUnresolvedVariables(msg); err != nil {
		log.Printf("Error: Message %s contains unresolved variables: %v", msg.ID, err)
		p.queue.UpdateStatus(msg.ID, models.StatusFailed, err)
		
		if p.webhookClient != nil {
			p.webhookClient.SendRejectEvent(ctx, msg, fmt.Sprintf("Unresolved variables: %v", err))
		}
		return "", fmt.Errorf("message contains unresolved variables: %w", err)
	}
	
	// Apply LLM personalization if enabled
	if p.personalizer != nil && p.personalizer.IsEnabled() {
		err := p.personalizer.PersonalizeMessage(ctx, msg)
		if err != nil {
			log.Printf("Warning: Failed to personalize message %s: %v", msg.ID, err)
			// Continue with original message if personalization fails
		}
	}
	
	// Route message to appropriate provider
	selectedProvider, err := p.providerRouter.RouteMessage(ctx, msg)
	if err != nil {
		log.Printf("Error: Failed to route message %s: %v", msg.ID, err)
		p.queue.UpdateStatus(msg.ID, models.StatusFailed, err)
		
		// Update recipient delivery status
		p.updateRecipientDeliveryStatus(msg, models.DeliveryStatusFailed, err.Error())
		
		if p.webhookClient != nil {
			p.webhookClient.SendRejectEvent(ctx, msg, fmt.Sprintf("Routing failed: %v", err))
		}
		return "", fmt.Errorf("failed to route message: %w", err)
	}
	
	providerID := selectedProvider.GetID()
	
	// Send via selected provider
	err = selectedProvider.SendMessage(ctx, msg)
	if err != nil {
		if strings.Contains(err.Error(), "authentication") || strings.Contains(err.Error(), "unauthorized") {
			log.Printf("Authentication error for message %s via provider %s: %v", msg.ID, providerID, err)
			p.queue.UpdateStatus(msg.ID, models.StatusAuthError, err)
			
			// Update recipient delivery status
			p.updateRecipientDeliveryStatus(msg, models.DeliveryStatusDeferred, err.Error())
			
			if p.webhookClient != nil {
				p.webhookClient.SendDeferredEvent(ctx, msg, "Authentication error")
			}
		} else {
			log.Printf("Error sending message %s via provider %s: %v", msg.ID, providerID, err)
			p.queue.UpdateStatus(msg.ID, models.StatusFailed, err)
			
			// Update recipient delivery status - determine if bounce or general failure
			deliveryStatus := models.DeliveryStatusFailed
			if strings.Contains(strings.ToLower(err.Error()), "bounce") ||
				strings.Contains(strings.ToLower(err.Error()), "invalid") ||
				strings.Contains(strings.ToLower(err.Error()), "not exist") {
				deliveryStatus = models.DeliveryStatusBounced
			}
			p.updateRecipientDeliveryStatus(msg, deliveryStatus, err.Error())
			
			if p.webhookClient != nil {
				p.webhookClient.SendBounceEvent(ctx, msg, err.Error())
			}
		}
		return providerID, err
	}
	
	// Mark as sent
	err = p.queue.UpdateStatus(msg.ID, models.StatusSent, nil)
	if err != nil {
		log.Printf("Error updating message status: %v", err)
	}
	
	// Update recipient delivery status to SENT
	p.updateRecipientDeliveryStatus(msg, models.DeliveryStatusSent, "")
	
	// Record successful send for rate limiting
	p.rateLimiter.RecordSend(msg.WorkspaceID, msg.From)
	
	// Send success webhook
	if p.webhookClient != nil {
		err = p.webhookClient.SendSentEvent(ctx, msg)
		if err != nil {
			log.Printf("Error sending webhook for message %s: %v", msg.ID, err)
		}
	}
	
	log.Printf("Successfully sent message %s via provider %s (%s)", msg.ID, providerID, selectedProvider.GetType())
	
	return providerID, nil
}

// updateRecipientDeliveryStatus updates the delivery status for all recipients of a message
func (p *UnifiedProcessor) updateRecipientDeliveryStatus(msg *models.Message, status models.DeliveryStatus, errorReason string) {
	if p.recipientService == nil {
		return // Recipient service not available
	}
	
	// Prepare bounce reason if this is an error
	var bounceReason *string
	if errorReason != "" {
		bounceReason = &errorReason
	}
	
	// Update all recipient types (TO, CC, BCC)
	allRecipients := append(append(msg.To, msg.CC...), msg.BCC...)
	
	for _, email := range allRecipients {
		if email == "" {
			continue
		}
		
		email = strings.TrimSpace(strings.ToLower(email))
		if err := p.recipientService.UpdateDeliveryStatus(msg.ID, email, status, bounceReason); err != nil {
			log.Printf("Warning: Failed to update delivery status for recipient %s: %v", email, err)
		}
	}
}

// GetStatus returns the current processing status
func (p *UnifiedProcessor) GetStatus() (bool, time.Time, any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	return p.processing, p.lastRun, p.stats
}

// GetRateLimitStatus returns rate limiting statistics
func (p *UnifiedProcessor) GetRateLimitStatus() (totalSent int, workspaceCount int, workspaces map[string]queue.WorkspaceStats) {
	return p.rateLimiter.GetGlobalStatus()
}

// GetProviderStats returns statistics about provider usage
func (p *UnifiedProcessor) GetProviderStats() map[string]interface{} {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	return map[string]interface{}{
		"provider_stats": p.stats.ProviderStats,
		"router_stats":   p.providerRouter.GetStats(),
	}
}

// HealthCheckProviders performs health checks on all providers
func (p *UnifiedProcessor) HealthCheckProviders() map[string]error {
	return p.providerRouter.HealthCheckAll(context.Background())
}