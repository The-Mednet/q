package provider

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
	"relay/pkg/models"
)

// MailgunProvider implements the Provider interface for Mailgun
type MailgunProvider struct {
	id              string
	workspaceID     string
	config          *config.WorkspaceMailgunConfig
	domains         []string
	displayName     string
	httpClient      *http.Client
	
	// Health monitoring
	mu              sync.RWMutex
	healthy         bool
	lastHealthCheck time.Time
	lastError       error
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

// NewMailgunProvider creates a new Mailgun provider instance
func NewMailgunProvider(workspaceID string, domain string, config *config.WorkspaceMailgunConfig) (*MailgunProvider, error) {
	if config == nil {
		return nil, fmt.Errorf("Mailgun config cannot be nil")
	}
	
	if !config.Enabled {
		return nil, fmt.Errorf("Mailgun is disabled for workspace %s", workspaceID)
	}
	
	if config.APIKey == "" {
		return nil, fmt.Errorf("Mailgun API key is required for workspace %s", workspaceID)
	}
	
	if config.Domain == "" {
		return nil, fmt.Errorf("Mailgun domain is required for workspace %s", workspaceID)
	}
	
	// Set default base URL if not provided
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://api.mailgun.net/v3"
	}
	
	provider := &MailgunProvider{
		id:              fmt.Sprintf("mailgun-%s", workspaceID),
		workspaceID:     workspaceID,
		config:          config,
		domains:         []string{domain},
		displayName:     fmt.Sprintf("Mailgun Provider for %s", domain),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		healthy:         true, // Assume healthy until proven otherwise
		lastHealthCheck: time.Now(),
	}
	
	return provider, nil
}

// SendMessage implements Provider.SendMessage
func (m *MailgunProvider) SendMessage(ctx context.Context, msg *models.Message) error {
	if msg == nil {
		return fmt.Errorf("message cannot be nil")
	}
	
	if msg.From == "" {
		return fmt.Errorf("sender email is required")
	}
	
	if len(msg.To) == 0 {
		return fmt.Errorf("at least one recipient is required")
	}
	
	// Prepare form data for Mailgun API
	form := make(url.Values)
	
	// Required fields
	form.Set("from", m.formatFromAddress(msg))
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
	
	// Ensure at least one body format is provided
	if msg.HTML == "" && msg.Text == "" {
		return fmt.Errorf("message must contain either HTML or text content")
	}
	
	// Add tracking settings
	m.addTrackingSettings(&form)
	
	// Add tags
	tags := m.buildTags(msg)
	for _, tag := range tags {
		form.Add("o:tag", tag)
	}
	
	// Add custom variables for tracking
	if msg.CampaignID != "" {
		form.Set("v:campaign_id", msg.CampaignID)
	}
	if msg.UserID != "" {
		form.Set("v:user_id", msg.UserID)
	}
	if msg.ID != "" {
		form.Set("v:message_id", msg.ID)
	}
	if msg.WorkspaceID != "" {
		form.Set("v:workspace_id", msg.WorkspaceID)
	}
	
	// Add metadata as custom variables
	if msg.Metadata != nil {
		for key, value := range msg.Metadata {
			if strValue, ok := value.(string); ok {
				form.Set(fmt.Sprintf("v:%s", key), strValue)
			}
		}
	}
	
	// Send the request
	startTime := time.Now()
	err := m.sendRequest(ctx, &form)
	sendDuration := time.Since(startTime)
	
	if err != nil {
		m.setUnhealthy(err)
		log.Printf("Mailgun send failed for %s (took %v): %v", msg.From, sendDuration, err)
		return fmt.Errorf("failed to send email via Mailgun: %w", err)
	}
	
	// Mark as healthy on successful send
	m.setHealthy()
	
	log.Printf("Mailgun send successful for %s to %v (took %v)", 
		msg.From, msg.To, sendDuration)
	
	return nil
}

// sendRequest sends the actual HTTP request to Mailgun
func (m *MailgunProvider) sendRequest(ctx context.Context, form *url.Values) error {
	// Create the request
	apiURL := fmt.Sprintf("%s/%s/messages", m.config.BaseURL, m.config.Domain)
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	
	// Set headers
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth("api", m.config.APIKey)
	
	// Send the request
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.Printf("Warning: failed to close response body: %v", closeErr)
		}
	}()
	
	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}
	
	// Handle success response
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var mailgunResp MailgunResponse
		if err := json.Unmarshal(body, &mailgunResp); err != nil {
			// If we can't parse the response but the status is success, consider it sent
			log.Printf("Warning: Could not parse Mailgun success response: %v", err)
		}
		return nil
	}
	
	// Handle error response
	var errorResp MailgunErrorResponse
	if err := json.Unmarshal(body, &errorResp); err != nil {
		return fmt.Errorf("mailgun API error (status: %d): %s", resp.StatusCode, string(body))
	}
	
	return fmt.Errorf("mailgun API error (status: %d): %s", resp.StatusCode, errorResp.Message)
}

// formatFromAddress formats the from address for Mailgun
func (m *MailgunProvider) formatFromAddress(msg *models.Message) string {
	// Extract local part from original sender
	fromParts := strings.Split(msg.From, "@")
	if len(fromParts) != 2 {
		log.Printf("Warning: Invalid from email format: %s", msg.From)
		return fmt.Sprintf("noreply@%s", m.config.Domain)
	}
	
	localPart := fromParts[0]
	
	// Use the configured Mailgun domain for sending
	rewrittenFrom := fmt.Sprintf("%s@%s", localPart, m.config.Domain)
	
	// Check for display name in headers
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

// addTrackingSettings adds tracking configuration to the form
func (m *MailgunProvider) addTrackingSettings(form *url.Values) {
	if m.config.Tracking.Opens {
		form.Set("o:tracking-opens", "true")
	}
	
	if m.config.Tracking.Clicks {
		form.Set("o:tracking-clicks", "htmlonly")
	}
	
	if m.config.Tracking.Unsubscribe {
		form.Set("o:tracking-unsubscribe", "true")
		log.Printf("Added unsubscribe tracking for Mailgun domain %s", m.config.Domain)
	}
}

// buildTags creates tags for the message
func (m *MailgunProvider) buildTags(msg *models.Message) []string {
	tags := make([]string, 0)
	
	// Add default tags
	if m.config.Tags != nil {
		tags = append(tags, m.config.Tags...)
	}
	
	// Add campaign tag if available
	if msg.CampaignID != "" {
		tags = append(tags, fmt.Sprintf("campaign:%s", msg.CampaignID))
	}
	
	// Add user tag if available
	if msg.UserID != "" {
		tags = append(tags, fmt.Sprintf("user:%s", msg.UserID))
	}
	
	// Add workspace tag
	if msg.WorkspaceID != "" {
		tags = append(tags, fmt.Sprintf("workspace:%s", msg.WorkspaceID))
	}
	
	// Add provider tag
	tags = append(tags, "provider:mailgun")
	
	return tags
}

// GetType implements Provider.GetType
func (m *MailgunProvider) GetType() ProviderType {
	return ProviderTypeMailgun
}

// GetID implements Provider.GetID
func (m *MailgunProvider) GetID() string {
	return m.id
}

// HealthCheck implements Provider.HealthCheck
func (m *MailgunProvider) HealthCheck(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.lastHealthCheck = time.Now()
	
	// Perform a simple API call to check if Mailgun is accessible
	apiURL := fmt.Sprintf("%s/%s", m.config.BaseURL, m.config.Domain)
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		m.lastError = err
		m.healthy = false
		return fmt.Errorf("failed to create health check request: %w", err)
	}
	
	req.SetBasicAuth("api", m.config.APIKey)
	
	resp, err := m.httpClient.Do(req)
	if err != nil {
		m.lastError = err
		m.healthy = false
		return fmt.Errorf("health check failed: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.Printf("Warning: failed to close health check response body: %v", closeErr)
		}
	}()
	
	// Check response status
	if resp.StatusCode == 401 {
		err := fmt.Errorf("authentication failed")
		m.lastError = err
		m.healthy = false
		return fmt.Errorf("mailgun authentication failed")
	}
	
	if resp.StatusCode >= 500 {
		err := fmt.Errorf("server error: %d", resp.StatusCode)
		m.lastError = err
		m.healthy = false
		return fmt.Errorf("mailgun server error: %d", resp.StatusCode)
	}
	
	// Mark as healthy
	m.lastError = nil
	m.healthy = true
	
	return nil
}

// IsHealthy implements Provider.IsHealthy
func (m *MailgunProvider) IsHealthy() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	return m.healthy
}

// GetLastError implements Provider.GetLastError
func (m *MailgunProvider) GetLastError() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	return m.lastError
}

// CanSendFromDomain implements Provider.CanSendFromDomain
func (m *MailgunProvider) CanSendFromDomain(domain string) bool {
	// Mailgun can send from any domain by rewriting the From address
	// to use the configured Mailgun domain
	return true
}

// GetSupportedDomains implements Provider.GetSupportedDomains
func (m *MailgunProvider) GetSupportedDomains() []string {
	return m.domains
}

// GetProviderInfo implements Provider.GetProviderInfo
func (m *MailgunProvider) GetProviderInfo() ProviderInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	var lastError *string
	if m.lastError != nil {
		errorMsg := m.lastError.Error()
		lastError = &errorMsg
	}
	
	var lastHealthy *time.Time
	if m.healthy && !m.lastHealthCheck.IsZero() {
		lastHealthy = &m.lastHealthCheck
	}
	
	return ProviderInfo{
		ID:          m.id,
		Type:        ProviderTypeMailgun,
		DisplayName: m.displayName,
		Domains:     m.domains,
		Enabled:     m.config.Enabled,
		LastHealthy: lastHealthy,
		LastError:   lastError,
		Capabilities: []string{
			"send_email",
			"domain_rewriting",
			"html_content",
			"tracking",
			"tags",
			"webhooks",
			"analytics",
		},
		Metadata: map[string]string{
			"workspace_id":   m.workspaceID,
			"api_domain":     m.config.Domain,
			"base_url":       m.config.BaseURL,
			"region":         m.config.Region,
			"tracking_opens": fmt.Sprintf("%t", m.config.Tracking.Opens),
			"tracking_clicks": fmt.Sprintf("%t", m.config.Tracking.Clicks),
			"tracking_unsubscribe": fmt.Sprintf("%t", m.config.Tracking.Unsubscribe),
		},
	}
}

// setHealthy marks the provider as healthy
func (m *MailgunProvider) setHealthy() {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.healthy = true
	m.lastError = nil
	m.lastHealthCheck = time.Now()
}

// setUnhealthy marks the provider as unhealthy with an error
func (m *MailgunProvider) setUnhealthy(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.healthy = false
	m.lastError = err
	m.lastHealthCheck = time.Now()
}