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
	"relay/internal/gmail"
	"relay/internal/llm"
	"relay/internal/queue"
	"relay/internal/recipient"
	"relay/internal/variables"
	"relay/internal/webhook"
	"relay/pkg/models"
)

type QueueProcessor struct {
	queue            queue.Queue
	config           *config.Config
	gmailClient      *gmail.Client
	webhookClient    *webhook.Client
	personalizer     *llm.Personalizer
	variableReplacer *variables.VariableReplacer
	rateLimiter      *queue.WorkspaceAwareRateLimiter
	recipientService *recipient.Service

	mu         sync.Mutex
	processing bool
	lastRun    time.Time
	stats      ProcessStats
	ctx        context.Context
	cancel     context.CancelFunc
}

type ProcessStats struct {
	TotalProcessed  int
	Sent            int
	Failed          int
	RateLimited     int
	LastProcessedAt time.Time
}

func NewQueueProcessor(
	q queue.Queue,
	cfg *config.Config,
	gc *gmail.Client,
	wc *webhook.Client,
	p *llm.Personalizer,
	rs *recipient.Service,
) *QueueProcessor {
	// Create workspace map for rate limiter
	workspaces := make(map[string]*config.WorkspaceConfig)
	log.Printf("Loading %d workspaces for rate limiter", len(cfg.Gmail.Workspaces))
	for i := range cfg.Gmail.Workspaces {
		ws := &cfg.Gmail.Workspaces[i]
		if ws.ID == "" {
			log.Printf("Warning: Workspace %d has empty ID, skipping", i)
			continue
		}
		log.Printf("Adding workspace: ID='%s', Domain='%s'", ws.ID, ws.Domain)
		workspaces[ws.ID] = ws
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
	
	processor := &QueueProcessor{
		queue:            q,
		config:           cfg,
		gmailClient:      gc,
		webhookClient:    wc,
		personalizer:     p,
		variableReplacer: variableReplacer,
		rateLimiter:      queue.NewWorkspaceAwareRateLimiter(workspaces, cfg.Queue.DailyRateLimit),
		recipientService: rs,
		ctx:              ctx,
		cancel:           cancel,
	}

	// Initialize rate limiter with historical data from the queue
	log.Printf("Initializing rate limiter with historical data...")
	if err := processor.rateLimiter.InitializeFromQueue(processor.queue); err != nil {
		log.Printf("Warning: Failed to initialize rate limiter: %v", err)
	} else {
		log.Printf("Rate limiter successfully initialized")
	}

	return processor
}

func (p *QueueProcessor) Start() {
	ticker := time.NewTicker(p.config.Queue.ProcessInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.Process()
		case <-p.ctx.Done():
			log.Println("Queue processor stopping...")
			return
		}
	}
}

// Stop gracefully stops the queue processor
func (p *QueueProcessor) Stop() {
	if p.cancel != nil {
		p.cancel()
	}
}

func (p *QueueProcessor) Process() error {
	p.mu.Lock()
	if p.processing {
		p.mu.Unlock()
		return fmt.Errorf("queue processing already in progress")
	}
	p.processing = true
	p.lastRun = time.Now()
	p.mu.Unlock()

	defer func() {
		p.mu.Lock()
		p.processing = false
		p.mu.Unlock()
	}()

	log.Println("Starting queue processing...")

	// Reset stats for this run
	stats := ProcessStats{
		LastProcessedAt: time.Now(),
	}

	messages, err := p.queue.Dequeue(p.config.Queue.BatchSize)
	if err != nil {
		log.Printf("Error dequeuing messages: %v", err)
		return err
	}

	for _, msg := range messages {
		stats.TotalProcessed++

		// Process recipient information for this message (defensive programming - continue on error)
		if p.recipientService != nil {
			if err := p.recipientService.ProcessMessageRecipients(msg); err != nil {
				log.Printf("Warning: Failed to process recipients for message %s: %v", msg.ID, err)
				// Continue processing the message even if recipient tracking fails
			}
		}

		// Check rate limit for this sender (provider-aware)
		if !p.rateLimiter.Allow(msg.ProviderID, msg.From) {
			log.Printf("Rate limit exceeded for sender %s in provider %s (message %s)", msg.From, msg.ProviderID, msg.ID)
			stats.RateLimited++

			// Put back in queue as deferred
			p.queue.UpdateStatus(msg.ID, models.StatusQueued, fmt.Errorf("rate limit exceeded for sender %s in provider %s", msg.From, msg.ProviderID))

			if p.webhookClient != nil {
				p.webhookClient.SendDeferredEvent(context.Background(), msg, fmt.Sprintf("Rate limit exceeded for %s in provider %s", msg.From, msg.ProviderID))
			}

			// Log rate limit status for this sender
			sent, remaining, resetTime := p.rateLimiter.GetStatus(msg.ProviderID, msg.From)
			log.Printf("Rate limit status for %s in provider %s: %d sent, %d remaining, resets at %s",
				msg.From, msg.ProviderID, sent, remaining, resetTime.Format(time.RFC3339))

			continue
		}

		// Process the message
		err := p.processMessage(msg)
		if err != nil {
			stats.Failed++
		} else {
			stats.Sent++
		}
	}

	// Update stats
	p.mu.Lock()
	p.stats = stats
	p.mu.Unlock()

	log.Printf("Queue processing completed: %d total, %d sent, %d failed, %d rate limited",
		stats.TotalProcessed, stats.Sent, stats.Failed, stats.RateLimited)

	return nil
}

func (p *QueueProcessor) processMessage(msg *models.Message) error {
	ctx := context.Background()

	if p.gmailClient == nil {
		log.Printf("Gmail client not initialized, marking message as failed")
		p.queue.UpdateStatus(msg.ID, models.StatusFailed, fmt.Errorf("Gmail client not initialized"))

		if p.webhookClient != nil {
			p.webhookClient.SendRejectEvent(ctx, msg, "Gmail client not initialized")
		}
		return fmt.Errorf("Gmail client not initialized")
	}

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
		return fmt.Errorf("message contains unresolved variables: %w", err)
	}

	// Apply LLM personalization if enabled
	if p.personalizer != nil && p.personalizer.IsEnabled() {
		err := p.personalizer.PersonalizeMessage(ctx, msg)
		if err != nil {
			log.Printf("Warning: Failed to personalize message %s: %v", msg.ID, err)
			// Continue with original message if personalization fails
		}
	}

	// Send via Gmail
	err := p.gmailClient.SendMessage(ctx, msg)
	if err != nil {
		if strings.Contains(err.Error(), "authentication") || strings.Contains(err.Error(), "unauthorized") {
			log.Printf("Authentication error for message %s: %v", msg.ID, err)
			p.queue.UpdateStatus(msg.ID, models.StatusAuthError, err)

			// Update recipient delivery status
			p.updateRecipientDeliveryStatus(msg, models.DeliveryStatusDeferred, err.Error())

			if p.webhookClient != nil {
				p.webhookClient.SendDeferredEvent(ctx, msg, "Authentication error")
			}
		} else {
			log.Printf("Error sending message %s: %v", msg.ID, err)
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
		return err
	}

	// Mark as sent
	err = p.queue.UpdateStatus(msg.ID, models.StatusSent, nil)
	if err != nil {
		log.Printf("Error updating message status: %v", err)
	}

	// Update recipient delivery status to SENT
	p.updateRecipientDeliveryStatus(msg, models.DeliveryStatusSent, "")

	// Record successful send for rate limiting
	p.rateLimiter.RecordSend(msg.ProviderID, msg.From)

	// Send success webhook
	if p.webhookClient != nil {
		err = p.webhookClient.SendSentEvent(ctx, msg)
		if err != nil {
			log.Printf("Error sending webhook for message %s: %v", msg.ID, err)
		}
	}

	return nil
}

// updateRecipientDeliveryStatus updates the delivery status for all recipients of a message
func (p *QueueProcessor) updateRecipientDeliveryStatus(msg *models.Message, status models.DeliveryStatus, errorReason string) {
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

func (p *QueueProcessor) GetStatus() (bool, time.Time, any) {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.processing, p.lastRun, p.stats
}

func (p *QueueProcessor) GetRateLimitStatus() (totalSent int, workspaceCount int, workspaces map[string]queue.WorkspaceStats) {
	return p.rateLimiter.GetGlobalStatus()
}
