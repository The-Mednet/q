package migration

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"relay/internal/config"
	"relay/internal/gateway"
	"relay/internal/gmail"
	"relay/internal/processor"
	"relay/pkg/models"
)

// LegacyGmailWrapper wraps the existing Gmail client to implement GatewayInterface
type LegacyGmailWrapper struct {
	client      *gmail.Client
	workspaceID string
	domain      string
	priority    int
	weight      int
	status      gateway.GatewayStatus
	rateLimit   gateway.RateLimit
	features    []gateway.GatewayFeature
}

// NewLegacyGmailWrapper creates a wrapper for the existing Gmail client
func NewLegacyGmailWrapper(client *gmail.Client, workspace *config.WorkspaceConfig, priority int) *LegacyGmailWrapper {
	rateLimit := gateway.RateLimit{
		DailyLimit:   workspace.RateLimits.WorkspaceDaily,
		PerUserLimit: workspace.RateLimits.PerUserDaily,
		CustomLimits: workspace.RateLimits.CustomUserLimits,
	}

	// Set default limits if not specified
	if rateLimit.DailyLimit == 0 {
		rateLimit.DailyLimit = 2000 // Default Gmail API limit
	}
	if rateLimit.PerUserLimit == 0 {
		rateLimit.PerUserLimit = 200 // Conservative per-user default
	}

	return &LegacyGmailWrapper{
		client:      client,
		workspaceID: workspace.ID,
		domain:      workspace.Domain,
		priority:    priority,
		weight:      100, // Default weight
		status:      gateway.GatewayStatusHealthy,
		rateLimit:   rateLimit,
		features: []gateway.GatewayFeature{
			gateway.FeatureDomainKeys,  // Google handles DKIM
			gateway.FeatureAttachments, // Gmail supports attachments
			gateway.FeatureMetadata,    // Can store in headers
		},
	}
}

// SendMessage implements GatewayInterface.SendMessage
func (lgw *LegacyGmailWrapper) SendMessage(ctx context.Context, msg *models.Message) (*gateway.SendResult, error) {
	start := time.Now()

	err := lgw.client.SendMessage(ctx, msg)

	sendDuration := time.Since(start)

	result := &gateway.SendResult{
		GatewayID:   lgw.workspaceID,
		GatewayType: gateway.GatewayTypeGoogleWorkspace,
		SendTime:    sendDuration,
		Metadata: map[string]interface{}{
			"provider_id": lgw.workspaceID,
			"domain":       lgw.domain,
		},
	}

	if err != nil {
		result.Status = "failed"
		result.Error = stringPtr(err.Error())
		lgw.status = gateway.GatewayStatusDegraded // Could implement more sophisticated status tracking
	} else {
		result.Status = "sent"
		result.MessageID = msg.ID // Gmail doesn't return a separate message ID
		lgw.status = gateway.GatewayStatusHealthy
	}

	return result, err
}

// GetType implements GatewayInterface.GetType
func (lgw *LegacyGmailWrapper) GetType() gateway.GatewayType {
	return gateway.GatewayTypeGoogleWorkspace
}

// GetID implements GatewayInterface.GetID
func (lgw *LegacyGmailWrapper) GetID() string {
	return lgw.workspaceID
}

// HealthCheck implements GatewayInterface.HealthCheck
func (lgw *LegacyGmailWrapper) HealthCheck(ctx context.Context) error {
	err := lgw.client.ValidateServiceAccount(ctx)
	if err != nil {
		lgw.status = gateway.GatewayStatusUnhealthy
		return fmt.Errorf("google workspace health check failed: %w", err)
	}

	lgw.status = gateway.GatewayStatusHealthy
	return nil
}

// GetStatus implements GatewayInterface.GetStatus
func (lgw *LegacyGmailWrapper) GetStatus() gateway.GatewayStatus {
	return lgw.status
}

// GetLastError implements GatewayInterface.GetLastError
func (lgw *LegacyGmailWrapper) GetLastError() error {
	// Legacy client doesn't track last error, would need to be enhanced
	return nil
}

// GetRateLimit implements GatewayInterface.GetRateLimit
func (lgw *LegacyGmailWrapper) GetRateLimit() gateway.RateLimit {
	return lgw.rateLimit
}

// CanSend implements GatewayInterface.CanSend
func (lgw *LegacyGmailWrapper) CanSend(ctx context.Context, senderEmail string) (bool, error) {
	// Check if the sender domain matches this workspace
	return lgw.CanRoute(senderEmail), nil
}

// CanRoute implements GatewayInterface.CanRoute
func (lgw *LegacyGmailWrapper) CanRoute(senderEmail string) bool {
	// Extract domain from sender email
	parts := strings.Split(senderEmail, "@")
	if len(parts) != 2 {
		return false
	}

	senderDomain := parts[1]
	return strings.EqualFold(senderDomain, lgw.domain)
}

// GetPriority implements GatewayInterface.GetPriority
func (lgw *LegacyGmailWrapper) GetPriority() int {
	return lgw.priority
}

// GetWeight implements GatewayInterface.GetWeight
func (lgw *LegacyGmailWrapper) GetWeight() int {
	return lgw.weight
}

// GetMetrics implements GatewayInterface.GetMetrics
func (lgw *LegacyGmailWrapper) GetMetrics() gateway.GatewayMetrics {
	// Legacy client doesn't track detailed metrics
	// This would need to be enhanced for full metrics support
	return gateway.GatewayMetrics{
		TotalSent:      0,     // Would need to be tracked
		TotalFailed:    0,     // Would need to be tracked
		SuccessRate:    100.0, // Optimistic default
		AverageLatency: 0,
		Uptime:         0,
		ErrorRate:      0.0,
	}
}

// GetSupportedFeatures implements GatewayInterface.GetSupportedFeatures
func (lgw *LegacyGmailWrapper) GetSupportedFeatures() []gateway.GatewayFeature {
	return lgw.features
}

// MigrationManager handles the migration from legacy to new gateway system
type MigrationManager struct {
	legacyGmailConfig *config.GmailConfig
	newGatewayConfig  *config.EnhancedGatewayConfig
	migrationMode     MigrationMode
}

// MigrationMode defines different migration strategies
type MigrationMode string

const (
	MigrationModeCompatibility MigrationMode = "compatibility" // Run both systems in parallel
	MigrationModeGradual       MigrationMode = "gradual"       // Gradually move domains to new system
	MigrationModeImmediate     MigrationMode = "immediate"     // Switch immediately to new system
)

// NewMigrationManager creates a new migration manager
func NewMigrationManager(legacyConfig *config.GmailConfig, newConfig *config.EnhancedGatewayConfig, mode MigrationMode) *MigrationManager {
	return &MigrationManager{
		legacyGmailConfig: legacyConfig,
		newGatewayConfig:  newConfig,
		migrationMode:     mode,
	}
}

// CreateCompatibilityGateways creates gateway wrappers for existing Gmail workspaces
func (mm *MigrationManager) CreateCompatibilityGateways(gmailClient *gmail.Client) ([]gateway.GatewayInterface, error) {
	if mm.legacyGmailConfig == nil {
		return nil, fmt.Errorf("no legacy Gmail configuration provided")
	}

	var gateways []gateway.GatewayInterface

	for i, workspace := range mm.legacyGmailConfig.Workspaces {
		wrapper := NewLegacyGmailWrapper(gmailClient, &workspace, i+1)
		gateways = append(gateways, wrapper)

		log.Printf("Created compatibility gateway for workspace %s (domain: %s)", workspace.ID, workspace.Domain)
	}

	return gateways, nil
}

// ShouldUseLegacyGateway determines if a message should use the legacy gateway system
func (mm *MigrationManager) ShouldUseLegacyGateway(msg *models.Message) bool {
	switch mm.migrationMode {
	case MigrationModeCompatibility:
		// In compatibility mode, use new system by default but fall back to legacy for specific cases
		return mm.requiresLegacyHandling(msg)

	case MigrationModeGradual:
		// In gradual mode, check if this domain has been migrated
		return !mm.isDomainMigrated(msg.From)

	case MigrationModeImmediate:
		// In immediate mode, never use legacy system
		return false

	default:
		// Default to legacy system for safety
		return true
	}
}

// requiresLegacyHandling checks if a message requires legacy handling
func (mm *MigrationManager) requiresLegacyHandling(msg *models.Message) bool {
	// Example criteria for requiring legacy handling:

	// 1. Messages with complex formatting that might not be handled correctly
	if len(msg.Attachments) > 0 {
		// For now, handle attachments through legacy system
		return true
	}

	// 2. Messages from specific domains that need special handling
	if mm.isLegacyOnlyDomain(msg.From) {
		return true
	}

	// 3. Messages with specific headers or metadata
	if msg.Headers != nil {
		if _, hasLegacyFlag := msg.Headers["X-Use-Legacy"]; hasLegacyFlag {
			return true
		}
	}

	return false
}

// isDomainMigrated checks if a domain has been migrated to the new system
func (mm *MigrationManager) isDomainMigrated(senderEmail string) bool {
	if mm.newGatewayConfig == nil {
		return false
	}

	parts := strings.Split(senderEmail, "@")
	if len(parts) != 2 {
		return false
	}

	domain := parts[1]

	// Check if any new gateway can handle this domain
	for _, gateway := range mm.newGatewayConfig.Gateways {
		if !gateway.Enabled {
			continue
		}

		// Check routing patterns
		for _, pattern := range gateway.Routing.CanRoute {
			if pattern == "*" || strings.EqualFold(pattern, "@"+domain) {
				return true
			}
		}
	}

	return false
}

// isLegacyOnlyDomain checks if a domain should only use legacy system
func (mm *MigrationManager) isLegacyOnlyDomain(senderEmail string) bool {
	parts := strings.Split(senderEmail, "@")
	if len(parts) != 2 {
		return false
	}

	domain := parts[1]

	// Example: internal domains might need to stay on legacy system
	legacyOnlyDomains := []string{
		"internal.joinmednet.org",
		"secure.joinmednet.org",
	}

	for _, legacyDomain := range legacyOnlyDomains {
		if strings.EqualFold(domain, legacyDomain) {
			return true
		}
	}

	return false
}

// GetMigrationStatus returns the current migration status
func (mm *MigrationManager) GetMigrationStatus() MigrationStatus {
	status := MigrationStatus{
		Mode:            mm.migrationMode,
		LegacyGateways:  0,
		NewGateways:     0,
		MigratedDomains: []string{},
		PendingDomains:  []string{},
	}

	// Count legacy gateways
	if mm.legacyGmailConfig != nil {
		status.LegacyGateways = len(mm.legacyGmailConfig.Workspaces)

		// Collect legacy domains
		for _, workspace := range mm.legacyGmailConfig.Workspaces {
			if mm.isDomainMigrated("user@" + workspace.Domain) {
				status.MigratedDomains = append(status.MigratedDomains, workspace.Domain)
			} else {
				status.PendingDomains = append(status.PendingDomains, workspace.Domain)
			}
		}
	}

	// Count new gateways
	if mm.newGatewayConfig != nil {
		for _, gateway := range mm.newGatewayConfig.Gateways {
			if gateway.Enabled {
				status.NewGateways++
			}
		}
	}

	return status
}

// MigrationStatus represents the current state of the migration
type MigrationStatus struct {
	Mode            MigrationMode `json:"mode"`
	LegacyGateways  int           `json:"legacy_gateways"`
	NewGateways     int           `json:"new_gateways"`
	MigratedDomains []string      `json:"migrated_domains"`
	PendingDomains  []string      `json:"pending_domains"`
}

// BackwardCompatibilityProcessor wraps the existing processor to support both systems
type BackwardCompatibilityProcessor struct {
	legacyProcessor   *processor.QueueProcessor // Reference to existing processor
	newGatewayManager gateway.GatewayManager
	migrationManager  *MigrationManager
}

// NewBackwardCompatibilityProcessor creates a processor that supports both systems
func NewBackwardCompatibilityProcessor(
	legacyProcessor *processor.QueueProcessor,
	newGatewayManager gateway.GatewayManager,
	migrationManager *MigrationManager,
) *BackwardCompatibilityProcessor {
	return &BackwardCompatibilityProcessor{
		legacyProcessor:   legacyProcessor,
		newGatewayManager: newGatewayManager,
		migrationManager:  migrationManager,
	}
}

// ProcessMessage processes a message using the appropriate system
func (bcp *BackwardCompatibilityProcessor) ProcessMessage(ctx context.Context, msg *models.Message) error {
	if bcp.migrationManager.ShouldUseLegacyGateway(msg) {
		log.Printf("Processing message %s via legacy system", msg.ID)
		// Use the Process method which handles the full queue processing
		// For individual message processing, we'd need to add the message to queue first
		return fmt.Errorf("legacy individual message processing not implemented - use queue-based processing")
	} else {
		log.Printf("Processing message %s via new gateway system", msg.ID)
		return bcp.processMessageNewSystem(ctx, msg)
	}
}

// processMessageNewSystem processes a message using the new gateway system
func (bcp *BackwardCompatibilityProcessor) processMessageNewSystem(ctx context.Context, msg *models.Message) error {
	// This would implement the new gateway processing logic
	// For now, return an error to indicate it's not yet implemented
	return fmt.Errorf("new gateway system processing not yet implemented")
}

// Helper functions
func stringPtr(s string) *string {
	return &s
}

// SendTime helper type for timing
type SendTime struct {
	start time.Time
}

func NewSendTime() SendTime {
	return SendTime{start: time.Now()}
}

func (st SendTime) Duration() time.Duration {
	return time.Since(st.start)
}
