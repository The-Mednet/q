package mailgun

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"relay/internal/config"
	"relay/internal/gateway"
	"relay/pkg/models"
)

// MailgunClient implements the GatewayInterface for Mailgun
type MailgunClient struct {
	config     *config.MailgunConfig
	gatewayID  string
	domain     string
	httpClient *http.Client

	// State management
	mu        sync.RWMutex
	status    gateway.GatewayStatus
	lastError error
	metrics   gateway.GatewayMetrics

	// Rate limiting
	rateLimit gateway.RateLimit

	// Circuit breaker (will be injected)
	circuitBreaker gateway.CircuitBreakerInterface

	// Configuration
	priority int
	weight   int
	features []gateway.GatewayFeature
}

// MailgunResponse represents the response from Mailgun API
type MailgunResponse struct {
	ID      string `json:"id"`
	Message string `json:"message"`
}

// MailgunErrorResponse represents an error response from Mailgun
type MailgunErrorResponse struct {
	Message string `json:"message"`
}

// NewMailgunClient creates a new Mailgun client
func NewMailgunClient(gatewayID string, config *config.MailgunConfig, rateLimit gateway.RateLimit, priority, weight int) (*MailgunClient, error) {
	if config == nil {
		return nil, fmt.Errorf("mailgun config cannot be nil")
	}

	if config.APIKey == "" {
		return nil, fmt.Errorf("mailgun API key is required")
	}

	if config.Domain == "" {
		return nil, fmt.Errorf("mailgun domain is required")
	}

	// Default base URL if not specified
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://api.mailgun.net/v3"
	}

	client := &MailgunClient{
		config:    config,
		gatewayID: gatewayID,
		domain:    config.Domain,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		status:    gateway.GatewayStatusHealthy,
		rateLimit: rateLimit,
		priority:  priority,
		weight:    weight,
		features: []gateway.GatewayFeature{
			gateway.FeatureTracking,
			gateway.FeatureTags,
			gateway.FeatureMetadata,
			gateway.FeatureWebhooks,
			gateway.FeatureDomainKeys,
			gateway.FeatureAnalytics,
			gateway.FeatureAttachments,
		},
		metrics: gateway.GatewayMetrics{
			TotalSent:        0,
			TotalFailed:      0,
			TotalRateLimited: 0,
			SuccessRate:      100.0,
			AverageLatency:   0,
			Uptime:           time.Since(time.Now()),
			ErrorRate:        0.0,
		},
	}

	return client, nil
}

// SendMessage implements GatewayInterface.SendMessage
func (mc *MailgunClient) SendMessage(ctx context.Context, msg *models.Message) (*gateway.SendResult, error) {
	startTime := time.Now()

	// Check if we can send (rate limiting handled by circuit breaker execution)
	sendFunc := func() error {
		return mc.sendMessageInternal(ctx, msg)
	}

	var err error
	if mc.circuitBreaker != nil {
		err = mc.circuitBreaker.Execute(ctx, sendFunc)
	} else {
		err = sendFunc()
	}

	duration := time.Since(startTime)

	result := &gateway.SendResult{
		GatewayID:   mc.gatewayID,
		GatewayType: gateway.GatewayTypeMailgun,
		SendTime:    duration,
		Metadata: map[string]interface{}{
			"domain": mc.domain,
			"region": mc.config.Region,
		},
	}

	// Update metrics
	mc.mu.Lock()
	if err != nil {
		mc.metrics.TotalFailed++
		mc.lastError = err
		result.Status = "failed"
		result.Error = stringPtr(err.Error())
	} else {
		mc.metrics.TotalSent++
		mc.lastError = nil
		result.Status = "sent"
		now := time.Now()
		mc.metrics.LastSent = &now
	}

	// Update success rate
	total := mc.metrics.TotalSent + mc.metrics.TotalFailed
	if total > 0 {
		mc.metrics.SuccessRate = float64(mc.metrics.TotalSent) / float64(total) * 100
		mc.metrics.ErrorRate = float64(mc.metrics.TotalFailed) / float64(total) * 100
	}

	// Update average latency (simple moving average)
	if mc.metrics.AverageLatency == 0 {
		mc.metrics.AverageLatency = duration
	} else {
		mc.metrics.AverageLatency = (mc.metrics.AverageLatency + duration) / 2
	}
	mc.mu.Unlock()

	return result, err
}

// sendMessageInternal performs the actual Mailgun API call
func (mc *MailgunClient) sendMessageInternal(ctx context.Context, msg *models.Message) error {
	// Prepare the form data
	form := make(url.Values)

	// Required fields
	form.Set("from", mc.formatFromAddress(msg))
	form.Set("to", strings.Join(msg.To, ","))
	form.Set("subject", msg.Subject)

	// Optional recipients
	if len(msg.CC) > 0 {
		form.Set("cc", strings.Join(msg.CC, ","))
	}
	if len(msg.BCC) > 0 {
		form.Set("bcc", strings.Join(msg.BCC, ","))
	}

	// Body content
	if msg.HTML != "" {
		form.Set("html", msg.HTML)
	}
	if msg.Text != "" {
		form.Set("text", msg.Text)
	}

	// Add tracking if enabled
	if mc.config.Tracking.Opens {
		form.Set("o:tracking-opens", "true")
	}
	if mc.config.Tracking.Clicks {
		form.Set("o:tracking-clicks", "htmlonly")
	}

	// Add unsubscribe functionality if enabled
	log.Printf("DEBUG: Mailgun unsubscribe tracking enabled: %v", mc.config.Tracking.Unsubscribe)
	if mc.config.Tracking.Unsubscribe {
		form.Set("o:tracking-unsubscribe", "true")
		log.Printf("DEBUG: Added unsubscribe tracking to Mailgun request")
		// Optional: Set custom unsubscribe URL if you want to handle it yourself
		// form.Set("h:List-Unsubscribe", "<https://your-domain.com/unsubscribe?email=%recipient%>")
	}

	// Skip adding tags - user preference to not include Mailgun tags

	// Add custom variables for invitation tracking
	if msg.InvitationID != "" {
		form.Set("v:invitation_id", msg.InvitationID)
	}
	if msg.EmailType != "" {
		form.Set("v:email_type", msg.EmailType)
	}
	if msg.InvitationDispatchID != "" {
		form.Set("v:invitation_dispatch_id", msg.InvitationDispatchID)
	}
	if msg.ID != "" {
		form.Set("v:message_id", msg.ID)
	}

	// Add metadata as custom variables
	for key, value := range msg.Metadata {
		if strValue, ok := value.(string); ok {
			form.Set(fmt.Sprintf("v:%s", key), strValue)
		}
	}

	// Add custom headers from message
	if len(msg.Headers) > 0 {
		log.Printf("DEBUG: Gateway Mailgun processing %d headers for message %s", len(msg.Headers), msg.ID)
		for headerName, headerValue := range msg.Headers {
			// Skip headers that Mailgun handles separately to avoid RFC 5322 violations
			headerLower := strings.ToLower(headerName)
			switch headerLower {
			case "content-type", "to", "from", "subject", "cc", "bcc", "date", "message-id":
				log.Printf("DEBUG: Gateway skipping standard email header: %s", headerName)
				continue
			}
			
			// Add as custom header with h: prefix for Mailgun
			form.Set(fmt.Sprintf("h:%s", headerName), headerValue)
			log.Printf("Gateway added custom header %s: %s for Mailgun domain %s", headerName, headerValue, mc.domain)
		}
	}

	// Create the request
	apiURL := fmt.Sprintf("%s/%s/messages", mc.config.BaseURL, mc.domain)
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth("api", mc.config.APIKey)

	// Send the request
	resp, err := mc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Handle response
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var mailgunResp MailgunResponse
		if err := json.Unmarshal(body, &mailgunResp); err != nil {
			// If we can't parse the response but the status is success, consider it sent
			return nil
		}
		// Success
		return nil
	}

	// Handle error response
	var errorResp MailgunErrorResponse
	if err := json.Unmarshal(body, &errorResp); err != nil {
		return fmt.Errorf("mailgun API error (status: %d): %s", resp.StatusCode, string(body))
	}

	return fmt.Errorf("mailgun API error (status: %d): %s", resp.StatusCode, errorResp.Message)
}

// formatFromAddress formats the from address, potentially rewriting for domain consistency
func (mc *MailgunClient) formatFromAddress(msg *models.Message) string {
	// Check if we need to rewrite the domain
	fromParts := strings.Split(msg.From, "@")
	if len(fromParts) != 2 {
		return msg.From // Invalid format, return as-is
	}

	localPart := fromParts[0]

	// Use the configured domain for sending
	rewrittenFrom := fmt.Sprintf("%s@%s", localPart, mc.domain)

	// If there's a display name in headers, use it
	if msg.Headers != nil {
		if senderName, exists := msg.Headers["X-Sender-Name"]; exists {
			return fmt.Sprintf("%s <%s>", senderName, rewrittenFrom)
		}
		if fromHeader, exists := msg.Headers["From"]; exists && strings.Contains(fromHeader, "<") {
			// Extract display name from existing From header
			if idx := strings.Index(fromHeader, "<"); idx > 0 {
				displayName := strings.TrimSpace(fromHeader[:idx])
				return fmt.Sprintf("%s <%s>", displayName, rewrittenFrom)
			}
		}
	}

	return rewrittenFrom
}


// GetType implements GatewayInterface.GetType
func (mc *MailgunClient) GetType() gateway.GatewayType {
	return gateway.GatewayTypeMailgun
}

// GetID implements GatewayInterface.GetID
func (mc *MailgunClient) GetID() string {
	return mc.gatewayID
}

// HealthCheck implements GatewayInterface.HealthCheck
func (mc *MailgunClient) HealthCheck(ctx context.Context) error {
	// Perform a simple API call to check if Mailgun is accessible
	apiURL := fmt.Sprintf("%s/%s", mc.config.BaseURL, mc.domain)
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		mc.setStatus(gateway.GatewayStatusUnhealthy, err)
		return fmt.Errorf("failed to create health check request: %w", err)
	}

	req.SetBasicAuth("api", mc.config.APIKey)

	resp, err := mc.httpClient.Do(req)
	if err != nil {
		mc.setStatus(gateway.GatewayStatusUnhealthy, err)
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		mc.setStatus(gateway.GatewayStatusUnhealthy, fmt.Errorf("authentication failed"))
		return fmt.Errorf("mailgun authentication failed")
	} else if resp.StatusCode >= 500 {
		mc.setStatus(gateway.GatewayStatusDegraded, fmt.Errorf("server error: %d", resp.StatusCode))
		return fmt.Errorf("mailgun server error: %d", resp.StatusCode)
	}

	mc.setStatus(gateway.GatewayStatusHealthy, nil)
	return nil
}

// GetStatus implements GatewayInterface.GetStatus
func (mc *MailgunClient) GetStatus() gateway.GatewayStatus {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return mc.status
}

// GetLastError implements GatewayInterface.GetLastError
func (mc *MailgunClient) GetLastError() error {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return mc.lastError
}

// GetRateLimit implements GatewayInterface.GetRateLimit
func (mc *MailgunClient) GetRateLimit() gateway.RateLimit {
	return mc.rateLimit
}

// CanSend implements GatewayInterface.CanSend
func (mc *MailgunClient) CanSend(ctx context.Context, senderEmail string) (bool, error) {
	// This would integrate with the rate limiter
	// For now, just check if we're healthy
	return mc.status == gateway.GatewayStatusHealthy, nil
}

// CanRoute implements GatewayInterface.CanRoute
func (mc *MailgunClient) CanRoute(senderEmail string) bool {
	// Mailgun can typically route any email by rewriting the domain
	// This would be configured via the routing configuration
	return true
}

// GetPriority implements GatewayInterface.GetPriority
func (mc *MailgunClient) GetPriority() int {
	return mc.priority
}

// GetWeight implements GatewayInterface.GetWeight
func (mc *MailgunClient) GetWeight() int {
	return mc.weight
}

// GetMetrics implements GatewayInterface.GetMetrics
func (mc *MailgunClient) GetMetrics() gateway.GatewayMetrics {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return mc.metrics
}

// GetSupportedFeatures implements GatewayInterface.GetSupportedFeatures
func (mc *MailgunClient) GetSupportedFeatures() []gateway.GatewayFeature {
	return mc.features
}

// setStatus updates the gateway status
func (mc *MailgunClient) setStatus(status gateway.GatewayStatus, err error) {
	mc.mu.Lock()
	mc.status = status
	if err != nil {
		mc.lastError = err
	}
	mc.mu.Unlock()
}

// SetCircuitBreaker sets the circuit breaker for this gateway
func (mc *MailgunClient) SetCircuitBreaker(cb gateway.CircuitBreakerInterface) {
	mc.circuitBreaker = cb
}

// Helper function
func stringPtr(s string) *string {
	return &s
}
