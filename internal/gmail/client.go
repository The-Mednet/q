package gmail

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"sync"

	"smtp_relay/internal/config"
	"smtp_relay/pkg/models"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

type Client struct {
	config         *config.GmailConfig
	router         *WorkspaceRouter
	serviceCacheMu sync.RWMutex
	serviceCache   map[string]*gmail.Service // keyed by workspace ID + sender email
}

func NewClient(cfg *config.GmailConfig) (*Client, error) {
	if len(cfg.Workspaces) == 0 {
		return nil, fmt.Errorf("no workspaces configured")
	}

	router := NewWorkspaceRouter(cfg)

	return &Client{
		config:       cfg,
		router:       router,
		serviceCache: make(map[string]*gmail.Service),
	}, nil
}

// GetRouter returns the workspace router for external use
func (c *Client) GetRouter() *WorkspaceRouter {
	return c.router
}

func (c *Client) SendMessage(ctx context.Context, msg *models.Message) error {
	// Determine which workspace to use
	workspace, err := c.router.RouteMessage(msg.From)
	if err != nil {
		return fmt.Errorf("failed to route message: %v", err)
	}

	// Set workspace ID on message for tracking
	msg.WorkspaceID = workspace.ID

	// Extract campaign and user IDs from headers
	if msg.Headers != nil {
		if campaignID, exists := msg.Headers["X-Campaign-ID"]; exists {
			msg.CampaignID = campaignID
		}
		if userID, exists := msg.Headers["X-User-ID"]; exists {
			msg.UserID = userID
		}
	}

	service, err := c.getServiceForWorkspaceAndSender(ctx, workspace, msg.From)
	if err != nil {
		return fmt.Errorf("failed to get Gmail service for %s: %v", msg.From, err)
	}

	gmailMessage := c.createGmailMessage(msg)

	_, err = service.Users.Messages.Send("me", gmailMessage).Do()
	if err != nil {
		return fmt.Errorf("failed to send email as %s via workspace %s: %v", msg.From, workspace.ID, err)
	}

	return nil
}

func (c *Client) getServiceForWorkspaceAndSender(ctx context.Context, workspace *config.WorkspaceConfig, senderEmail string) (*gmail.Service, error) {
	cacheKey := fmt.Sprintf("%s:%s", workspace.ID, senderEmail)

	c.serviceCacheMu.RLock()
	if service, ok := c.serviceCache[cacheKey]; ok {
		c.serviceCacheMu.RUnlock()
		return service, nil
	}
	c.serviceCacheMu.RUnlock()

	// Read service account file for this workspace
	serviceAccountData, err := os.ReadFile(workspace.ServiceAccountFile)
	if err != nil {
		return nil, fmt.Errorf("unable to read service account file %s: %v", workspace.ServiceAccountFile, err)
	}

	// Create JWT config for domain-wide delegation
	jwtConfig, err := google.JWTConfigFromJSON(serviceAccountData, gmail.GmailSendScope)
	if err != nil {
		return nil, fmt.Errorf("unable to create JWT config: %v", err)
	}

	// Set the subject to impersonate the user
	jwtConfig.Subject = senderEmail

	// Create HTTP client with impersonated credentials
	client := jwtConfig.Client(ctx)

	// Create Gmail service
	service, err := gmail.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("unable to create Gmail service: %v", err)
	}

	// Cache the service
	c.serviceCacheMu.Lock()
	c.serviceCache[cacheKey] = service
	c.serviceCacheMu.Unlock()

	return service, nil
}

func (c *Client) createGmailMessage(msg *models.Message) *gmail.Message {
	var message strings.Builder

	// Format From header with display name if available
	fromHeader := c.formatFromHeader(msg)
	message.WriteString(fmt.Sprintf("From: %s\r\n", fromHeader))
	
	// Format To header with display names if available
	toHeader := c.formatRecipientHeader(msg.To, "To")
	message.WriteString(fmt.Sprintf("To: %s\r\n", toHeader))

	if len(msg.CC) > 0 {
		ccHeader := c.formatRecipientHeader(msg.CC, "Cc")
		message.WriteString(fmt.Sprintf("Cc: %s\r\n", ccHeader))
	}

	if len(msg.BCC) > 0 {
		bccHeader := c.formatRecipientHeader(msg.BCC, "Bcc")
		message.WriteString(fmt.Sprintf("Bcc: %s\r\n", bccHeader))
	}

	message.WriteString(fmt.Sprintf("Subject: %s\r\n", msg.Subject))

	// Add custom headers (excluding tracking headers that are now in dedicated fields)
	for key, value := range msg.Headers {
		if !isReservedHeader(key) && !isTrackingHeader(key) {
			message.WriteString(fmt.Sprintf("%s: %s\r\n", key, value))
		}
	}

	if msg.HTML != "" {
		message.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
		message.WriteString("\r\n")
		message.WriteString(msg.HTML)
	} else {
		message.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
		message.WriteString("\r\n")
		message.WriteString(msg.Text)
	}

	return &gmail.Message{
		Raw: base64.URLEncoding.EncodeToString([]byte(message.String())),
	}
}

// formatFromHeader creates a proper From header with display name if available
func (c *Client) formatFromHeader(msg *models.Message) string {
	// Check if there's a From header with display name in the headers
	if fromHeader, exists := msg.Headers["From"]; exists {
		return fromHeader
	}
	
	// Check for X-Sender-Name header (common in email services)
	if senderName, exists := msg.Headers["X-Sender-Name"]; exists {
		return fmt.Sprintf("%s <%s>", senderName, msg.From)
	}
	
	// Extract domain from email and create a display name
	parts := strings.Split(msg.From, "@")
	if len(parts) == 2 {
		domain := parts[1]
		// Remove common TLD extensions for cleaner display
		domainParts := strings.Split(domain, ".")
		if len(domainParts) > 0 {
			displayName := strings.ToUpper(string(domainParts[0][0])) + domainParts[0][1:]
			return fmt.Sprintf("%s <%s>", displayName, msg.From)
		}
	}
	
	// Fallback to just the email address
	return msg.From
}

// formatRecipientHeader creates properly formatted recipient headers with display names
func (c *Client) formatRecipientHeader(addresses []string, headerType string) string {
	if len(addresses) == 0 {
		return ""
	}
	
	formatted := make([]string, len(addresses))
	for i, address := range addresses {
		formatted[i] = c.formatSingleRecipient(address, headerType)
	}
	
	return strings.Join(formatted, ", ")
}

// formatSingleRecipient formats a single recipient address with display name if available
func (c *Client) formatSingleRecipient(address, headerType string) string {
	// If the address already contains a display name format, use it as-is
	if strings.Contains(address, "<") && strings.Contains(address, ">") {
		return address
	}
	
	// Extract local part and create a display name based on email
	parts := strings.Split(address, "@")
	if len(parts) == 2 {
		localPart := parts[0]
		// Capitalize first letter of local part for display name
		if len(localPart) > 0 {
			displayName := strings.ToUpper(string(localPart[0])) + localPart[1:]
			return fmt.Sprintf("%s <%s>", displayName, address)
		}
	}
	
	// Fallback to just the email address
	return address
}

func isReservedHeader(header string) bool {
	reserved := []string{"from", "to", "cc", "bcc", "subject", "content-type"}
	lower := strings.ToLower(header)
	for _, r := range reserved {
		if lower == r {
			return true
		}
	}
	return false
}

func isTrackingHeader(header string) bool {
	tracking := []string{"x-campaign-id", "x-user-id"}
	lower := strings.ToLower(header)
	for _, t := range tracking {
		if lower == t {
			return true
		}
	}
	return false
}

func (c *Client) clearServiceCache(workspaceID, senderEmail string) {
	cacheKey := fmt.Sprintf("%s:%s", workspaceID, senderEmail)
	c.serviceCacheMu.Lock()
	delete(c.serviceCache, cacheKey)
	c.serviceCacheMu.Unlock()
}

// ValidateServiceAccount checks if all workspaces are properly configured
func (c *Client) ValidateServiceAccount(ctx context.Context) error {
	workspaces := c.router.GetAllWorkspaces()
	if len(workspaces) == 0 {
		return fmt.Errorf("no workspaces configured")
	}

	for _, workspace := range workspaces {
		// Test with a dummy email for this workspace domain
		testEmail := fmt.Sprintf("test@%s", workspace.Domain)
		_, err := c.getServiceForWorkspaceAndSender(ctx, workspace, testEmail)
		if err != nil {
			return fmt.Errorf("workspace %s validation failed: %v", workspace.ID, err)
		}
	}

	return nil
}

// GetWorkspaceStats returns statistics about workspace usage
func (c *Client) GetWorkspaceStats() map[string]interface{} {
	workspaces := c.router.GetAllWorkspaces()
	legacyDomains := c.router.GetLegacyDomains()

	c.serviceCacheMu.RLock()
	activeConnections := len(c.serviceCache)
	c.serviceCacheMu.RUnlock()

	return map[string]interface{}{
		"total_workspaces":   len(workspaces),
		"legacy_domains":     legacyDomains,
		"active_connections": activeConnections,
		"workspaces":         workspaces,
	}
}
