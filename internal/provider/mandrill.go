package provider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"relay/internal/config"
	"relay/pkg/models"
)

// MandrillProvider implements the Provider interface for Mandrill
type MandrillProvider struct {
	config     *config.WorkspaceMandrillConfig
	httpClient *http.Client
	baseURL    string
}

// NewMandrillProvider creates a new Mandrill provider
func NewMandrillProvider(cfg *config.WorkspaceMandrillConfig) (*MandrillProvider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("mandrill config is nil")
	}

	if cfg.APIKey == "" {
		return nil, fmt.Errorf("mandrill API key is required")
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://mandrillapp.com/api/1.0"
	}

	return &MandrillProvider{
		config: cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL: baseURL,
	}, nil
}

// GetID returns the provider ID
func (m *MandrillProvider) GetID() string {
	return "mandrill"
}

// GetType returns the provider type
func (m *MandrillProvider) GetType() string {
	return "mandrill"
}

// IsEnabled returns whether the provider is enabled
func (m *MandrillProvider) IsEnabled() bool {
	return m.config != nil && m.config.Enabled
}

// GetRateLimits returns the provider's rate limits (Mandrill doesn't have strict daily limits)
func (m *MandrillProvider) GetRateLimits() (daily int, perUser int) {
	// Mandrill doesn't have strict daily limits, return high values
	return 100000, 10000
}

// HealthCheck verifies the Mandrill API is accessible
func (m *MandrillProvider) HealthCheck(ctx context.Context) error {
	// Use the ping endpoint to check API connectivity
	payload := map[string]string{
		"key": m.config.APIKey,
	}

	resp, err := m.apiCall(ctx, "/users/ping2.json", payload)
	if err != nil {
		return fmt.Errorf("mandrill health check failed: %w", err)
	}

	// Check if the response contains PING!
	if respMap, ok := resp.(map[string]interface{}); ok {
		if ping, ok := respMap["PING"].(string); !ok || ping != "PONG!" {
			return fmt.Errorf("mandrill health check failed: unexpected response")
		}
	} else {
		return fmt.Errorf("mandrill health check failed: invalid response format")
	}

	return nil
}

// SendEmail sends an email via Mandrill
func (m *MandrillProvider) SendEmail(ctx context.Context, msg *models.Message, options map[string]interface{}) error {
	if msg == nil {
		return fmt.Errorf("message is nil")
	}

	// Build Mandrill message structure
	mandrillMsg := m.buildMandrillMessage(msg)

	// Apply tracking settings
	if m.config.Tracking.Opens {
		mandrillMsg["track_opens"] = true
	}
	if m.config.Tracking.Clicks {
		mandrillMsg["track_clicks"] = "all"
	}
	if m.config.Tracking.AutoText {
		mandrillMsg["auto_text"] = true
	}
	if m.config.Tracking.AutoHtml {
		mandrillMsg["auto_html"] = true
	}
	if m.config.Tracking.InlineCss {
		mandrillMsg["inline_css"] = true
	}
	if m.config.Tracking.UrlStripQs {
		mandrillMsg["url_strip_qs"] = true
	}

	// Add default tags
	if len(m.config.Tags) > 0 {
		mandrillMsg["tags"] = m.config.Tags
	}

	// Set subaccount if configured
	if m.config.Subaccount != "" {
		mandrillMsg["subaccount"] = m.config.Subaccount
	}

	// Apply header rewriting if configured
	if m.config.HeaderRewrite.Enabled && len(m.config.HeaderRewrite.Rules) > 0 {
		headers := make(map[string]string)
		
		// Copy existing headers
		if msg.Headers != nil {
			for k, v := range msg.Headers {
				headers[k] = v
			}
		}

		// Apply rewrite rules
		for _, rule := range m.config.HeaderRewrite.Rules {
			if rule.NewValue != "" {
				headers[rule.HeaderName] = rule.NewValue
			} else {
				// Remove header if no new value is provided
				delete(headers, rule.HeaderName)
			}
		}

		mandrillMsg["headers"] = headers
	} else if msg.Headers != nil && len(msg.Headers) > 0 {
		mandrillMsg["headers"] = msg.Headers
	}

	// Prepare API request
	payload := map[string]interface{}{
		"key":     m.config.APIKey,
		"message": mandrillMsg,
		"async":   false, // Send synchronously for immediate feedback
	}

	// Send the email
	resp, err := m.apiCall(ctx, "/messages/send.json", payload)
	if err != nil {
		return fmt.Errorf("failed to send email via Mandrill: %w", err)
	}

	// Parse response to check for success
	if results, ok := resp.([]interface{}); ok && len(results) > 0 {
		if result, ok := results[0].(map[string]interface{}); ok {
			status, _ := result["status"].(string)
			if status == "sent" || status == "queued" || status == "scheduled" {
				// Success - store message ID if available
				if msgID, ok := result["_id"].(string); ok {
					msg.ID = msgID
				}
				return nil
			}
			
			// Handle rejection or other errors
			rejectReason, _ := result["reject_reason"].(string)
			return fmt.Errorf("mandrill rejected email: status=%s, reason=%s", status, rejectReason)
		}
	}

	return fmt.Errorf("unexpected response format from Mandrill")
}

// buildMandrillMessage converts our Message model to Mandrill's format
func (m *MandrillProvider) buildMandrillMessage(msg *models.Message) map[string]interface{} {
	mandrillMsg := map[string]interface{}{
		"from_email": msg.From,
		"subject":    msg.Subject,
	}

	// Set recipients
	to := make([]map[string]string, 0)
	for _, recipient := range msg.To {
		to = append(to, map[string]string{"email": recipient, "type": "to"})
	}
	
	// Add CC recipients
	for _, cc := range msg.CC {
		to = append(to, map[string]string{"email": cc, "type": "cc"})
	}
	
	// Add BCC recipients
	for _, bcc := range msg.BCC {
		to = append(to, map[string]string{"email": bcc, "type": "bcc"})
	}
	
	mandrillMsg["to"] = to

	// Set email content
	if msg.HTML != "" {
		mandrillMsg["html"] = msg.HTML
	}
	if msg.Text != "" {
		mandrillMsg["text"] = msg.Text
	}

	// Add attachments if present
	if len(msg.Attachments) > 0 {
		attachments := make([]map[string]string, 0, len(msg.Attachments))
		for _, att := range msg.Attachments {
			attachment := map[string]string{
				"type":    att.ContentType,
				"name":    att.Name,
				"content": base64.StdEncoding.EncodeToString(att.Content),
			}
			attachments = append(attachments, attachment)
		}
		mandrillMsg["attachments"] = attachments
	}

	// Important flag for transactional emails
	mandrillMsg["important"] = false

	return mandrillMsg
}

// apiCall makes a call to the Mandrill API
func (m *MandrillProvider) apiCall(ctx context.Context, endpoint string, payload interface{}) (interface{}, error) {
	url := m.baseURL + endpoint

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "SMTP-Relay/1.0")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse response
	var result interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Check for API errors
	if resp.StatusCode != http.StatusOK {
		if errMap, ok := result.(map[string]interface{}); ok {
			if status, ok := errMap["status"].(string); ok && status == "error" {
				message, _ := errMap["message"].(string)
				code, _ := errMap["code"].(float64)
				return nil, fmt.Errorf("mandrill API error: %s (code: %.0f)", message, code)
			}
		}
		return nil, fmt.Errorf("mandrill API returned status %d: %s", resp.StatusCode, string(body))
	}

	return result, nil
}

// ValidateSender checks if a sender email is valid for this provider
func (m *MandrillProvider) ValidateSender(ctx context.Context, senderEmail string) error {
	// Mandrill doesn't require pre-verified senders like some other providers
	// You can send from any email address
	if senderEmail == "" {
		return fmt.Errorf("sender email cannot be empty")
	}
	
	// Basic email validation
	if !strings.Contains(senderEmail, "@") {
		return fmt.Errorf("invalid sender email format")
	}
	
	return nil
}

// GetCapabilities returns the provider's capabilities
func (m *MandrillProvider) GetCapabilities() map[string]bool {
	return map[string]bool{
		"attachments":     true,
		"html":           true,
		"text":           true,
		"tracking":       true,
		"custom_headers": true,
		"tags":           true,
		"subaccounts":    true,
		"templates":      true,  // Mandrill supports templates
		"scheduling":     true,  // Mandrill supports scheduled sending
		"webhooks":       true,  // Mandrill has webhook support
	}
}

// GetStatus returns the current status of the provider
func (m *MandrillProvider) GetStatus(ctx context.Context) (map[string]interface{}, error) {
	// Get account info from Mandrill
	payload := map[string]string{
		"key": m.config.APIKey,
	}

	resp, err := m.apiCall(ctx, "/users/info.json", payload)
	if err != nil {
		return nil, fmt.Errorf("failed to get Mandrill account info: %w", err)
	}

	status := map[string]interface{}{
		"provider": "mandrill",
		"enabled":  m.IsEnabled(),
	}

	// Extract useful information from the response
	if info, ok := resp.(map[string]interface{}); ok {
		if username, ok := info["username"].(string); ok {
			status["username"] = username
		}
		if reputation, ok := info["reputation"].(float64); ok {
			status["reputation"] = reputation
		}
		if stats, ok := info["stats"].(map[string]interface{}); ok {
			if today, ok := stats["today"].(map[string]interface{}); ok {
				status["sent_today"] = today["sent"]
				status["hard_bounces_today"] = today["hard_bounces"]
				status["soft_bounces_today"] = today["soft_bounces"]
				status["rejects_today"] = today["rejects"]
			}
		}
	}

	return status, nil
}

// Shutdown performs any cleanup needed
func (m *MandrillProvider) Shutdown() error {
	// No specific cleanup needed for Mandrill
	log.Printf("Mandrill provider shutting down")
	return nil
}