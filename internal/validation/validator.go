package validation

import (
	"fmt"
	"net/mail"
	"regexp"
	"strings"
)

var (
	// Email validation regex - RFC 5322 compliant
	emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	
	// UUID validation regex
	uuidRegex = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	
	// Domain validation regex
	domainRegex = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)
	
	// Invitation ID validation - alphanumeric and hyphens
	invitationRegex = regexp.MustCompile(`^[a-zA-Z0-9-_]+$`)
	
	// Header name validation - RFC 5322 compliant
	headerNameRegex = regexp.MustCompile(`^[!-9;-~]+$`)
	
	// Max lengths for fields (medical context - conservative)
	maxEmailLength    = 320  // RFC 5321
	maxSubjectLength  = 998  // RFC 5322
	maxBodyLength     = 25 * 1024 * 1024 // 25MB
	maxHeaderValue    = 2048
	maxRecipients     = 100
	maxInvitationID   = 255
)

// ValidateEmail validates an email address
func ValidateEmail(email string) error {
	if email == "" {
		return fmt.Errorf("email address cannot be empty")
	}
	
	if len(email) > maxEmailLength {
		return fmt.Errorf("email address too long (max %d characters)", maxEmailLength)
	}
	
	// Try to parse with mail.ParseAddress for better validation
	if _, err := mail.ParseAddress(email); err != nil {
		// Fallback to regex for simpler email formats
		if !emailRegex.MatchString(email) {
			return fmt.Errorf("invalid email format: %s", email)
		}
	}
	
	// Check for common injection patterns
	if strings.ContainsAny(email, "\r\n") {
		return fmt.Errorf("email contains illegal characters (CRLF)")
	}
	
	return nil
}

// ValidateEmailList validates a list of email addresses
func ValidateEmailList(emails []string) error {
	if len(emails) == 0 {
		return fmt.Errorf("at least one email address is required")
	}
	
	if len(emails) > maxRecipients {
		return fmt.Errorf("too many recipients (max %d)", maxRecipients)
	}
	
	seen := make(map[string]bool)
	for _, email := range emails {
		// Normalize email
		normalized := strings.TrimSpace(strings.ToLower(email))
		
		// Check for duplicates
		if seen[normalized] {
			return fmt.Errorf("duplicate email address: %s", email)
		}
		seen[normalized] = true
		
		// Validate individual email
		if err := ValidateEmail(email); err != nil {
			return fmt.Errorf("invalid email in list: %w", err)
		}
	}
	
	return nil
}

// ValidateSubject validates an email subject
func ValidateSubject(subject string) error {
	if len(subject) > maxSubjectLength {
		return fmt.Errorf("subject too long (max %d characters)", maxSubjectLength)
	}
	
	// Check for header injection
	if strings.ContainsAny(subject, "\r\n") {
		return fmt.Errorf("subject contains illegal characters (CRLF)")
	}
	
	return nil
}

// ValidateBody validates email body content
func ValidateBody(body string) error {
	if len(body) > maxBodyLength {
		return fmt.Errorf("body too large (max %d bytes)", maxBodyLength)
	}
	
	// Check for null bytes
	if strings.Contains(body, "\x00") {
		return fmt.Errorf("body contains null bytes")
	}
	
	return nil
}

// ValidateUUID validates a UUID string
func ValidateUUID(uuid string) error {
	if uuid == "" {
		return fmt.Errorf("UUID cannot be empty")
	}
	
	if !uuidRegex.MatchString(uuid) {
		return fmt.Errorf("invalid UUID format: %s", uuid)
	}
	
	return nil
}

// ValidateDomain validates a domain name
func ValidateDomain(domain string) error {
	if domain == "" {
		return fmt.Errorf("domain cannot be empty")
	}
	
	if len(domain) > 253 {
		return fmt.Errorf("domain too long (max 253 characters)")
	}
	
	if !domainRegex.MatchString(domain) {
		return fmt.Errorf("invalid domain format: %s", domain)
	}
	
	return nil
}

// ValidateInvitationID validates an invitation identifier
func ValidateInvitationID(id string) error {
	if id == "" {
		return nil // Invitation ID is optional
	}
	
	if len(id) > maxInvitationID {
		return fmt.Errorf("invitation ID too long (max %d characters)", maxInvitationID)
	}
	
	if !invitationRegex.MatchString(id) {
		return fmt.Errorf("invalid invitation ID format (only alphanumeric, hyphens, and underscores allowed)")
	}
	
	return nil
}

// ValidateEmailType validates an email type
func ValidateEmailType(emailType string) error {
	if emailType == "" {
		return nil // Email type is optional
	}
	
	validTypes := []string{"invite", "reminder", "follow_up", "notification", "transactional"}
	for _, validType := range validTypes {
		if emailType == validType {
			return nil
		}
	}
	
	return fmt.Errorf("invalid email type: %s", emailType)
}

// ValidateInvitationDispatchID validates an invitation dispatch identifier  
func ValidateInvitationDispatchID(id string) error {
	if id == "" {
		return nil // Dispatch ID is optional
	}
	
	if len(id) > maxInvitationID {
		return fmt.Errorf("invitation dispatch ID too long (max %d characters)", maxInvitationID)
	}
	
	// Check for SQL injection patterns
	if strings.ContainsAny(id, "';\"") {
		return fmt.Errorf("invitation dispatch ID contains illegal characters")
	}
	
	return nil
}

// ValidateHeaders validates email headers
func ValidateHeaders(headers map[string]string) error {
	for name, value := range headers {
		// Validate header name
		if !headerNameRegex.MatchString(name) {
			return fmt.Errorf("invalid header name: %s", name)
		}
		
		// Validate header value length
		if len(value) > maxHeaderValue {
			return fmt.Errorf("header value too long for %s (max %d characters)", name, maxHeaderValue)
		}
		
		// Check for header injection
		if strings.Contains(value, "\r") || strings.Contains(value, "\n") {
			// Allow CRLF only if properly folded (RFC 5322)
			if !isProperlyFoldedHeader(value) {
				return fmt.Errorf("header %s contains illegal line breaks", name)
			}
		}
		
		// Check for null bytes
		if strings.Contains(value, "\x00") {
			return fmt.Errorf("header %s contains null bytes", name)
		}
	}
	
	return nil
}

// isProperlyFoldedHeader checks if a header value is properly folded per RFC 5322
func isProperlyFoldedHeader(value string) bool {
	lines := strings.Split(value, "\r\n")
	for i, line := range lines {
		if i > 0 {
			// Continuation lines must start with whitespace
			if len(line) == 0 || (line[0] != ' ' && line[0] != '\t') {
				return false
			}
		}
	}
	return true
}

// SanitizeString removes potentially dangerous characters from a string
func SanitizeString(s string) string {
	// Remove null bytes
	s = strings.ReplaceAll(s, "\x00", "")
	
	// Remove control characters except tab, newline, carriage return
	result := make([]rune, 0, len(s))
	for _, r := range s {
		if r == '\t' || r == '\n' || r == '\r' || (r >= 32 && r < 127) || r > 127 {
			result = append(result, r)
		}
	}
	
	return string(result)
}

// ValidateProviderID validates a workspace identifier
func ValidateProviderID(id string) error {
	if id == "" {
		return fmt.Errorf("workspace ID cannot be empty")
	}
	
	if len(id) > 100 {
		return fmt.Errorf("workspace ID too long (max 100 characters)")
	}
	
	// Only allow alphanumeric, hyphens, and underscores
	if !regexp.MustCompile(`^[a-zA-Z0-9-_]+$`).MatchString(id) {
		return fmt.Errorf("invalid workspace ID format")
	}
	
	return nil
}