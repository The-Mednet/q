package smtp

import (
	"crypto/subtle"
	"encoding/base64"
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

	// Determine workspace for this sender
	var workspaceID string
	if s.workspaceManager != nil {
		if workspace, err := s.workspaceManager.GetWorkspaceForSender(from); err == nil {
			workspaceID = workspace.ID
		} else {
			log.Printf("Warning: Could not route message from %s: %v", from, err)
		}
	}

	s.message = &models.Message{
		ID:          uuid.New().String(),
		From:        from,
		WorkspaceID: workspaceID,
		Status:      models.StatusQueued,
		QueuedAt:    time.Now(),
		Headers:     make(map[string]string),
		Metadata:    make(map[string]interface{}),
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
	if s.message == nil {
		return errors.New("no mail transaction in progress")
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

	for _, line := range lines {
		if headers {
			if line == "" || line == "\r" {
				headers = false
				continue
			}

			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])

				switch strings.ToLower(key) {
				case "subject":
					// Validate subject
					if err := validation.ValidateSubject(value); err != nil {
						log.Printf("Invalid subject: %v", err)
						value = validation.SanitizeString(value) // Sanitize instead of rejecting
					}
					s.message.Subject = value
				case "content-type":
					if strings.Contains(strings.ToLower(value), "text/html") {
						s.message.Headers["Content-Type"] = value
					}
				case "cc":
					s.message.CC = parseAddresses(value)
				case "bcc":
					s.message.BCC = parseAddresses(value)
				case "x-campaign-id":
					if err := validation.ValidateCampaignID(value); err != nil {
						log.Printf("Invalid campaign ID: %v", err)
						continue // Skip invalid campaign ID
					}
					s.message.CampaignID = value
				case "x-user-id":
					if err := validation.ValidateUserID(value); err != nil {
						log.Printf("Invalid user ID: %v", err)
						continue // Skip invalid user ID
					}
					s.message.UserID = value
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
		} else {
			body.WriteString(line)
			body.WriteString("\n")
		}
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

// modifyHeaderForProvider modifies headers based on the workspace provider type
func (s *Session) modifyHeaderForProvider(headerName, headerValue string) string {
	log.Printf("DEBUG: modifyHeaderForProvider called: header='%s', value='%s'", headerName, headerValue)

	// Only modify if we have a workspace ID and workspace manager
	if s.message == nil {
		log.Printf("DEBUG: s.message is nil, returning original value")
		return headerValue
	}
	if s.message.WorkspaceID == "" {
		log.Printf("DEBUG: s.message.WorkspaceID is empty, returning original value")
		return headerValue
	}
	if s.workspaceManager == nil {
		log.Printf("DEBUG: s.workspaceManager is nil, returning original value")
		return headerValue
	}

	log.Printf("DEBUG: Getting workspace for sender: %s", s.from)

	// Get workspace configuration to determine provider type
	workspace, err := s.workspaceManager.GetWorkspaceForSender(s.from)
	if err != nil {
		// If we can't determine workspace, return original value
		log.Printf("DEBUG: Could not get workspace for sender %s: %v", s.from, err)
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
	if strings.ToLower(headerName) != "list-unsubscribe" {
		return headerValue
	}

	// Default behavior: Replace common Mandrill patterns with Mailgun equivalents
	modifiedValue := headerValue

	// Replace Mandrill domains with Mailgun equivalents
	if strings.Contains(headerValue, "mandrillapp.com") {
		// Extract the domain from workspace config
		domain := workspace.GetPrimaryDomain()

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
	if s.message.WorkspaceID == "" {
		log.Printf("DEBUG: s.message.WorkspaceID is empty, skipping addMissingHeaders")
		return
	}
	if s.workspaceManager == nil {
		log.Printf("DEBUG: s.workspaceManager is nil, skipping addMissingHeaders")
		return
	}

	log.Printf("DEBUG: Getting workspace for sender: %s", s.from)

	// Get workspace configuration
	workspace, err := s.workspaceManager.GetWorkspaceForSender(s.from)
	if err != nil {
		log.Printf("DEBUG: Could not get workspace for sender %s: %v", s.from, err)
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
