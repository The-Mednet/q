package processor

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"smtp_relay/internal/config"
	"smtp_relay/internal/gmail"
	"smtp_relay/internal/llm"
	"smtp_relay/internal/queue"
	"smtp_relay/internal/webhook"
	"smtp_relay/pkg/models"
)

type QueueProcessor struct {
	queue         queue.Queue
	config        *config.Config
	gmailClient   *gmail.Client
	webhookClient *webhook.Client
	personalizer  *llm.Personalizer
	rateLimiter   *queue.WorkspaceAwareRateLimiter

	mu         sync.Mutex
	processing bool
	lastRun    time.Time
	stats      ProcessStats
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

	processor := &QueueProcessor{
		queue:         q,
		config:        cfg,
		gmailClient:   gc,
		webhookClient: wc,
		personalizer:  p,
		rateLimiter:   queue.NewWorkspaceAwareRateLimiter(workspaces, cfg.Queue.DailyRateLimit),
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
		}
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

			if p.webhookClient != nil {
				p.webhookClient.SendDeferredEvent(ctx, msg, "Authentication error")
			}
		} else {
			log.Printf("Error sending message %s: %v", msg.ID, err)
			p.queue.UpdateStatus(msg.ID, models.StatusFailed, err)

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

	// Record successful send for rate limiting
	p.rateLimiter.RecordSend(msg.WorkspaceID, msg.From)

	// Send success webhook
	if p.webhookClient != nil {
		err = p.webhookClient.SendSentEvent(ctx, msg)
		if err != nil {
			log.Printf("Error sending webhook for message %s: %v", msg.ID, err)
		}
	}

	return nil
}

func (p *QueueProcessor) GetStatus() (bool, time.Time, ProcessStats) {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.processing, p.lastRun, p.stats
}

func (p *QueueProcessor) GetRateLimitStatus() (totalSent int, workspaceCount int, workspaces map[string]queue.WorkspaceStats) {
	return p.rateLimiter.GetGlobalStatus()
}
