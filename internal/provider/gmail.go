package provider

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net/mail"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"relay/internal/config"
	"relay/pkg/models"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

// GmailAuthError represents authentication-specific errors
type GmailAuthError struct {
	SenderEmail string
	ErrorType   string // "invalid_grant", "domain_wide_delegation", "user_not_found", etc.
	Message     string
	Cause       error
}

func (e *GmailAuthError) Error() string {
	return fmt.Sprintf("Gmail auth error for %s (%s): %s", e.SenderEmail, e.ErrorType, e.Message)
}

func (e *GmailAuthError) Unwrap() error {
	return e.Cause
}

// GmailProvider implements the Provider interface for Gmail/Google Workspace
type GmailProvider struct {
	id              string
	workspaceID     string
	config          *config.WorkspaceGmailConfig
	domains         []string
	displayName     string
	
	// Service cache for different sender emails within this workspace
	serviceCacheMu  sync.RWMutex
	serviceCache    map[string]*gmail.Service
	
	// Validation cache to avoid repeated validation attempts
	validationCacheMu sync.RWMutex
	validationCache   map[string]validationResult
	
	// Health monitoring
	mu              sync.RWMutex
	healthy         bool
	lastHealthCheck time.Time
	lastError       error
}

// validationResult caches sender validation results
type validationResult struct {
	valid     bool
	error     error
	timestamp time.Time
}

// NewGmailProvider creates a new Gmail provider instance
// NewGmailProvider creates a new Gmail provider instance
// The domains parameter allows specifying multiple domains this provider serves
func NewGmailProvider(workspaceID string, domains []string, config *config.WorkspaceGmailConfig) (*GmailProvider, error) {
	if config == nil {
		return nil, fmt.Errorf("Gmail config cannot be nil")
	}
	
	if !config.Enabled {
		return nil, fmt.Errorf("Gmail is disabled for workspace %s", workspaceID)
	}
	
	// Check if we have either file or env variable configured (or credentials in DB)
	// We'll allow creation even without file/env if we have a credentials loader
	if config.ServiceAccountFile == "" && config.ServiceAccountEnv == "" && GetCredentialsLoader() == nil {
		return nil, fmt.Errorf("Gmail service account credentials required for workspace %s (provide either service_account_file, service_account_env, or upload via dashboard)", workspaceID)
	}
	
	// If using file, validate it exists
	if config.ServiceAccountFile != "" {
		if _, err := os.Stat(config.ServiceAccountFile); os.IsNotExist(err) {
			return nil, fmt.Errorf("Gmail service account file does not exist: %s", config.ServiceAccountFile)
		}
	}
	
	// If using env variable, validate it's set
	if config.ServiceAccountEnv != "" {
		if os.Getenv(config.ServiceAccountEnv) == "" {
			return nil, fmt.Errorf("Gmail service account environment variable %s is not set for workspace %s", config.ServiceAccountEnv, workspaceID)
		}
	}
	
	// Validate default sender if provided
	if config.DefaultSender != "" {
		if !isValidEmail(config.DefaultSender) {
			return nil, fmt.Errorf("default sender email is not valid: %s", config.DefaultSender)
		}
		log.Printf("Gmail provider for workspace %s configured with default sender: %s", workspaceID, config.DefaultSender)
	}
	
	// Create display name based on domains
	displayName := "Gmail Provider"
	if len(domains) > 0 {
		displayName = fmt.Sprintf("Gmail Provider for %v", domains)
	}
	
	provider := &GmailProvider{
		id:              fmt.Sprintf("gmail-%s", workspaceID),
		workspaceID:     workspaceID,
		config:          config,
		domains:         domains,
		displayName:     displayName,
		serviceCache:    make(map[string]*gmail.Service),
		validationCache: make(map[string]validationResult),
		healthy:         true, // Assume healthy until proven otherwise
		lastHealthCheck: time.Now(),
	}
	
	log.Printf("Created Gmail provider for workspace %s, domains %v (require_valid_sender: %v)", 
		workspaceID, domains, config.RequireValidSender)
	
	return provider, nil
}

// SendMessage implements Provider.SendMessage
func (g *GmailProvider) SendMessage(ctx context.Context, msg *models.Message) error {
	if msg == nil {
		return fmt.Errorf("message cannot be nil")
	}
	
	if msg.From == "" {
		return fmt.Errorf("sender email is required")
	}
	
	if len(msg.To) == 0 {
		return fmt.Errorf("at least one recipient is required")
	}
	
	// Validate sender email format
	if !isValidEmail(msg.From) {
		return fmt.Errorf("sender email format is invalid: %s", msg.From)
	}
	
	originalSender := msg.From
	var authAttempts []string
	
	// Try to get Gmail service for the original sender
	service, err := g.getServiceForSender(ctx, msg.From)
	if err != nil {
		authAttempts = append(authAttempts, fmt.Sprintf("%s: %v", msg.From, err))
		
		// If we have a default sender configured and the original sender failed, try it
		if g.config.DefaultSender != "" && g.config.DefaultSender != msg.From {
			log.Printf("Authentication failed for %s, attempting with default sender %s", msg.From, g.config.DefaultSender)
			
			service, err = g.getServiceForSender(ctx, g.config.DefaultSender)
			if err != nil {
				authAttempts = append(authAttempts, fmt.Sprintf("%s (default): %v", g.config.DefaultSender, err))
				g.setUnhealthy(err)
				return &GmailAuthError{
					SenderEmail: originalSender,
					ErrorType:   "authentication_failed",
					Message:     fmt.Sprintf("Authentication failed for both original sender and default sender. Attempts: %s", strings.Join(authAttempts, "; ")),
					Cause:       err,
				}
			} else {
				// Successfully authenticated with default sender
				msg.From = g.config.DefaultSender
				log.Printf("Successfully authenticated with default sender %s for original sender %s", g.config.DefaultSender, originalSender)
				
				// Add metadata to track the sender substitution
				msg.Metadata = initializeMetadata(msg.Metadata)
				msg.Metadata["original_sender"] = originalSender
				msg.Metadata["actual_sender"] = g.config.DefaultSender
				msg.Metadata["sender_substitution"] = true
			}
		} else {
			g.setUnhealthy(err)
			return &GmailAuthError{
				SenderEmail: originalSender,
				ErrorType:   "authentication_failed",
				Message:     fmt.Sprintf("Failed to authenticate sender %s and no default sender configured. %s", originalSender, g.formatAuthenticationGuidance(err)),
				Cause:       err,
			}
		}
	}
	
	// Create Gmail message
	gmailMessage, err := g.createGmailMessage(msg)
	if err != nil {
		return fmt.Errorf("failed to create Gmail message: %w", err)
	}
	
	// Send the message
	startTime := time.Now()
	result, err := service.Users.Messages.Send("me", gmailMessage).Do()
	sendDuration := time.Since(startTime)
	
	if err != nil {
		g.setUnhealthy(err)
		
		// Provide detailed error information for send failures
		if googleErr, ok := err.(*googleapi.Error); ok {
			log.Printf("Gmail API error for %s (took %v): Code=%d, Message=%s", msg.From, sendDuration, googleErr.Code, googleErr.Message)
			return fmt.Errorf("Gmail API error (code %d): %s", googleErr.Code, googleErr.Message)
		}
		
		log.Printf("Gmail send failed for %s (took %v): %v", msg.From, sendDuration, err)
		return fmt.Errorf("failed to send email via Gmail: %w", err)
	}
	
	// Update message ID with Gmail's message ID
	if result != nil && result.Id != "" {
		msg.Metadata = initializeMetadata(msg.Metadata)
		msg.Metadata["gmail_message_id"] = result.Id
	}
	
	// Mark as healthy on successful send
	g.setHealthy()
	
	if originalSender != msg.From {
		log.Printf("Gmail send successful using default sender %s for original %s to %v (took %v, Gmail ID: %s)", 
			msg.From, originalSender, msg.To, sendDuration, result.Id)
	} else {
		log.Printf("Gmail send successful for %s to %v (took %v, Gmail ID: %s)", 
			msg.From, msg.To, sendDuration, result.Id)
	}
	
	return nil
}

// getServiceForSender creates or retrieves a Gmail service for a specific sender email
// getServiceAccountData returns the service account JSON data from file or environment variable
func (g *GmailProvider) getServiceAccountData() ([]byte, error) {
	// Use credentials loader if available (loads from DB, env, or file)
	if loader := GetCredentialsLoader(); loader != nil {
		credentials, err := loader.LoadGmailCredentials(
			g.workspaceID,
			g.config.ServiceAccountEnv,
			g.config.ServiceAccountFile,
		)
		if err == nil {
			return credentials, nil
		}
		// Fall back to traditional methods if loader fails
		log.Printf("Credentials loader failed for workspace %s, falling back: %v", g.workspaceID, err)
	}

	// Try environment variable first if configured
	if g.config.ServiceAccountEnv != "" {
		envData := os.Getenv(g.config.ServiceAccountEnv)
		if envData != "" {
			return []byte(envData), nil
		}
		// Fall back to file if env is empty
		log.Printf("Warning: Environment variable %s is empty, trying file fallback", g.config.ServiceAccountEnv)
	}
	
	// Try file if configured
	if g.config.ServiceAccountFile != "" {
		return os.ReadFile(g.config.ServiceAccountFile)
	}
	
	return nil, fmt.Errorf("no service account credentials available")
}

func (g *GmailProvider) getServiceForSender(ctx context.Context, senderEmail string) (*gmail.Service, error) {
	// Check validation cache first if sender validation is required
	if g.config.RequireValidSender {
		if !g.isValidatedSender(senderEmail) {
			return nil, &GmailAuthError{
				SenderEmail: senderEmail,
				ErrorType:   "invalid_sender",
				Message:     "sender email has not been validated or validation failed",
			}
		}
	}
	
	// Check service cache
	g.serviceCacheMu.RLock()
	if service, exists := g.serviceCache[senderEmail]; exists {
		g.serviceCacheMu.RUnlock()
		return service, nil
	}
	g.serviceCacheMu.RUnlock()
	
	// Create new service for this sender
	g.serviceCacheMu.Lock()
	defer g.serviceCacheMu.Unlock()
	
	// Double-check after acquiring write lock
	if service, exists := g.serviceCache[senderEmail]; exists {
		return service, nil
	}
	
	// Get service account data from file or environment variable
	serviceAccountData, err := g.getServiceAccountData()
	if err != nil {
		return nil, &GmailAuthError{
			SenderEmail: senderEmail,
			ErrorType:   "service_account_credentials",
			Message:     fmt.Sprintf("unable to get service account credentials: %v", err),
			Cause:       err,
		}
	}
	
	// Create JWT config for domain-wide delegation
	jwtConfig, err := google.JWTConfigFromJSON(serviceAccountData, gmail.GmailSendScope)
	if err != nil {
		return nil, &GmailAuthError{
			SenderEmail: senderEmail,
			ErrorType:   "jwt_config",
			Message:     "unable to create JWT config from service account file",
			Cause:       err,
		}
	}
	
	// Validate sender email format and domain before impersonation
	if err := g.validateSenderBeforeImpersonation(senderEmail); err != nil {
		return nil, err
	}
	
	// Set the subject to impersonate the user
	jwtConfig.Subject = senderEmail
	
	// Create HTTP client with impersonated credentials
	client := jwtConfig.Client(ctx)
	
	// Test the token by attempting to get one
	token, err := jwtConfig.TokenSource(ctx).Token()
	if err != nil {
		return nil, g.parseOAuth2Error(senderEmail, err)
	}
	
	log.Printf("Successfully obtained OAuth2 token for %s (expires: %v)", senderEmail, token.Expiry)
	
	// Create Gmail service
	service, err := gmail.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, &GmailAuthError{
			SenderEmail: senderEmail,
			ErrorType:   "service_creation",
			Message:     "unable to create Gmail service",
			Cause:       err,
		}
	}
	
	// Skip service validation since OAuth2 token success and service creation 
	// are sufficient proof of working Gmail integration with gmail.send scope
	
	// Cache the service
	g.serviceCache[senderEmail] = service
	
	// Cache successful validation
	g.cacheValidationResult(senderEmail, true, nil)
	
	log.Printf("Created and validated Gmail service for sender: %s (workspace: %s)", senderEmail, g.workspaceID)
	
	return service, nil
}

// createGmailMessage converts a models.Message to a Gmail message
func (g *GmailProvider) createGmailMessage(msg *models.Message) (*gmail.Message, error) {
	var messageBuilder strings.Builder
	
	// Format From header
	fromHeader := g.formatFromHeader(msg)
	messageBuilder.WriteString(fmt.Sprintf("From: %s\r\n", fromHeader))
	
	// Format To header
	messageBuilder.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(msg.To, ", ")))
	
	// Format CC header if present
	if len(msg.CC) > 0 {
		messageBuilder.WriteString(fmt.Sprintf("Cc: %s\r\n", strings.Join(msg.CC, ", ")))
	}
	
	// Format BCC header if present
	if len(msg.BCC) > 0 {
		messageBuilder.WriteString(fmt.Sprintf("Bcc: %s\r\n", strings.Join(msg.BCC, ", ")))
	}
	
	// Subject
	messageBuilder.WriteString(fmt.Sprintf("Subject: %s\r\n", msg.Subject))
	
	// Add custom headers (excluding reserved ones)
	if msg.Headers != nil {
		for key, value := range msg.Headers {
			if !g.isReservedHeader(key) {
				messageBuilder.WriteString(fmt.Sprintf("%s: %s\r\n", key, value))
			}
		}
	}
	
	// Content type and body
	if msg.HTML != "" {
		messageBuilder.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
		messageBuilder.WriteString("\r\n")
		messageBuilder.WriteString(msg.HTML)
	} else if msg.Text != "" {
		messageBuilder.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
		messageBuilder.WriteString("\r\n")
		messageBuilder.WriteString(msg.Text)
	} else {
		return nil, fmt.Errorf("message must contain either HTML or text content")
	}
	
	// Encode message
	rawMessage := base64.URLEncoding.EncodeToString([]byte(messageBuilder.String()))
	
	return &gmail.Message{
		Raw: rawMessage,
	}, nil
}

// formatFromHeader creates a properly formatted From header
func (g *GmailProvider) formatFromHeader(msg *models.Message) string {
	// Check if there's already a formatted From header
	if msg.Headers != nil {
		if fromHeader, exists := msg.Headers["From"]; exists {
			return fromHeader
		}
		
		// Check for sender name header
		if senderName, exists := msg.Headers["X-Sender-Name"]; exists {
			return fmt.Sprintf("%s <%s>", senderName, msg.From)
		}
	}
	
	// Create a display name from the email domain
	parts := strings.Split(msg.From, "@")
	if len(parts) == 2 {
		domain := parts[1]
		domainParts := strings.Split(domain, ".")
		if len(domainParts) > 0 {
			displayName := strings.Title(domainParts[0])
			return fmt.Sprintf("%s <%s>", displayName, msg.From)
		}
	}
	
	// Fallback to just the email address
	return msg.From
}

// isReservedHeader checks if a header is reserved and should not be added manually
func (g *GmailProvider) isReservedHeader(header string) bool {
	reserved := []string{"from", "to", "cc", "bcc", "subject", "content-type", "message-id", "date"}
	headerLower := strings.ToLower(header)
	
	for _, reservedHeader := range reserved {
		if headerLower == reservedHeader {
			return true
		}
	}
	
	return false
}

// GetType implements Provider.GetType
func (g *GmailProvider) GetType() ProviderType {
	return ProviderTypeGmail
}

// GetID implements Provider.GetID
func (g *GmailProvider) GetID() string {
	return g.id
}

// HealthCheck implements Provider.HealthCheck
func (g *GmailProvider) HealthCheck(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	
	g.lastHealthCheck = time.Now()
	
	// Test by checking service account credentials accessibility
	if _, err := g.getServiceAccountData(); err != nil {
		err := fmt.Errorf("service account credentials not accessible: %v", err)
		g.lastError = err
		g.healthy = false
		return err
	}
	
	// Test by creating a service for a test email in this domain
	if len(g.domains) == 0 {
		err := fmt.Errorf("no domains configured for Gmail provider")
		g.lastError = err
		g.healthy = false
		return err
	}
	
	// Use default sender if available for health check, otherwise use test email
	var testEmail string
	if g.config.DefaultSender != "" {
		testEmail = g.config.DefaultSender
		log.Printf("Health check using default sender: %s", testEmail)
	} else {
		testEmail = fmt.Sprintf("brian@%s", g.domains[0])
		log.Printf("Health check using test email: %s (no default sender configured)", testEmail)
	}
	
	// Try to create a Gmail service (this will validate credentials)
	_, err := g.getServiceForSender(ctx, testEmail)
	if err != nil {
		g.lastError = err
		g.healthy = false
		
		// Provide detailed health check failure information
		if authErr, ok := err.(*GmailAuthError); ok {
			log.Printf("Gmail health check failed with authentication error: %s", authErr.Error())
			return fmt.Errorf("Gmail health check failed: %s", authErr.Message)
		}
		
		log.Printf("Gmail health check failed: %v", err)
		return fmt.Errorf("Gmail health check failed: %w", err)
	}
	
	// Service creation with valid OAuth2 token is sufficient proof of working Gmail integration
	// We avoid calling GetProfile() as it requires additional scopes beyond gmail.send
	
	g.lastError = nil
	g.healthy = true
	
	log.Printf("Gmail health check passed for workspace %s", g.workspaceID)
	return nil
}

// IsHealthy implements Provider.IsHealthy
func (g *GmailProvider) IsHealthy() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	
	return g.healthy
}

// GetLastError implements Provider.GetLastError
func (g *GmailProvider) GetLastError() error {
	g.mu.RLock()
	defer g.mu.RUnlock()
	
	return g.lastError
}

// CanSendFromDomain implements Provider.CanSendFromDomain
func (g *GmailProvider) CanSendFromDomain(domain string) bool {
	for _, supportedDomain := range g.domains {
		if supportedDomain == domain {
			return true
		}
	}
	return false
}

// GetSupportedDomains implements Provider.GetSupportedDomains
func (g *GmailProvider) GetSupportedDomains() []string {
	return g.domains
}

// GetProviderInfo implements Provider.GetProviderInfo
func (g *GmailProvider) GetProviderInfo() ProviderInfo {
	g.mu.RLock()
	defer g.mu.RUnlock()
	
	var lastError *string
	if g.lastError != nil {
		errorMsg := g.lastError.Error()
		lastError = &errorMsg
	}
	
	var lastHealthy *time.Time
	if g.healthy && !g.lastHealthCheck.IsZero() {
		lastHealthy = &g.lastHealthCheck
	}
	
	// Get cache statistics
	cacheStats := g.GetCacheStats()
	validationStats := g.getValidationCacheStats()
	
	capabilities := []string{
		"send_email",
		"domain_impersonation",
		"html_content",
		"attachments",
		"custom_headers",
		"authentication_validation",
	}
	
	if g.config.DefaultSender != "" {
		capabilities = append(capabilities, "default_sender_fallback")
	}
	
	// Determine credential source for metadata
	credSource := "not_configured"
	if g.config.ServiceAccountEnv != "" {
		credSource = "environment_variable"
	} else if g.config.ServiceAccountFile != "" {
		credSource = "file"
	}
	
	metadata := map[string]string{
		"workspace_id":           g.workspaceID,
		"credential_source":      credSource,
		"require_valid_sender":   fmt.Sprintf("%v", g.config.RequireValidSender),
		"cached_services":        fmt.Sprintf("%d", cacheStats["cached_services"]),
		"validated_senders":      fmt.Sprintf("%d", validationStats["valid_count"]),
		"failed_validations":     fmt.Sprintf("%d", validationStats["invalid_count"]),
	}
	
	if g.config.DefaultSender != "" {
		metadata["default_sender"] = g.config.DefaultSender
	}
	
	return ProviderInfo{
		ID:          g.id,
		Type:        ProviderTypeGmail,
		DisplayName: g.displayName,
		Domains:     g.domains,
		Enabled:     g.config.Enabled,
		LastHealthy: lastHealthy,
		LastError:   lastError,
		Capabilities: capabilities,
		Metadata:     metadata,
	}
}

// setHealthy marks the provider as healthy
func (g *GmailProvider) setHealthy() {
	g.mu.Lock()
	defer g.mu.Unlock()
	
	g.healthy = true
	g.lastError = nil
	g.lastHealthCheck = time.Now()
}

// setUnhealthy marks the provider as unhealthy with an error
func (g *GmailProvider) setUnhealthy(err error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	
	g.healthy = false
	g.lastError = err
	g.lastHealthCheck = time.Now()
}

// ClearServiceCache clears the service cache for a specific sender or all senders
func (g *GmailProvider) ClearServiceCache(senderEmail string) {
	g.serviceCacheMu.Lock()
	defer g.serviceCacheMu.Unlock()
	
	if senderEmail == "" {
		// Clear all cached services
		g.serviceCache = make(map[string]*gmail.Service)
		log.Printf("Cleared all Gmail service cache for workspace %s", g.workspaceID)
		
		// Also clear validation cache when clearing all services
		g.ClearValidationCache("")
	} else {
		// Clear specific sender
		delete(g.serviceCache, senderEmail)
		log.Printf("Cleared Gmail service cache for sender %s", senderEmail)
		
		// Also clear validation cache for this sender
		g.ClearValidationCache(senderEmail)
	}
}

// GetCacheStats returns statistics about the service cache
func (g *GmailProvider) GetCacheStats() map[string]interface{} {
	g.serviceCacheMu.RLock()
	defer g.serviceCacheMu.RUnlock()
	
	cachedSenders := make([]string, 0, len(g.serviceCache))
	for sender := range g.serviceCache {
		cachedSenders = append(cachedSenders, sender)
	}
	
	return map[string]interface{}{
		"cached_services": len(g.serviceCache),
		"cached_senders":  cachedSenders,
	}
}

// getValidationCacheStats returns statistics about sender validation cache
func (g *GmailProvider) getValidationCacheStats() map[string]interface{} {
	g.validationCacheMu.RLock()
	defer g.validationCacheMu.RUnlock()
	
	validCount := 0
	invalidCount := 0
	expiredCount := 0
	
	for _, result := range g.validationCache {
		if time.Since(result.timestamp) > 5*time.Minute {
			expiredCount++
		} else if result.valid {
			validCount++
		} else {
			invalidCount++
		}
	}
	
	return map[string]interface{}{
		"total_cached":    len(g.validationCache),
		"valid_count":     validCount,
		"invalid_count":   invalidCount,
		"expired_count":   expiredCount,
	}
}

// ClearValidationCache clears the validation cache for a specific sender or all senders
func (g *GmailProvider) ClearValidationCache(senderEmail string) {
	g.validationCacheMu.Lock()
	defer g.validationCacheMu.Unlock()
	
	if senderEmail == "" {
		// Clear all cached validations
		g.validationCache = make(map[string]validationResult)
		log.Printf("Cleared all Gmail validation cache for workspace %s", g.workspaceID)
	} else {
		// Clear specific sender
		delete(g.validationCache, senderEmail)
		log.Printf("Cleared Gmail validation cache for sender %s", senderEmail)
	}
}

// ValidateSender explicitly validates a sender email without caching a service
func (g *GmailProvider) ValidateSender(ctx context.Context, senderEmail string) error {
	// Check cache first
	if result := g.getCachedValidationResult(senderEmail); result != nil {
		if result.valid {
			return nil
		}
		return result.error
	}
	
	// Perform validation
	if err := g.validateSenderBeforeImpersonation(senderEmail); err != nil {
		g.cacheValidationResult(senderEmail, false, err)
		return err
	}
	
	// Test authentication without caching the service
	serviceAccountData, err := g.getServiceAccountData()
	if err != nil {
		err = &GmailAuthError{
			SenderEmail: senderEmail,
			ErrorType:   "service_account_credentials",
			Message:     fmt.Sprintf("unable to get service account credentials: %v", err),
			Cause:       err,
		}
		g.cacheValidationResult(senderEmail, false, err)
		return err
	}
	
	jwtConfig, err := google.JWTConfigFromJSON(serviceAccountData, gmail.GmailSendScope)
	if err != nil {
		err = &GmailAuthError{
			SenderEmail: senderEmail,
			ErrorType:   "jwt_config",
			Message:     "unable to create JWT config from service account file",
			Cause:       err,
		}
		g.cacheValidationResult(senderEmail, false, err)
		return err
	}
	
	jwtConfig.Subject = senderEmail
	
	// Test token generation
	_, err = jwtConfig.TokenSource(ctx).Token()
	if err != nil {
		err = g.parseOAuth2Error(senderEmail, err)
		g.cacheValidationResult(senderEmail, false, err)
		return err
	}
	
	// Cache successful validation
	g.cacheValidationResult(senderEmail, true, nil)
	log.Printf("Successfully validated sender %s for workspace %s", senderEmail, g.workspaceID)
	
	return nil
}

// getCachedValidationResult gets a cached validation result if it's still fresh
func (g *GmailProvider) getCachedValidationResult(senderEmail string) *validationResult {
	g.validationCacheMu.RLock()
	defer g.validationCacheMu.RUnlock()
	
	result, exists := g.validationCache[senderEmail]
	if !exists {
		return nil
	}
	
	// Check if validation is still fresh (5 minutes)
	if time.Since(result.timestamp) > 5*time.Minute {
		return nil
	}
	
	return &result
}

// isValidEmail validates email format
func isValidEmail(email string) bool {
	if email == "" {
		return false
	}
	
	// Use Go's built-in email parsing
	_, err := mail.ParseAddress(email)
	if err != nil {
		return false
	}
	
	// Additional checks for Gmail compatibility
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	return emailRegex.MatchString(email)
}

// validateSenderBeforeImpersonation performs pre-impersonation validation
func (g *GmailProvider) validateSenderBeforeImpersonation(senderEmail string) error {
	if !isValidEmail(senderEmail) {
		return &GmailAuthError{
			SenderEmail: senderEmail,
			ErrorType:   "invalid_email_format",
			Message:     "sender email format is invalid",
		}
	}
	
	// Check if sender domain matches supported domains
	parts := strings.Split(senderEmail, "@")
	if len(parts) != 2 {
		return &GmailAuthError{
			SenderEmail: senderEmail,
			ErrorType:   "invalid_email_format",
			Message:     "sender email does not contain exactly one @ symbol",
		}
	}
	
	senderDomain := parts[1]
	if !g.CanSendFromDomain(senderDomain) {
		return &GmailAuthError{
			SenderEmail: senderEmail,
			ErrorType:   "unsupported_domain",
			Message:     fmt.Sprintf("sender domain %s is not supported by this provider (supported: %v)", senderDomain, g.domains),
		}
	}
	
	return nil
}

// parseOAuth2Error converts OAuth2 errors into more descriptive GmailAuthError
func (g *GmailProvider) parseOAuth2Error(senderEmail string, err error) error {
	if oauth2Err, ok := err.(*oauth2.RetrieveError); ok {
		errMsg := string(oauth2Err.Body)
		
		// Parse common OAuth2 error types
		if strings.Contains(errMsg, "invalid_grant") {
			if strings.Contains(errMsg, "Not a valid email or user ID") {
				return &GmailAuthError{
					SenderEmail: senderEmail,
					ErrorType:   "invalid_grant",
					Message:     fmt.Sprintf("sender email %s is not a valid Google Workspace user. %s", senderEmail, g.formatDomainDelegationGuidance()),
					Cause:       err,
				}
			}
			return &GmailAuthError{
				SenderEmail: senderEmail,
				ErrorType:   "invalid_grant",
				Message:     fmt.Sprintf("OAuth2 grant invalid for %s. This usually indicates domain-wide delegation is not properly configured. %s", senderEmail, g.formatDomainDelegationGuidance()),
				Cause:       err,
			}
		}
		
		if strings.Contains(errMsg, "unauthorized_client") {
			return &GmailAuthError{
				SenderEmail: senderEmail,
				ErrorType:   "unauthorized_client",
				Message:     fmt.Sprintf("service account is not authorized for domain-wide delegation. %s", g.formatDomainDelegationGuidance()),
				Cause:       err,
			}
		}
	}
	
	// Generic OAuth2 error
	return &GmailAuthError{
		SenderEmail: senderEmail,
		ErrorType:   "oauth2_error",
		Message:     fmt.Sprintf("OAuth2 authentication failed: %v. %s", err, g.formatAuthenticationGuidance(err)),
		Cause:       err,
	}
}

// validateGmailService tests the Gmail service by performing a simple operation
func (g *GmailProvider) validateGmailService(ctx context.Context, service *gmail.Service, senderEmail string) error {
	// Try to get the user's profile to validate the service works
	profile, err := service.Users.GetProfile("me").Context(ctx).Do()
	if err != nil {
		if googleErr, ok := err.(*googleapi.Error); ok {
			switch googleErr.Code {
			case 401:
				return &GmailAuthError{
					SenderEmail: senderEmail,
					ErrorType:   "unauthorized",
					Message:     fmt.Sprintf("unauthorized access to Gmail for %s. Check domain-wide delegation configuration. %s", senderEmail, g.formatDomainDelegationGuidance()),
					Cause:       err,
				}
			case 403:
				return &GmailAuthError{
					SenderEmail: senderEmail,
					ErrorType:   "forbidden",
					Message:     fmt.Sprintf("forbidden access to Gmail for %s. Check API scopes and permissions. %s", senderEmail, g.formatDomainDelegationGuidance()),
					Cause:       err,
				}
			case 404:
				return &GmailAuthError{
					SenderEmail: senderEmail,
					ErrorType:   "user_not_found",
					Message:     fmt.Sprintf("Gmail user %s not found. Ensure the user exists in Google Workspace.", senderEmail),
					Cause:       err,
				}
			}
		}
		
		return &GmailAuthError{
			SenderEmail: senderEmail,
			ErrorType:   "service_validation",
			Message:     fmt.Sprintf("failed to validate Gmail service for %s", senderEmail),
			Cause:       err,
		}
	}
	
	log.Printf("Gmail service validation successful for %s (email: %s, threads: %d, messages: %d)", 
		senderEmail, profile.EmailAddress, profile.ThreadsTotal, profile.MessagesTotal)
	
	return nil
}

// isValidatedSender checks if a sender has been validated recently
func (g *GmailProvider) isValidatedSender(senderEmail string) bool {
	g.validationCacheMu.RLock()
	defer g.validationCacheMu.RUnlock()
	
	result, exists := g.validationCache[senderEmail]
	if !exists {
		return false
	}
	
	// Check if validation is still fresh (5 minutes)
	if time.Since(result.timestamp) > 5*time.Minute {
		return false
	}
	
	return result.valid
}

// cacheValidationResult caches the validation result for a sender
func (g *GmailProvider) cacheValidationResult(senderEmail string, valid bool, err error) {
	g.validationCacheMu.Lock()
	defer g.validationCacheMu.Unlock()
	
	g.validationCache[senderEmail] = validationResult{
		valid:     valid,
		error:     err,
		timestamp: time.Now(),
	}
}

// formatDomainDelegationGuidance provides helpful guidance for domain-wide delegation setup
func (g *GmailProvider) formatDomainDelegationGuidance() string {
	return "To fix this: 1) Ensure the service account has domain-wide delegation enabled, 2) Add the service account client ID to Google Admin Console > Security > API Controls > Domain-wide delegation, 3) Grant the scope 'https://www.googleapis.com/auth/gmail.send', 4) Ensure the sender email is a valid Google Workspace user"
}

// formatAuthenticationGuidance provides context-specific guidance based on the error
func (g *GmailProvider) formatAuthenticationGuidance(err error) string {
	if err == nil {
		return g.formatDomainDelegationGuidance()
	}
	
	errStr := err.Error()
	if strings.Contains(errStr, "invalid_grant") {
		return "This error typically occurs when: 1) The sender email is not a valid Google Workspace user, 2) Domain-wide delegation is not properly configured, or 3) The service account doesn't have the required permissions. " + g.formatDomainDelegationGuidance()
	}
	
	if strings.Contains(errStr, "unauthorized_client") {
		return "The service account is not authorized for domain-wide delegation. " + g.formatDomainDelegationGuidance()
	}
	
	return g.formatDomainDelegationGuidance()
}

// initializeMetadata ensures metadata map is initialized
func initializeMetadata(metadata map[string]interface{}) map[string]interface{} {
	if metadata == nil {
		return make(map[string]interface{})
	}
	return metadata
}