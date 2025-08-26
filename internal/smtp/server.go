package smtp

import (
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"relay/internal/config"
	"relay/internal/queue"
	"relay/internal/validation"
	"relay/internal/workspace"
	"relay/pkg/models"

	"github.com/emersion/go-smtp"
	"github.com/google/uuid"
)

type Server struct {
	config           *config.SMTPConfig
	queue            queue.Queue
	workspaceManager *workspace.Manager
	server           *smtp.Server
}

func NewServer(cfg *config.SMTPConfig, q queue.Queue, workspaceManager *workspace.Manager) *Server {
	// Defensive programming: validate inputs
	if cfg == nil {
		log.Printf("Error: SMTP config is nil, using defaults")
		cfg = &config.SMTPConfig{
			Host: "localhost",
			Port: 2525,
			ReadTimeout: 30 * time.Second,
			WriteTimeout: 30 * time.Second,
			MaxSize: 25 * 1024 * 1024, // 25MB
		}
	}
	if q == nil {
		log.Fatal("Error: Queue cannot be nil - service cannot function without message queue")
	}
	if workspaceManager == nil {
		log.Printf("Warning: WorkspaceManager is nil - workspace routing will be disabled")
	}

	s := &Server{
		config:           cfg,
		queue:            q,
		workspaceManager: workspaceManager,
	}

	backend := &Backend{queue: q, workspaceManager: workspaceManager}

	server := smtp.NewServer(backend)
	server.Addr = fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	server.Domain = cfg.Host
	server.ReadTimeout = cfg.ReadTimeout
	server.WriteTimeout = cfg.WriteTimeout
	server.MaxMessageBytes = cfg.MaxSize
	server.MaxRecipients = 100
	server.AllowInsecureAuth = false // Require secure authentication

	s.server = server
	return s
}

func (s *Server) Start() error {
	// Defensive programming: check for nil server
	if s == nil {
		return fmt.Errorf("server instance is nil")
	}
	if s.server == nil {
		return fmt.Errorf("SMTP server instance is nil - initialization failed")
	}
	
	// Validate authentication is configured
	if os.Getenv("SMTP_AUTH_USERNAME") == "" || os.Getenv("SMTP_AUTH_PASSWORD") == "" {
		return fmt.Errorf("SMTP authentication not configured: SMTP_AUTH_USERNAME and SMTP_AUTH_PASSWORD must be set")
	}
	
	log.Printf("Starting SMTP server on %s", s.server.Addr)
	return s.server.ListenAndServe()
}

func (s *Server) Stop() error {
	return s.server.Close()
}

type Backend struct {
	queue            queue.Queue
	workspaceManager *workspace.Manager
}

func (b *Backend) NewSession(_ *smtp.Conn) (smtp.Session, error) {
	// Defensive programming: validate backend components
	if b == nil {
		return nil, fmt.Errorf("backend is nil")
	}
	if b.queue == nil {
		return nil, fmt.Errorf("queue is nil - cannot create session")
	}
	// workspaceManager can be nil - we'll handle it in session methods
	if b.workspaceManager == nil {
		log.Printf("Warning: WorkspaceManager is nil in session creation - workspace routing disabled")
	}
	
	return &Session{queue: b.queue, workspaceManager: b.workspaceManager}, nil
}

type Session struct {
	queue            queue.Queue
	workspaceManager *workspace.Manager
	from             string
	to               []string
	message          *models.Message
}

func (s *Session) AuthPlain(username, password string) error {
	// Require authentication credentials
	if username == "" || password == "" {
		return fmt.Errorf("authentication required: username and password cannot be empty")
	}
	
	// Get expected credentials from environment
	expectedUsername := os.Getenv("SMTP_AUTH_USERNAME")
	expectedPassword := os.Getenv("SMTP_AUTH_PASSWORD")
	
	// Ensure credentials are configured
	if expectedUsername == "" || expectedPassword == "" {
		log.Printf("Warning: SMTP authentication not properly configured")
		return fmt.Errorf("server configuration error: authentication not available")
	}
	
	// Use constant-time comparison to prevent timing attacks
	usernameMatch := subtle.ConstantTimeCompare([]byte(username), []byte(expectedUsername)) == 1
	passwordMatch := subtle.ConstantTimeCompare([]byte(password), []byte(expectedPassword)) == 1
	
	if !usernameMatch || !passwordMatch {
		log.Printf("Authentication failed for user: %s", username)
		return fmt.Errorf("authentication failed: invalid credentials")
	}
	
	log.Printf("Authentication successful for user: %s", username)
	return nil
}

func (s *Session) Mail(from string, opts *smtp.MailOptions) error {
	// Validate sender email address
	if err := validation.ValidateEmail(from); err != nil {
		log.Printf("Invalid sender email: %s - %v", from, err)
		return fmt.Errorf("invalid sender address: %w", err)
	}
	
	s.from = from

	// Determine provider for this sender and check if domain rewriting is needed
	var providerID string
	var actualFrom string = from
	
	if s.workspaceManager != nil {
		log.Printf("DEBUG: Getting workspace for sender: %s", from)
		result, err := s.workspaceManager.GetWorkspaceForSenderWithRewrite(from)
		if err == nil && result != nil {
			providerID = result.Workspace.ID
			log.Printf("DEBUG: Found workspace %s for sender %s (NeedsDomainRewrite=%v)", providerID, from, result.NeedsDomainRewrite)
			
			// If domain rewriting is needed, modify the sender address
			if result.NeedsDomainRewrite {
				// Get the primary domain from the selected workspace
				primaryDomain := result.Workspace.GetPrimaryDomain()
				if primaryDomain != "" {
					// Extract username from original sender
					atIndex := strings.LastIndex(from, "@")
					if atIndex > 0 {
						username := from[:atIndex]
						actualFrom = username + "@" + primaryDomain
						log.Printf("Rewriting sender domain: %s -> %s (using provider %s)", from, actualFrom, providerID)
					}
				}
			}
		} else {
			log.Printf("Warning: Could not route message from %s: %v", from, err)
		}
	}

	s.message = &models.Message{
		ID:         uuid.New().String(),
		From:       actualFrom, // Use the potentially rewritten sender address
		ProviderID: providerID,
		Status:     models.StatusQueued,
		QueuedAt:   time.Now(),
		Headers:    make(map[string]string),
		Metadata:   make(map[string]interface{}),
	}
	
	// Store original sender in metadata if it was rewritten
	if actualFrom != from {
		s.message.Metadata["original_sender"] = from
		s.message.Metadata["domain_rewritten"] = true
	}
	return nil
}

func (s *Session) Rcpt(to string, opts *smtp.RcptOptions) error {
	// Validate recipient email address
	if err := validation.ValidateEmail(to); err != nil {
		log.Printf("Invalid recipient email: %s - %v", to, err)
		return fmt.Errorf("invalid recipient address: %w", err)
	}
	
	// Check recipient limit
	if len(s.to) >= 100 {
		return fmt.Errorf("too many recipients (max 100)")
	}
	
	s.to = append(s.to, to)
	return nil
}

func (s *Session) Data(r io.Reader) error {
	// Defensive programming: validate session state
	if s == nil {
		return fmt.Errorf("session is nil")
	}
	if s.message == nil {
		return errors.New("no mail transaction in progress")
	}
	if s.queue == nil {
		return fmt.Errorf("queue is nil - cannot process message")
	}
	if r == nil {
		return fmt.Errorf("reader is nil - cannot read message data")
	}

	// Implement message size limit to prevent DoS attacks
	const maxMessageSize = 25 * 1024 * 1024 // 25MB limit
	limitedReader := io.LimitReader(r, maxMessageSize+1) // +1 to detect if limit exceeded
	
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return err
	}
	
	// Check if message exceeded size limit
	if len(data) > maxMessageSize {
		log.Printf("Message size exceeded limit: %d bytes (max: %d bytes)", len(data), maxMessageSize)
		return fmt.Errorf("message too large: exceeds %d MB limit", maxMessageSize/(1024*1024))
	}

	s.message.To = s.to

	if err := s.parseMessage(data); err != nil {
		return err
	}

	// Defensive check before enqueueing
	if s.queue == nil {
		return fmt.Errorf("queue is nil - cannot enqueue message")
	}
	if s.message == nil {
		return fmt.Errorf("message is nil - cannot enqueue")
	}
	
	if err := s.queue.Enqueue(s.message); err != nil {
		return fmt.Errorf("failed to queue message: %w", err)
	}

	log.Printf("Message %s queued successfully", s.message.ID)
	return nil
}

func (s *Session) Reset() {
	s.from = ""
	s.to = nil
	s.message = nil
}

func (s *Session) Logout() error {
	return nil
}

func (s *Session) parseMessage(data []byte) error {
	lines := strings.Split(string(data), "\n")
	headers := true
	var body strings.Builder
	
	// Variables to handle multi-line headers (RFC 5322 folding)
	var currentHeaderKey string
	var currentHeaderValue strings.Builder

	for _, line := range lines {
		if headers {
			if line == "" || line == "\r" {
				// Process any pending header before switching to body
				if currentHeaderKey != "" {
					s.processHeader(currentHeaderKey, strings.TrimSpace(currentHeaderValue.String()))
					currentHeaderKey = ""
					currentHeaderValue.Reset()
				}
				headers = false
				continue
			}

			// Check if this is a continuation of the previous header (starts with whitespace)
			if (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) && currentHeaderKey != "" {
				// This is a continuation line - append to current header value
				currentHeaderValue.WriteString(" ")
				currentHeaderValue.WriteString(strings.TrimSpace(line))
				continue
			}

			// Process any pending header
			if currentHeaderKey != "" {
				s.processHeader(currentHeaderKey, strings.TrimSpace(currentHeaderValue.String()))
				currentHeaderKey = ""
				currentHeaderValue.Reset()
			}

			// Parse new header
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				currentHeaderKey = strings.TrimSpace(parts[0])
				currentHeaderValue.WriteString(strings.TrimSpace(parts[1]))
			}
		} else {
			body.WriteString(line)
			body.WriteString("\n")
		}
	}
	
	// Process any remaining header
	if headers && currentHeaderKey != "" {
		s.processHeader(currentHeaderKey, strings.TrimSpace(currentHeaderValue.String()))
	}

	bodyContent := strings.TrimSpace(body.String())

	// Handle Content-Transfer-Encoding
	if encoding := s.message.Headers["Content-Transfer-Encoding"]; encoding != "" {
		switch strings.ToLower(encoding) {
		case "base64":
			if decoded, err := base64.StdEncoding.DecodeString(bodyContent); err == nil {
				bodyContent = string(decoded)
			} else {
				log.Printf("Warning: Failed to decode Base64 body: %v", err)
			}
		case "quoted-printable":
			// Could add quoted-printable decoding here if needed
			log.Printf("Warning: Quoted-printable encoding detected but not decoded")
		}
	}

	if isHTML(s.message.Headers["Content-Type"]) {
		s.message.HTML = bodyContent
	} else {
		s.message.Text = bodyContent
	}

	// Add any missing headers defined in workspace rewrite rules
	s.addMissingHeaders()

	return nil
}

// processHeader processes a single header key-value pair
func (s *Session) processHeader(key, value string) {
	switch strings.ToLower(key) {
	case "subject":
		// Validate subject
		if err := validation.ValidateSubject(value); err != nil {
			log.Printf("Invalid subject: %v", err)
			value = validation.SanitizeString(value) // Sanitize instead of rejecting
		}
		s.message.Subject = value
	case "content-type":
		s.message.Headers["Content-Type"] = value
	case "content-transfer-encoding":
		s.message.Headers["Content-Transfer-Encoding"] = value
	case "cc":
		s.message.CC = parseAddresses(value)
	case "bcc":
		s.message.BCC = parseAddresses(value)
	case "x-mc-tags":
		// Store the original header for visibility
		s.message.Headers["X-MC-Tags"] = value
		// Parse tags array - could be JSON array or comma-separated
		tags := parseTagsHeader(value)
		if len(tags) > 0 {
			if s.message.Metadata == nil {
				s.message.Metadata = make(map[string]interface{})
			}
			s.message.Metadata["tags"] = tags
		}
	case "x-mc-metadata":
		// Store the original header for visibility
		s.message.Headers["X-MC-Metadata"] = value
		// Parse JSON metadata
		if err := parseMCMetadata(value, s.message); err != nil {
			log.Printf("Warning: Failed to parse X-MC-Metadata: %v", err)
		}
	default:
		// Apply header modifications based on workspace provider
		modifiedValue := s.modifyHeaderForProvider(key, value)
		// Check if header should be removed
		if modifiedValue != "__REMOVE_HEADER__" {
			s.message.Headers[key] = modifiedValue
		} else {
			log.Printf("DEBUG: Removing header %s based on workspace rules", key)
		}

		// Extract recipient metadata from X-Recipient-* headers
		if strings.HasPrefix(strings.ToLower(key), "x-recipient-") {
			if s.message.Metadata["recipient"] == nil {
				s.message.Metadata["recipient"] = make(map[string]interface{})
			}
			recipientKey := strings.TrimPrefix(strings.ToLower(key), "x-recipient-")
			if recipientMap, ok := s.message.Metadata["recipient"].(map[string]interface{}); ok {
				recipientMap[recipientKey] = value
			}
		}
	}
}

// modifyHeaderForProvider modifies headers based on the workspace provider type
func (s *Session) modifyHeaderForProvider(headerName, headerValue string) string {
	log.Printf("DEBUG: modifyHeaderForProvider called: header='%s', value='%s'", headerName, headerValue)

	// Only modify if we have a workspace ID and workspace manager
	if s.message == nil {
		log.Printf("DEBUG: s.message is nil, returning original value")
		return headerValue
	}
	if s.message.ProviderID == "" {
		log.Printf("DEBUG: s.message.ProviderID is empty, returning original value")
		return headerValue
	}
	if s.workspaceManager == nil {
		log.Printf("DEBUG: s.workspaceManager is nil, returning original value")
		return headerValue
	}

	log.Printf("DEBUG: Getting workspace for sender: %s", s.from)

	// Defensive programming: check workspaceManager before use
	if s.workspaceManager == nil {
		log.Printf("Warning: WorkspaceManager is nil - cannot modify headers based on provider")
		return headerValue
	}
	
	// Get workspace configuration to determine provider type
	workspace, err := s.workspaceManager.GetWorkspaceForSender(s.from)
	if err != nil {
		// If we can't determine workspace, return original value
		log.Printf("DEBUG: Could not get workspace for sender %s: %v", s.from, err)
		return headerValue
	}
	
	// Defensive check for nil workspace
	if workspace == nil {
		log.Printf("Warning: GetWorkspaceForSender returned nil workspace for %s", s.from)
		return headerValue
	}

	log.Printf("DEBUG: Found workspace: ID='%s', Domain='%s', HasMailgun=%v",
		workspace.ID, workspace.Domain, workspace.Mailgun != nil)

	// Apply header rewriting rules for any header type
	return s.applyHeaderRewriteRules(headerName, headerValue, workspace)
}

// applyHeaderRewriteRules applies configured header rewrite rules for a workspace
func (s *Session) applyHeaderRewriteRules(headerName, headerValue string, workspace *config.WorkspaceConfig) string {
	log.Printf("DEBUG: applyHeaderRewriteRules called: header='%s', value='%s', workspace='%s'", headerName, headerValue, workspace.ID)

	// Check if this workspace uses Gmail with header rewriting enabled
	if workspace.Gmail != nil && workspace.Gmail.Enabled && workspace.Gmail.HeaderRewrite.Enabled {
		log.Printf("DEBUG: Workspace %s has Gmail with header rewrite enabled, checking %d rules", 
			workspace.ID, len(workspace.Gmail.HeaderRewrite.Rules))
		
		// Apply configured rewrite rules for Gmail
		for i, rule := range workspace.Gmail.HeaderRewrite.Rules {
			log.Printf("DEBUG: Checking Gmail rule %d: header_name='%s', new_value='%s'", i, rule.HeaderName, rule.NewValue)
			if strings.EqualFold(rule.HeaderName, headerName) {
				if rule.NewValue == "" {
					// Empty new_value means remove the header
					log.Printf("DEBUG: Gmail rule %d matched! Removing header %s from workspace %s",
						i, headerName, workspace.ID)
					return "__REMOVE_HEADER__" // Special marker for header removal
				} else {
					log.Printf("DEBUG: Gmail rule %d matched! Replacing header %s in workspace %s: %s -> %s",
						i, headerName, workspace.ID, headerValue, rule.NewValue)
					return rule.NewValue
				}
			}
		}
	}
	
	// Check if this workspace uses Mailgun with header rewriting enabled
	if workspace.Mailgun != nil && workspace.Mailgun.Enabled {
		log.Printf("DEBUG: Workspace %s has Mailgun enabled, checking header rewrite", workspace.ID)

		if !workspace.Mailgun.HeaderRewrite.Enabled {
			log.Printf("DEBUG: Header rewrite not enabled for workspace %s, applying default behavior", workspace.ID)
			// Apply default behavior for Mailgun workspaces without custom rules
			return s.applyDefaultMailgunHeaderRewrite(headerName, headerValue, workspace)
		}

		log.Printf("DEBUG: Header rewrite enabled for workspace %s, checking %d rules", workspace.ID, len(workspace.Mailgun.HeaderRewrite.Rules))

		// Apply configured rewrite rules for Mailgun
		for i, rule := range workspace.Mailgun.HeaderRewrite.Rules {
			log.Printf("DEBUG: Checking Mailgun rule %d: header_name='%s', new_value='%s'", i, rule.HeaderName, rule.NewValue)
			if strings.EqualFold(rule.HeaderName, headerName) {
				if rule.NewValue == "" {
					// Empty new_value means remove the header
					log.Printf("DEBUG: Mailgun rule %d matched! Removing header %s from workspace %s",
						i, headerName, workspace.ID)
					return "__REMOVE_HEADER__" // Special marker for header removal
				} else {
					log.Printf("DEBUG: Mailgun rule %d matched! Replacing header %s in workspace %s: %s -> %s",
						i, headerName, workspace.ID, headerValue, rule.NewValue)
					return rule.NewValue
				}
			}
		}
	}

	log.Printf("DEBUG: No matching rule found for header %s, returning original value", headerName)
	// No matching rule found, return original value
	return headerValue
}

// applyDefaultMailgunHeaderRewrite applies default header rewriting for Mailgun workspaces
func (s *Session) applyDefaultMailgunHeaderRewrite(headerName, headerValue string, workspace *config.WorkspaceConfig) string {
	// Defensive programming: validate inputs
	if workspace == nil {
		log.Printf("Warning: Workspace is nil in applyDefaultMailgunHeaderRewrite")
		return headerValue
	}
	
	if strings.ToLower(headerName) != "list-unsubscribe" {
		return headerValue
	}

	// Default behavior: Replace common Mandrill patterns with Mailgun equivalents
	modifiedValue := headerValue

	// Replace Mandrill domains with Mailgun equivalents
	if strings.Contains(headerValue, "mandrillapp.com") {
		// Extract the domain from workspace config
		domain := workspace.GetPrimaryDomain()
		if domain == "" {
			log.Printf("Warning: Workspace %s has no primary domain for header rewrite", workspace.ID)
			return headerValue
		}

		// Replace Mandrill URL with Mailgun-compatible URL
		modifiedValue = strings.ReplaceAll(headerValue, "mandrillapp.com", domain)
		log.Printf("Applied default Mailgun header rewrite for workspace %s: %s -> %s",
			workspace.ID, headerValue, modifiedValue)
	}

	return modifiedValue
}

// addMissingHeaders adds headers defined in rewrite rules that don't exist in the message
func (s *Session) addMissingHeaders() {
	log.Printf("DEBUG: addMissingHeaders called")

	if s.message == nil {
		log.Printf("DEBUG: s.message is nil, skipping addMissingHeaders")
		return
	}
	if s.message.ProviderID == "" {
		log.Printf("DEBUG: s.message.ProviderID is empty, skipping addMissingHeaders")
		return
	}
	if s.workspaceManager == nil {
		log.Printf("DEBUG: s.workspaceManager is nil, skipping addMissingHeaders")
		return
	}

	log.Printf("DEBUG: Getting workspace for sender: %s", s.from)

	// Defensive programming: check workspaceManager before use
	if s.workspaceManager == nil {
		log.Printf("Warning: WorkspaceManager is nil - cannot add missing headers")
		return
	}
	
	// Get workspace configuration
	workspace, err := s.workspaceManager.GetWorkspaceForSender(s.from)
	if err != nil {
		log.Printf("DEBUG: Could not get workspace for sender %s: %v", s.from, err)
		return
	}
	
	// Defensive check for nil workspace
	if workspace == nil {
		log.Printf("Warning: GetWorkspaceForSender returned nil workspace for %s", s.from)
		return
	}

	log.Printf("DEBUG: Found workspace: ID='%s', Domain='%s', HasMailgun=%v, HasGmail=%v",
		workspace.ID, workspace.Domain, workspace.Mailgun != nil, workspace.Gmail != nil)

	// Process Gmail workspaces with header rewriting enabled
	if workspace.Gmail != nil && workspace.Gmail.Enabled && workspace.Gmail.HeaderRewrite.Enabled {
		log.Printf("DEBUG: Processing %d Gmail header rewrite rules for workspace %s", 
			len(workspace.Gmail.HeaderRewrite.Rules), workspace.ID)
		
		// Check each rewrite rule to see if the header exists
		for i, rule := range workspace.Gmail.HeaderRewrite.Rules {
			log.Printf("DEBUG: Processing Gmail rule %d: header_name='%s', new_value='%s'", i, rule.HeaderName, rule.NewValue)
			
			if rule.HeaderName == "" {
				log.Printf("DEBUG: Gmail rule %d has empty header_name, skipping", i)
				continue
			}
			
			// Check if header already exists (case-insensitive)
			headerExists := false
			for existingHeaderName := range s.message.Headers {
				if strings.EqualFold(existingHeaderName, rule.HeaderName) {
					log.Printf("DEBUG: Header %s already exists as %s", rule.HeaderName, existingHeaderName)
					headerExists = true
					// If the rule has no new_value, remove the existing header
					if rule.NewValue == "" {
						log.Printf("DEBUG: Removing existing header %s for Gmail workspace %s", existingHeaderName, workspace.ID)
						delete(s.message.Headers, existingHeaderName)
					}
					break
				}
			}
			
			// If header doesn't exist and we have a new value, add it
			if !headerExists && rule.NewValue != "" {
				log.Printf("DEBUG: Adding missing header %s with value: %s", rule.HeaderName, rule.NewValue)
				s.message.Headers[rule.HeaderName] = rule.NewValue
				log.Printf("Added missing header %s to Gmail workspace %s: %s",
					rule.HeaderName, workspace.ID, rule.NewValue)
			}
		}
	}
	
	// Process Mailgun workspaces with header rewriting enabled
	if workspace.Mailgun != nil && workspace.Mailgun.Enabled && workspace.Mailgun.HeaderRewrite.Enabled {
		log.Printf("DEBUG: Processing %d Mailgun header rewrite rules for workspace %s", 
			len(workspace.Mailgun.HeaderRewrite.Rules), workspace.ID)

		// Check each rewrite rule to see if the header exists
		for i, rule := range workspace.Mailgun.HeaderRewrite.Rules {
			log.Printf("DEBUG: Processing Mailgun rule %d: header_name='%s', new_value='%s'", i, rule.HeaderName, rule.NewValue)

			if rule.HeaderName == "" {
				log.Printf("DEBUG: Mailgun rule %d has empty header_name, skipping", i)
				continue
			}

			// Check if header already exists (case-insensitive)
			headerExists := false
			for existingHeaderName := range s.message.Headers {
				if strings.EqualFold(existingHeaderName, rule.HeaderName) {
					log.Printf("DEBUG: Header %s already exists as %s", rule.HeaderName, existingHeaderName)
					headerExists = true
					// If the rule has no new_value, remove the existing header
					if rule.NewValue == "" {
						log.Printf("DEBUG: Removing existing header %s for Mailgun workspace %s", existingHeaderName, workspace.ID)
						delete(s.message.Headers, existingHeaderName)
					}
					break
				}
			}

			// If header doesn't exist and we have a new value, add it
			if !headerExists && rule.NewValue != "" {
				log.Printf("DEBUG: Adding missing header %s with value: %s", rule.HeaderName, rule.NewValue)
				s.message.Headers[rule.HeaderName] = rule.NewValue
				log.Printf("Added missing header %s to Mailgun workspace %s: %s",
					rule.HeaderName, workspace.ID, rule.NewValue)
			}
		}
	}

	headerNames := make([]string, 0, len(s.message.Headers))
	for headerName := range s.message.Headers {
		headerNames = append(headerNames, headerName)
	}
	log.Printf("DEBUG: addMissingHeaders completed, message now has %d headers: %s", len(s.message.Headers), strings.Join(headerNames, ", "))
}

func parseAddresses(addresses string) []string {
	parts := strings.Split(addresses, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		addr := strings.TrimSpace(part)
		if addr != "" {
			result = append(result, addr)
		}
	}
	return result
}

func isHTML(contentType string) bool {
	return strings.Contains(strings.ToLower(contentType), "text/html")
}

// parseTagsHeader parses X-MC-Tags header which can be JSON array or comma-separated values
func parseTagsHeader(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	
	// Try parsing as JSON array first
	if strings.HasPrefix(value, "[") {
		var tags []string
		if err := json.Unmarshal([]byte(value), &tags); err == nil {
			return tags
		}
	}
	
	// Fall back to comma-separated values
	parts := strings.Split(value, ",")
	tags := make([]string, 0, len(parts))
	for _, part := range parts {
		tag := strings.TrimSpace(part)
		if tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}

// parseMCMetadata parses X-MC-Metadata JSON and populates message fields
func parseMCMetadata(jsonStr string, msg *models.Message) error {
	var metadata map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &metadata); err != nil {
		return err
	}
	
	// Extract invitation tracking fields
	if invitationID, ok := getStringFromInterface(metadata["invitation_id"]); ok && invitationID != "" {
		msg.InvitationID = invitationID
	}
	
	if emailType, ok := getStringFromInterface(metadata["email_type"]); ok && emailType != "" {
		msg.EmailType = emailType
	}
	
	if dispatchID, ok := getStringFromInterface(metadata["invitation_dispatch_id"]); ok && dispatchID != "" {
		msg.InvitationDispatchID = dispatchID
	}
	
	// Store all metadata for reference
	if msg.Metadata == nil {
		msg.Metadata = make(map[string]interface{})
	}
	msg.Metadata["mc_metadata"] = metadata
	
	return nil
}

// getStringFromInterface safely converts interface{} to string
func getStringFromInterface(v interface{}) (string, bool) {
	if v == nil {
		return "", false
	}
	switch s := v.(type) {
	case string:
		return s, true
	case float64:
		return fmt.Sprintf("%.0f", s), true
	case int:
		return fmt.Sprintf("%d", s), true
	default:
		return fmt.Sprintf("%v", s), true
	}
}
