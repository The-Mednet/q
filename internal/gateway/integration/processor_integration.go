package integration

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"relay/internal/blaster"
	"relay/internal/config"
	"relay/internal/gateway"
	"relay/internal/gateway/mailgun"
	"relay/internal/gateway/manager"
	"relay/internal/gateway/ratelimit"
	"relay/internal/gateway/reliability"
	"relay/internal/gateway/router"
	"relay/internal/llm"
	"relay/internal/queue"
	"relay/internal/recipient"
	"relay/internal/variables"
	"relay/internal/webhook"
	"relay/pkg/models"
)

// EnhancedQueueProcessor replaces the original QueueProcessor with gateway support
type EnhancedQueueProcessor struct {
	// Core components
	queue            queue.Queue
	config           *config.Config
	webhookClient    *webhook.Client
	personalizer     *llm.Personalizer
	variableReplacer *variables.VariableReplacer
	recipientService *recipient.Service

	// New gateway components
	gatewayManager  gateway.GatewayManager
	gatewayRouter   gateway.GatewayRouter
	rateLimiter     *ratelimit.MultiGatewayRateLimiter
	circuitBreakers *reliability.CircuitBreakerManager

	// State management
	mu         sync.Mutex
	processing bool
	lastRun    time.Time
	stats      EnhancedProcessStats
	ctx        context.Context
	cancel     context.CancelFunc
}

// EnhancedProcessStats extends the original ProcessStats
type EnhancedProcessStats struct {
	TotalProcessed  int
	Sent            int
	Failed          int
	RateLimited     int
	LastProcessedAt time.Time

	// New gateway-specific stats
	GatewayStats        map[string]GatewayProcessStats
	CircuitBreakerTrips int
	FailoverCount       int
	AverageLatency      time.Duration
}

// GatewayProcessStats tracks per-gateway processing statistics
type GatewayProcessStats struct {
	GatewayID      string
	GatewayType    string
	Sent           int
	Failed         int
	RateLimited    int
	AverageLatency time.Duration
	HealthStatus   gateway.GatewayStatus
}

// MigrationMode defines how legacy and new systems coexist
type MigrationMode string

const (
	MigrationModeLegacyOnly MigrationMode = "legacy_only"
	MigrationModeNewOnly    MigrationMode = "new_only"
	MigrationModeHybrid     MigrationMode = "hybrid"
)

// NewEnhancedQueueProcessor creates a new enhanced processor
func NewEnhancedQueueProcessor(
	q queue.Queue,
	cfg *config.Config,
	wc *webhook.Client,
	p *llm.Personalizer,
	rs *recipient.Service,
	gatewayConfig *config.EnhancedGatewayConfig,
) (*EnhancedQueueProcessor, error) {

	// Create circuit breaker manager
	cbManager := reliability.NewCircuitBreakerManager(gatewayConfig.GlobalDefaults.CircuitBreaker)

	// Create rate limiter
	rateLimiter := ratelimit.NewMultiGatewayRateLimiter(&gatewayConfig.GlobalDefaults, q)

	// Create gateway router
	gatewayRouter := router.NewGatewayRouter(gatewayConfig.Routing.Strategy, cbManager)

	// Create gateway manager
	gatewayManager := manager.NewGatewayManager(gatewayRouter, rateLimiter, cbManager)

	// Initialize variable replacer if blaster is configured
	var variableReplacer *variables.VariableReplacer
	if cfg.Blaster.BaseURL != "" && cfg.Blaster.APIKey != "" {
		trendingClient := blaster.NewTrendingClient(cfg.Blaster.BaseURL, cfg.Blaster.APIKey)
		variableReplacer = variables.NewVariableReplacer(trendingClient)
	}

	ctx, cancel := context.WithCancel(context.Background())
	
	processor := &EnhancedQueueProcessor{
		queue:            q,
		config:           cfg,
		webhookClient:    wc,
		personalizer:     p,
		variableReplacer: variableReplacer,
		recipientService: rs,
		gatewayManager:   gatewayManager,
		gatewayRouter:    gatewayRouter,
		rateLimiter:      rateLimiter,
		circuitBreakers:  cbManager,
		ctx:              ctx,
		cancel:           cancel,
		stats: EnhancedProcessStats{
			GatewayStats: make(map[string]GatewayProcessStats),
		},
	}

	// Register gateways from configuration
	if err := processor.registerGateways(gatewayConfig); err != nil {
		return nil, fmt.Errorf("failed to register gateways: %w", err)
	}

	// Initialize rate limiter with historical data
	if err := rateLimiter.InitializeFromQueue(); err != nil {
		log.Printf("Warning: Failed to initialize rate limiter from queue: %v", err)
	}

	// Start health monitoring
	processor.startHealthMonitoring()

	return processor, nil
}

// Start begins queue processing with enhanced gateway support
func (eqp *EnhancedQueueProcessor) Start() {
	log.Println("Starting enhanced queue processor...")

	// Start health monitoring if not already started
	if eqp.circuitBreakers != nil {
		eqp.startHealthMonitoring()
	}

	// Main processing loop
	ticker := time.NewTicker(eqp.config.Queue.ProcessInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := eqp.Process(); err != nil {
				log.Printf("Error during enhanced queue processing: %v", err)
			}
		case <-eqp.ctx.Done():
			log.Println("Enhanced queue processor stopping...")
			return
		}
	}
}

// Stop gracefully stops the enhanced queue processor
func (eqp *EnhancedQueueProcessor) Stop() {
	if eqp.cancel != nil {
		eqp.cancel()
	}
}

// registerGateways registers all gateways from configuration
func (eqp *EnhancedQueueProcessor) registerGateways(config *config.EnhancedGatewayConfig) error {
	for _, gatewayConfig := range config.Gateways {
		if !gatewayConfig.Enabled {
			continue
		}

		// Create gateway based on type
		gw, err := eqp.createGateway(&gatewayConfig)
		if err != nil {
			return fmt.Errorf("failed to create gateway %s: %w", gatewayConfig.ID, err)
		}

		// Register with manager
		if err := eqp.gatewayManager.RegisterGateway(gw); err != nil {
			return fmt.Errorf("failed to register gateway %s: %w", gatewayConfig.ID, err)
		}

		// Register with router (cast to implementation type)
		if routerImpl, ok := eqp.gatewayRouter.(*router.GatewayRouterImpl); ok {
			if err := routerImpl.RegisterGateway(gw, &gatewayConfig); err != nil {
				return fmt.Errorf("failed to register gateway with router %s: %w", gatewayConfig.ID, err)
			}
		}

		// Register rate limiting
		rateLimits := gatewayConfig.GetEffectiveRateLimits(&config.GlobalDefaults)
		if err := eqp.rateLimiter.RegisterGateway(gatewayConfig.ID, gw.GetType(), rateLimits); err != nil {
			return fmt.Errorf("failed to register rate limiting for gateway %s: %w", gatewayConfig.ID, err)
		}

		log.Printf("Registered gateway: %s (%s)", gatewayConfig.ID, gatewayConfig.Type)
	}

	return nil
}

// createGateway creates a gateway instance based on configuration
func (eqp *EnhancedQueueProcessor) createGateway(config *config.GatewayConfig) (gateway.GatewayInterface, error) {
	switch config.Type {
	case gateway.GatewayTypeGoogleWorkspace:
		return eqp.createGoogleWorkspaceGateway(config)

	case gateway.GatewayTypeMailgun:
		return eqp.createMailgunGateway(config)

	default:
		return nil, fmt.Errorf("unsupported gateway type: %s", config.Type)
	}
}

// createGoogleWorkspaceGateway creates a Google Workspace gateway
func (eqp *EnhancedQueueProcessor) createGoogleWorkspaceGateway(config *config.GatewayConfig) (gateway.GatewayInterface, error) {
	if config.GoogleWorkspace == nil {
		return nil, fmt.Errorf("Google Workspace configuration missing")
	}

	// This would create a new Google Workspace gateway implementation
	// For now, we'll use the legacy wrapper during migration
	return nil, fmt.Errorf("Google Workspace gateway implementation pending")
}

// createMailgunGateway creates a Mailgun gateway
func (eqp *EnhancedQueueProcessor) createMailgunGateway(config *config.GatewayConfig) (gateway.GatewayInterface, error) {
	if config.Mailgun == nil {
		return nil, fmt.Errorf("Mailgun configuration missing")
	}

	// Create rate limit configuration
	rateLimit := gateway.RateLimit{
		DailyLimit:   config.RateLimits.WorkspaceDaily,
		PerUserLimit: config.RateLimits.PerUserDaily,
		PerHourLimit: config.RateLimits.PerHour,
		BurstLimit:   config.RateLimits.BurstLimit,
		CustomLimits: config.RateLimits.CustomUserLimits,
	}

	return mailgun.NewMailgunClient(
		config.ID,
		config.Mailgun,
		rateLimit,
		config.Priority,
		config.Weight,
	)
}

// Process processes messages in the queue with enhanced gateway support
func (eqp *EnhancedQueueProcessor) Process() error {
	eqp.mu.Lock()
	if eqp.processing {
		eqp.mu.Unlock()
		return fmt.Errorf("queue processing already in progress")
	}
	eqp.processing = true
	eqp.lastRun = time.Now()
	eqp.mu.Unlock()

	defer func() {
		eqp.mu.Lock()
		eqp.processing = false
		eqp.mu.Unlock()
	}()

	log.Println("Starting enhanced queue processing...")

	// Reset stats for this run
	stats := EnhancedProcessStats{
		LastProcessedAt: time.Now(),
		GatewayStats:    make(map[string]GatewayProcessStats),
	}

	messages, err := eqp.queue.Dequeue(eqp.config.Queue.BatchSize)
	if err != nil {
		log.Printf("Error dequeuing messages: %v", err)
		return err
	}

	for _, msg := range messages {
		stats.TotalProcessed++

		startTime := time.Now()
		err := eqp.processMessage(msg)
		processingTime := time.Since(startTime)

		// Update overall stats
		if err != nil {
			stats.Failed++
		} else {
			stats.Sent++
		}

		// Update average latency
		if stats.AverageLatency == 0 {
			stats.AverageLatency = processingTime
		} else {
			stats.AverageLatency = (stats.AverageLatency + processingTime) / 2
		}
	}

	// Update overall stats
	eqp.mu.Lock()
	eqp.stats = stats
	eqp.mu.Unlock()

	log.Printf("Enhanced queue processing completed: %d total, %d sent, %d failed, %d circuit breaker trips, %d failovers",
		stats.TotalProcessed, stats.Sent, stats.Failed, stats.CircuitBreakerTrips, stats.FailoverCount)

	return nil
}

// processMessage processes a single message using the enhanced gateway system
func (eqp *EnhancedQueueProcessor) processMessage(msg *models.Message) error {
	ctx := context.Background()

	// Process recipient information
	if eqp.recipientService != nil {
		if err := eqp.recipientService.ProcessMessageRecipients(msg); err != nil {
			log.Printf("Warning: Failed to process recipients for message %s: %v", msg.ID, err)
		}
	}

	// Apply variable replacement
	if eqp.variableReplacer != nil {
		if err := eqp.variableReplacer.ReplaceVariables(ctx, msg); err != nil {
			log.Printf("Warning: Failed to replace variables in message %s: %v", msg.ID, err)
		}
	}

	// Validate variables
	if err := variables.ValidateNoUnresolvedVariables(msg); err != nil {
		log.Printf("Error: Message %s contains unresolved variables: %v", msg.ID, err)
		eqp.queue.UpdateStatus(msg.ID, models.StatusFailed, err)
		eqp.sendWebhookEvent(ctx, msg, "reject", err.Error())
		return err
	}

	// Apply LLM personalization if enabled
	if eqp.personalizer != nil && eqp.personalizer.IsEnabled() {
		if err := eqp.personalizer.PersonalizeMessage(ctx, msg); err != nil {
			log.Printf("Warning: Failed to personalize message %s: %v", msg.ID, err)
		}
	}

	// Route message to appropriate gateway
	selectedGateway, err := eqp.gatewayRouter.RouteMessage(ctx, msg)
	if err != nil {
		log.Printf("Error routing message %s: %v", msg.ID, err)
		eqp.queue.UpdateStatus(msg.ID, models.StatusFailed, err)
		eqp.updateRecipientDeliveryStatusWithGateway(msg, models.DeliveryStatusFailed, err.Error(), "", "")
		eqp.sendWebhookEvent(ctx, msg, "reject", err.Error())
		return err
	}

	// Check rate limits
	rateLimitResult := eqp.rateLimiter.Allow(selectedGateway.GetID(), msg.From)
	if !rateLimitResult.Allowed {
		log.Printf("Rate limit exceeded for message %s: %s", msg.ID, rateLimitResult.Reason)
		eqp.queue.UpdateStatus(msg.ID, models.StatusQueued, fmt.Errorf("rate limit exceeded: %s", rateLimitResult.Reason))
		eqp.sendWebhookEvent(ctx, msg, "defer", rateLimitResult.Reason)

		// Update stats
		eqp.updateGatewayStats(selectedGateway.GetID(), "rate_limited")
		return fmt.Errorf("rate limit exceeded: %s", rateLimitResult.Reason)
	}

	// Send via selected gateway
	sendResult, err := selectedGateway.SendMessage(ctx, msg)
	if err != nil {
		eqp.handleSendError(ctx, msg, selectedGateway, err)
		return err
	}

	// Success - update status and tracking
	eqp.handleSendSuccess(ctx, msg, selectedGateway, sendResult)
	return nil
}

// handleSendError handles send failures with appropriate status updates
func (eqp *EnhancedQueueProcessor) handleSendError(ctx context.Context, msg *models.Message, gw gateway.GatewayInterface, err error) {
	gatewayID := gw.GetID()

	// Determine error type and status
	var status models.MessageStatus
	var deliveryStatus models.DeliveryStatus
	var webhookEvent string

	if isAuthError(err) {
		status = models.StatusAuthError
		deliveryStatus = models.DeliveryStatusDeferred
		webhookEvent = "defer"
	} else if isBounceError(err) {
		status = models.StatusFailed
		deliveryStatus = models.DeliveryStatusBounced
		webhookEvent = "bounce"
	} else {
		status = models.StatusFailed
		deliveryStatus = models.DeliveryStatusFailed
		webhookEvent = "reject"
	}

	// Update message status
	eqp.queue.UpdateStatus(msg.ID, status, err)

	// Update recipient delivery status with gateway information
	eqp.updateRecipientDeliveryStatusWithGateway(msg, deliveryStatus, err.Error(), gatewayID, gw.GetType())

	// Send webhook
	eqp.sendWebhookEvent(ctx, msg, webhookEvent, err.Error())

	// Update gateway-specific stats
	eqp.updateGatewayStats(gatewayID, "failed")

	log.Printf("Error sending message %s via gateway %s (%s): %v",
		msg.ID, gatewayID, gw.GetType(), err)
}

// handleSendSuccess handles successful sends
func (eqp *EnhancedQueueProcessor) handleSendSuccess(ctx context.Context, msg *models.Message, gw gateway.GatewayInterface, result *gateway.SendResult) {
	gatewayID := gw.GetID()

	// Update message status
	eqp.queue.UpdateStatus(msg.ID, models.StatusSent, nil)

	// Record successful send for rate limiting
	eqp.rateLimiter.RecordSend(gatewayID, msg.From)

	// Update recipient delivery status with gateway information
	eqp.updateRecipientDeliveryStatusWithGateway(msg, models.DeliveryStatusSent, "", gatewayID, gw.GetType())

	// Send success webhook
	eqp.sendWebhookEvent(ctx, msg, "send", "")

	// Update gateway-specific stats
	eqp.updateGatewayStats(gatewayID, "sent")

	log.Printf("Successfully sent message %s via gateway %s (%s) in %v",
		msg.ID, gatewayID, gw.GetType(), result.SendTime)
}

// updateRecipientDeliveryStatusWithGateway updates recipient status with gateway information
func (eqp *EnhancedQueueProcessor) updateRecipientDeliveryStatusWithGateway(
	msg *models.Message,
	status models.DeliveryStatus,
	errorReason string,
	gatewayID string,
	gatewayType gateway.GatewayType,
) {
	if eqp.recipientService == nil {
		return
	}

	var bounceReason *string
	if errorReason != "" {
		// Include gateway information in bounce reason
		fullReason := fmt.Sprintf("[%s:%s] %s", gatewayType, gatewayID, errorReason)
		bounceReason = &fullReason
	}

	// Update all recipient types (TO, CC, BCC)
	allRecipients := append(append(msg.To, msg.CC...), msg.BCC...)

	for _, email := range allRecipients {
		if email == "" {
			continue
		}

		email = strings.TrimSpace(strings.ToLower(email))

		// Update delivery status (this would be enhanced to include gateway info)
		if err := eqp.recipientService.UpdateDeliveryStatus(msg.ID, email, status, bounceReason); err != nil {
			log.Printf("Warning: Failed to update delivery status for recipient %s: %v", email, err)
		}

		// Record gateway used for this delivery (this would require database schema extension)
		if err := eqp.recordGatewayUsage(msg.ID, email, gatewayID, string(gatewayType)); err != nil {
			log.Printf("Warning: Failed to record gateway usage: %v", err)
		}
	}
}

// recordGatewayUsage records which gateway was used for delivery (future enhancement)
func (eqp *EnhancedQueueProcessor) recordGatewayUsage(messageID, recipientEmail, gatewayID, gatewayType string) error {
	// This would be implemented to track gateway usage per recipient
	// Could be added to the message_recipients table or a separate gateway_usage table
	return nil
}

// sendWebhookEvent sends webhook events with enhanced information
func (eqp *EnhancedQueueProcessor) sendWebhookEvent(ctx context.Context, msg *models.Message, eventType, reason string) {
	if eqp.webhookClient == nil {
		return
	}

	var err error
	switch eventType {
	case "send":
		err = eqp.webhookClient.SendSentEvent(ctx, msg)
	case "bounce":
		err = eqp.webhookClient.SendBounceEvent(ctx, msg, reason)
	case "reject":
		err = eqp.webhookClient.SendRejectEvent(ctx, msg, reason)
	case "defer":
		err = eqp.webhookClient.SendDeferredEvent(ctx, msg, reason)
	}

	if err != nil {
		log.Printf("Error sending webhook for message %s: %v", msg.ID, err)
	}
}

// updateGatewayStats updates per-gateway statistics
func (eqp *EnhancedQueueProcessor) updateGatewayStats(gatewayID, eventType string) {
	eqp.mu.Lock()
	defer eqp.mu.Unlock()

	stats, exists := eqp.stats.GatewayStats[gatewayID]
	if !exists {
		stats = GatewayProcessStats{
			GatewayID: gatewayID,
		}
	}

	switch eventType {
	case "sent":
		stats.Sent++
	case "failed":
		stats.Failed++
	case "rate_limited":
		stats.RateLimited++
	}

	eqp.stats.GatewayStats[gatewayID] = stats
}

// startHealthMonitoring starts health monitoring for all gateways
func (eqp *EnhancedQueueProcessor) startHealthMonitoring() {
	go func() {
		ticker := time.NewTicker(30 * time.Second) // Health check interval
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				eqp.performHealthChecks()
			}
		}
	}()
}

// performHealthChecks performs health checks on all registered gateways
func (eqp *EnhancedQueueProcessor) performHealthChecks() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	gateways := eqp.gatewayManager.GetAllGateways()
	for _, gw := range gateways {
		gatewayID := gw.GetID()

		err := gw.HealthCheck(ctx)

		var status gateway.GatewayStatus
		if err != nil {
			status = gateway.GatewayStatusUnhealthy
			log.Printf("Health check failed for gateway %s: %v", gatewayID, err)
		} else {
			status = gateway.GatewayStatusHealthy
		}

		// Update router with health status
		eqp.gatewayRouter.UpdateGatewayHealth(gatewayID, status, err)

		// Update stats
		eqp.mu.Lock()
		if stats, exists := eqp.stats.GatewayStats[gatewayID]; exists {
			stats.HealthStatus = status
			eqp.stats.GatewayStats[gatewayID] = stats
		}
		eqp.mu.Unlock()
	}
}

// GetStatus returns processing status compatible with the expected interface
func (eqp *EnhancedQueueProcessor) GetStatus() (bool, time.Time, any) {
	eqp.mu.Lock()
	defer eqp.mu.Unlock()

	// Return enhanced stats as generic interface
	return eqp.processing, eqp.lastRun, eqp.stats
}

// GetEnhancedStatus returns enhanced processing status with full details
func (eqp *EnhancedQueueProcessor) GetEnhancedStatus() (bool, time.Time, EnhancedProcessStats, map[string]interface{}) {
	eqp.mu.Lock()
	defer eqp.mu.Unlock()

	// Get gateway statistics
	gatewayStats := make(map[string]interface{})
	// TODO: Implement GetGatewayStats method in router
	// if eqp.gatewayRouter != nil {
	//     gatewayStats = eqp.gatewayRouter.GetGatewayStats()
	// }

	return eqp.processing, eqp.lastRun, eqp.stats, gatewayStats
}

// Helper functions
func isAuthError(err error) bool {
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "authentication") ||
		strings.Contains(errStr, "unauthorized") ||
		strings.Contains(errStr, "invalid credentials")
}

func isBounceError(err error) bool {
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "bounce") ||
		strings.Contains(errStr, "invalid email") ||
		strings.Contains(errStr, "does not exist")
}
