package recipient

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"relay/pkg/models"
)

// WebhookHandler handles incoming webhook events for recipient engagement tracking
type WebhookHandler struct {
	recipientService *Service
}

// NewWebhookHandler creates a new webhook handler
func NewWebhookHandler(service *Service) *WebhookHandler {
	return &WebhookHandler{
		recipientService: service,
	}
}

// HandleMandrillWebhook handles incoming Mandrill-compatible webhook events
func (h *WebhookHandler) HandleMandrillWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse the webhook payload
	var events []models.MandrillWebhookEvent
	if err := json.NewDecoder(r.Body).Decode(&events); err != nil {
		log.Printf("Error decoding webhook payload: %v", err)
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	ctx := context.Background()

	// Process each event
	for _, event := range events {
		if err := h.processWebhookEvent(ctx, event, r); err != nil {
			log.Printf("Error processing webhook event %s: %v", event.Event, err)
			// Continue processing other events even if one fails
		}
	}

	// Return success response
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// processWebhookEvent processes a single webhook event
func (h *WebhookHandler) processWebhookEvent(ctx context.Context, event models.MandrillWebhookEvent, r *http.Request) error {
	messageID := event.Msg.ID
	email := strings.ToLower(strings.TrimSpace(event.Msg.Email))

	// Get client IP and user agent for tracking
	clientIP := h.getClientIP(r)
	userAgent := r.Header.Get("User-Agent")

	// Prepare event data
	eventData := map[string]interface{}{
		"ts":         event.TS,
		"webhook_id": event.ID,
		"state":      event.Msg.State,
		"subject":    event.Msg.Subject,
		"sender":     event.Msg.Sender,
		"tags":       event.Msg.Tags,
		"metadata":   event.Msg.Metadata,
	}

	// Add event-specific data
	switch event.Event {
	case "send":
		// Message was successfully sent
		return h.recipientService.UpdateDeliveryStatus(messageID, email, models.DeliveryStatusSent, nil)

	case "open":
		// Email was opened
		return h.recipientService.RecordEngagementEvent(
			messageID, email, models.EventTypeOpen, eventData, &clientIP, &userAgent,
		)

	case "click":
		// Link was clicked
		if event.URL != "" {
			eventData["url"] = event.URL
		}
		return h.recipientService.RecordEngagementEvent(
			messageID, email, models.EventTypeClick, eventData, &clientIP, &userAgent,
		)

	case "bounce":
		// Email bounced
		bounceReason := fmt.Sprintf("Bounce: %s", event.Msg.State)
		return h.recipientService.UpdateDeliveryStatus(messageID, email, models.DeliveryStatusBounced, &bounceReason)

	case "reject":
		// Email was rejected
		rejectReason := fmt.Sprintf("Rejected: %s", event.Msg.State)
		return h.recipientService.UpdateDeliveryStatus(messageID, email, models.DeliveryStatusFailed, &rejectReason)

	case "spam":
		// Email was marked as spam
		return h.recipientService.RecordEngagementEvent(
			messageID, email, models.EventTypeComplaint, eventData, &clientIP, &userAgent,
		)

	case "unsub":
		// User unsubscribed
		return h.recipientService.RecordEngagementEvent(
			messageID, email, models.EventTypeUnsubscribe, eventData, &clientIP, &userAgent,
		)

	default:
		log.Printf("Unknown webhook event type: %s", event.Event)
		return nil
	}
}

// getClientIP extracts the real client IP from the request
func (h *WebhookHandler) getClientIP(r *http.Request) string {
	// Check for X-Forwarded-For header (common with load balancers/proxies)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP if multiple are present
		if ips := strings.Split(xff, ","); len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Check for X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	// Fallback to RemoteAddr
	if ip := strings.Split(r.RemoteAddr, ":"); len(ip) > 0 {
		return ip[0]
	}

	return r.RemoteAddr
}

// HandlePixelTracking handles pixel tracking for email opens
func (h *WebhookHandler) HandlePixelTracking(w http.ResponseWriter, r *http.Request) {
	// Get tracking parameters from URL query
	messageID := r.URL.Query().Get("mid")
	email := r.URL.Query().Get("email")

	if messageID == "" || email == "" {
		// Return 1x1 transparent pixel even for invalid requests
		h.serveTrackingPixel(w)
		return
	}

	// Normalize email
	email = strings.ToLower(strings.TrimSpace(email))

	// Get client information
	clientIP := h.getClientIP(r)
	userAgent := r.Header.Get("User-Agent")

	// Record the open event
	eventData := map[string]interface{}{
		"method":  "pixel",
		"referer": r.Header.Get("Referer"),
	}

	if err := h.recipientService.RecordEngagementEvent(messageID, email, models.EventTypeOpen, eventData, &clientIP, &userAgent); err != nil {
		log.Printf("Error recording pixel tracking open for message %s, email %s: %v", messageID, email, err)
	}

	// Always return the tracking pixel
	h.serveTrackingPixel(w)
}

// serveTrackingPixel serves a 1x1 transparent PNG pixel
func (h *WebhookHandler) serveTrackingPixel(w http.ResponseWriter) {
	// 1x1 transparent PNG in base64: iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==
	pixel := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4,
		0x89, 0x00, 0x00, 0x00, 0x0D, 0x49, 0x44, 0x41, 0x54, 0x78, 0xDA, 0x63, 0x64, 0x60, 0xF8, 0x5F,
		0x0F, 0x00, 0x02, 0x87, 0x01, 0x80, 0xEB, 0x47, 0xBA, 0x92, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45,
		0x4E, 0x44, 0xAE, 0x42, 0x60, 0x82,
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(pixel)))
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.WriteHeader(http.StatusOK)
	w.Write(pixel)
}

// HandleLinkTracking handles link click tracking with redirect
func (h *WebhookHandler) HandleLinkTracking(w http.ResponseWriter, r *http.Request) {
	// Get tracking parameters
	messageID := r.URL.Query().Get("mid")
	email := r.URL.Query().Get("email")
	targetURL := r.URL.Query().Get("url")

	if messageID == "" || email == "" || targetURL == "" {
		http.Error(w, "Missing required parameters", http.StatusBadRequest)
		return
	}

	// Normalize email
	email = strings.ToLower(strings.TrimSpace(email))

	// Get client information
	clientIP := h.getClientIP(r)
	userAgent := r.Header.Get("User-Agent")

	// Record the click event
	eventData := map[string]interface{}{
		"url":     targetURL,
		"referer": r.Header.Get("Referer"),
	}

	if err := h.recipientService.RecordEngagementEvent(messageID, email, models.EventTypeClick, eventData, &clientIP, &userAgent); err != nil {
		log.Printf("Error recording click tracking for message %s, email %s: %v", messageID, email, err)
	}

	// Redirect to the target URL
	http.Redirect(w, r, targetURL, http.StatusFound)
}

// HandleUnsubscribe handles unsubscribe requests
func (h *WebhookHandler) HandleUnsubscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		// Show unsubscribe form
		h.showUnsubscribeForm(w, r)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse form data
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	messageID := r.FormValue("mid")
	email := r.FormValue("email")

	if messageID == "" || email == "" {
		http.Error(w, "Missing required parameters", http.StatusBadRequest)
		return
	}

	// Normalize email
	email = strings.ToLower(strings.TrimSpace(email))

	// Get client information
	clientIP := h.getClientIP(r)
	userAgent := r.Header.Get("User-Agent")

	// Record the unsubscribe event
	eventData := map[string]interface{}{
		"method":  "form",
		"referer": r.Header.Get("Referer"),
	}

	if err := h.recipientService.RecordEngagementEvent(messageID, email, models.EventTypeUnsubscribe, eventData, &clientIP, &userAgent); err != nil {
		log.Printf("Error recording unsubscribe for message %s, email %s: %v", messageID, email, err)
		http.Error(w, "Failed to process unsubscribe", http.StatusInternalServerError)
		return
	}

	// Show success page
	h.showUnsubscribeSuccess(w)
}

// showUnsubscribeForm displays a simple unsubscribe confirmation form
func (h *WebhookHandler) showUnsubscribeForm(w http.ResponseWriter, r *http.Request) {
	messageID := r.URL.Query().Get("mid")
	email := r.URL.Query().Get("email")

	html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <title>Unsubscribe</title>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <style>
        body { font-family: Arial, sans-serif; max-width: 500px; margin: 50px auto; padding: 20px; }
        .form-group { margin-bottom: 20px; }
        button { background: #dc3545; color: white; padding: 10px 20px; border: none; border-radius: 4px; cursor: pointer; }
        button:hover { background: #c82333; }
    </style>
</head>
<body>
    <h2>Unsubscribe from Email List</h2>
    <p>Are you sure you want to unsubscribe <strong>%s</strong> from our mailing list?</p>
    <form method="POST">
        <input type="hidden" name="mid" value="%s">
        <input type="hidden" name="email" value="%s">
        <div class="form-group">
            <button type="submit">Yes, Unsubscribe Me</button>
        </div>
    </form>
</body>
</html>
`, email, messageID, email)

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(html))
}

// showUnsubscribeSuccess shows the unsubscribe success page
func (h *WebhookHandler) showUnsubscribeSuccess(w http.ResponseWriter) {
	html := `
<!DOCTYPE html>
<html>
<head>
    <title>Unsubscribed Successfully</title>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <style>
        body { font-family: Arial, sans-serif; max-width: 500px; margin: 50px auto; padding: 20px; text-align: center; }
        .success { color: #28a745; }
    </style>
</head>
<body>
    <h2 class="success">Successfully Unsubscribed</h2>
    <p>You have been successfully unsubscribed from our mailing list.</p>
    <p>You will no longer receive emails from us.</p>
</body>
</html>
`

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(html))
}
