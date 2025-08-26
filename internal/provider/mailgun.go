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
	domains         []string  // Domains this workspace handles (also used for Mailgun API)
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
// The domains parameter allows specifying multiple domains this provider serves
func NewMailgunProvider(workspaceID string, domains []string, config *config.WorkspaceMailgunConfig) (*MailgunProvider, error) {
	if config == nil {
		return nil, fmt.Errorf("Mailgun config cannot be nil")
	}
	
	if !config.Enabled {
		return nil, fmt.Errorf("Mailgun is disabled for workspace %s", workspaceID)
	}
	
	if config.APIKey == "" {
		return nil, fmt.Errorf("Mailgun API key is required for workspace %s", workspaceID)
	}
	
	// Domains are now provided by the router and must be configured in Mailgun
	if len(domains) == 0 {
		return nil, fmt.Errorf("No domains configured for Mailgun workspace %s", workspaceID)
	}
	
	// Set default base URL if not provided
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://api.mailgun.net/v3"
	}
	
	// Create display name based on domains
	displayName := "Mailgun Provider"
	if len(domains) > 0 {
		displayName = fmt.Sprintf("Mailgun Provider for %v", domains)
	}
	
	provider := &MailgunProvider{
		id:              fmt.Sprintf("mailgun-%s", workspaceID),
		workspaceID:     workspaceID,
		config:          config,
		domains:         domains,
		displayName:     displayName,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		healthy:         true, // Assume healthy until proven otherwise
		lastHealthCheck: time.Now(),
	}
	
	return provider, nil
}

// extractDomain extracts the domain from an email address
func extractDomain(email string) (string, error) {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid email format: %s", email)
	}
	return parts[1], nil
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
	
	// Extract domain from sender's email
	senderDomain, err := extractDomain(msg.From)
	if err != nil {
		return fmt.Errorf("failed to extract domain from sender email: %w", err)
	}
	
	// Verify this domain is in our configured domains
	domainFound := false
	for _, d := range m.domains {
		if d == senderDomain {
			domainFound = true
			break
		}
	}
	if !domainFound {
		return fmt.Errorf("sender domain %s is not configured for this Mailgun provider", senderDomain)
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
	
	// Skip adding tags - user preference to not include Mailgun tags
	
	// Add custom variables for tracking
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
	if msg.ProviderID != "" {
		form.Set("v:provider_id", msg.ProviderID)
	}
	
	// Add metadata as custom variables
	if msg.Metadata != nil {
		for key, value := range msg.Metadata {
			if strValue, ok := value.(string); ok {
				form.Set(fmt.Sprintf("v:%s", key), strValue)
			}
		}
	}
	
	// Add custom headers from message
	log.Printf("DEBUG: Mailgun provider processing headers for message %s", msg.ID)
	if len(msg.Headers) > 0 {
		log.Printf("DEBUG: Message has %d headers to process", len(msg.Headers))
		for headerName, headerValue := range msg.Headers {
			log.Printf("DEBUG: Processing header: %s = %s", headerName, headerValue)
			
			// Skip headers that Mailgun handles separately to avoid RFC 5322 violations
			headerLower := strings.ToLower(headerName)
			switch headerLower {
			case "content-type", "to", "from", "subject", "cc", "bcc", "date", "message-id":
				log.Printf("DEBUG: Skipping standard email header: %s", headerName)
				continue
			}
			
			// Add as custom header with h: prefix for Mailgun
			form.Set(fmt.Sprintf("h:%s", headerName), headerValue)
			log.Printf("Added custom header %s: %s for Mailgun domain %s", headerName, headerValue, senderDomain)
		}
	} else {
		log.Printf("DEBUG: Message has no headers to process (msg.Headers is nil or empty)")
	}
	
	// Send the request
	startTime := time.Now()
	err = m.sendRequest(ctx, &form, senderDomain)
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
func (m *MailgunProvider) sendRequest(ctx context.Context, form *url.Values, domain string) error {
	// Create the request
	apiURL := fmt.Sprintf("%s/%s/messages", m.config.BaseURL, domain)
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
	// Use the actual sender's email
	fromEmail := msg.From
	
	// Check for display name in headers
	if msg.Headers != nil {
		if senderName, exists := msg.Headers["X-Sender-Name"]; exists {
			return fmt.Sprintf("%s <%s>", senderName, fromEmail)
		}
		if fromHeader, exists := msg.Headers["From"]; exists && strings.Contains(fromHeader, "<") {
			// Extract display name from existing From header
			if idx := strings.Index(fromHeader, "<"); idx > 0 {
				displayName := strings.TrimSpace(fromHeader[:idx])
				return fmt.Sprintf("%s <%s>", displayName, fromEmail)
			}
		}
	}
	
	return fromEmail
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
		log.Printf("Added unsubscribe tracking for Mailgun")
	}
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
	// Use the first configured domain for health checks
	if len(m.domains) == 0 {
		m.lastError = fmt.Errorf("no domains configured")
		m.healthy = false
		return m.lastError
	}
	apiURL := fmt.Sprintf("%s/%s", m.config.BaseURL, m.domains[0])
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
	// Check if this domain is in our configured domains list
	for _, d := range m.domains {
		if d == domain {
			return true
		}
	}
	return false
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
			"provider_id":   m.workspaceID,
			"domains":        strings.Join(m.domains, ","),
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