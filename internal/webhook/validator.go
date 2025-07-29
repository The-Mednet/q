package webhook

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"io"
	"net/http"
)

// ValidateMandrillSignature validates the X-Mandrill-Signature header
func ValidateMandrillSignature(webhookKey string, signature string, body []byte) bool {
	if webhookKey == "" {
		return true // Skip validation if no key is configured
	}

	expectedSig := GenerateMandrillSignature(webhookKey, body)
	return hmac.Equal([]byte(signature), []byte(expectedSig))
}

// GenerateMandrillSignature creates a Mandrill-compatible signature
func GenerateMandrillSignature(webhookKey string, body []byte) string {
	// Mandrill creates signatures by:
	// 1. Sorting POST parameters by key
	// 2. Concatenating key=value pairs
	// 3. HMAC-SHA1 with webhook key
	// 4. Base64 encoding
	
	// For JSON webhooks, we use the raw body
	mac := hmac.New(sha1.New, []byte(webhookKey))
	mac.Write(body)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// WebhookValidationMiddleware creates middleware for validating webhook signatures
func WebhookValidationMiddleware(webhookKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			signature := r.Header.Get("X-Mandrill-Signature")
			
			// Read body
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Failed to read request body", http.StatusBadRequest)
				return
			}
			
			// Validate signature
			if !ValidateMandrillSignature(webhookKey, signature, body) {
				http.Error(w, "Invalid signature", http.StatusUnauthorized)
				return
			}
			
			// Restore body for next handler
			r.Body = io.NopCloser(bytes.NewReader(body))
			
			next.ServeHTTP(w, r)
		})
	}
}