package mailgun

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"relay/pkg/models"
)

// MailgunWebhookHandler handles incoming webhooks from Mailgun
type MailgunWebhookHandler struct {
	signingKey       string
	recipientService RecipientService // Interface for updating recipient status
}

// RecipientService interface for updating recipient delivery status
type RecipientService interface {
	UpdateDeliveryStatus(messageID, recipientEmail string, status models.DeliveryStatus, bounceReason *string) error
	RecordEvent(messageID, recipientEmail string, eventType models.EventType, eventData map[string]interface{}) error
}

// MailgunWebhookEvent represents a Mailgun webhook event
type MailgunWebhookEvent struct {
	EventData MailgunEventData `json:"event-data"`
	Signature MailgunSignature `json:"signature"`
}

// MailgunEventData contains the actual event information
type MailgunEventData struct {
	Event       string                 `json:"event"`
	Timestamp   float64                `json:"timestamp"`
	ID          string                 `json:"id"`
	Message     MailgunMessageData     `json:"message"`
	Recipient   string                 `json:"recipient"`
	Domain      string                 `json:"domain"`
	Reason      string                 `json:"reason,omitempty"`
	Description string                 `json:"description,omitempty"`
	Tags        []string               `json:"tags,omitempty"`
	UserVars    map[string]interface{} `json:"user-variables,omitempty"`
	URL         string                 `json:"url,omitempty"`         // For clicks
	IP          string                 `json:"ip,omitempty"`          // For opens/clicks
	UserAgent   string                 `json:"user-agent,omitempty"`  // For opens/clicks
	DeviceType  string                 `json:"device-type,omitempty"` // For opens
	ClientType  string                 `json:"client-type,omitempty"` // For opens
	ClientName  string                 `json:"client-name,omitempty"` // For opens
	ClientOS    string                 `json:"client-os,omitempty"`   // For opens
}

// MailgunMessageData contains message-specific information
type MailgunMessageData struct {
	Headers     map[string]string `json:"headers"`
	Attachments []interface{}     `json:"attachments,omitempty"`
	Size        int               `json:"size,omitempty"`
}

// MailgunSignature contains webhook signature for verification
type MailgunSignature struct {
	Token     string `json:"token"`
	Timestamp string `json:"timestamp"`
	Signature string `json:"signature"`
}

// NewMailgunWebhookHandler creates a new webhook handler
func NewMailgunWebhookHandler(signingKey string, recipientService RecipientService) *MailgunWebhookHandler {
	return &MailgunWebhookHandler{
		signingKey:       signingKey,
		recipientService: recipientService,
	}
}

// HandleWebhook processes incoming Mailgun webhooks
func (mwh *MailgunWebhookHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	// Parse the webhook event
	var event MailgunWebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		http.Error(w, "Failed to parse webhook data", http.StatusBadRequest)
		return
	}

	// Verify the webhook signature
	if mwh.signingKey != "" {
		if !mwh.verifySignature(event.Signature, body) {
			http.Error(w, "Invalid signature", http.StatusUnauthorized)
			return
		}
	}

	// Process the event
	if err := mwh.processEvent(context.Background(), &event); err != nil {
		http.Error(w, fmt.Sprintf("Failed to process event: %v", err), http.StatusInternalServerError)
		return
	}

	// Return success
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// verifySignature verifies the Mailgun webhook signature
func (mwh *MailgunWebhookHandler) verifySignature(sig MailgunSignature, body []byte) bool {
	// Mailgun signature verification
	// signature = hmac-sha256(key=api_key, msg=timestamp + token)
	message := sig.Timestamp + sig.Token

	mac := hmac.New(sha256.New, []byte(mwh.signingKey))
	mac.Write([]byte(message))
	expectedSignature := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(sig.Signature), []byte(expectedSignature))
}

// processEvent processes a webhook event and updates recipient status
func (mwh *MailgunWebhookHandler) processEvent(ctx context.Context, event *MailgunWebhookEvent) error {
	if mwh.recipientService == nil {
		return fmt.Errorf("recipient service not configured")
	}

	// Extract message ID from user variables
	messageID := mwh.extractMessageID(event)
	if messageID == "" {
		// If no message ID, we can't track this event
		return fmt.Errorf("no message ID found in webhook event")
	}

	recipient := event.EventData.Recipient
	if recipient == "" {
		return fmt.Errorf("no recipient found in webhook event")
	}

	// Convert recipient to lowercase for consistent matching
	recipient = strings.ToLower(strings.TrimSpace(recipient))

	// Process based on event type
	switch event.EventData.Event {
	case "delivered":
		return mwh.recipientService.UpdateDeliveryStatus(messageID, recipient, models.DeliveryStatusSent, nil)

	case "failed", "bounced":
		reason := mwh.buildBounceReason(event)
		status := models.DeliveryStatusFailed
		if event.EventData.Event == "bounced" {
			status = models.DeliveryStatusBounced
		}
		return mwh.recipientService.UpdateDeliveryStatus(messageID, recipient, status, &reason)

	case "rejected":
		reason := mwh.buildBounceReason(event)
		return mwh.recipientService.UpdateDeliveryStatus(messageID, recipient, models.DeliveryStatusFailed, &reason)

	case "opened":
		// Record open event
		eventData := map[string]interface{}{
			"timestamp":   event.EventData.Timestamp,
			"ip":          event.EventData.IP,
			"user_agent":  event.EventData.UserAgent,
			"device_type": event.EventData.DeviceType,
			"client_type": event.EventData.ClientType,
			"client_name": event.EventData.ClientName,
			"client_os":   event.EventData.ClientOS,
		}
		return mwh.recipientService.RecordEvent(messageID, recipient, models.EventTypeOpen, eventData)

	case "clicked":
		// Record click event
		eventData := map[string]interface{}{
			"timestamp":  event.EventData.Timestamp,
			"url":        event.EventData.URL,
			"ip":         event.EventData.IP,
			"user_agent": event.EventData.UserAgent,
		}
		return mwh.recipientService.RecordEvent(messageID, recipient, models.EventTypeClick, eventData)

	case "unsubscribed":
		// Record unsubscribe event
		eventData := map[string]interface{}{
			"timestamp":  event.EventData.Timestamp,
			"ip":         event.EventData.IP,
			"user_agent": event.EventData.UserAgent,
		}
		return mwh.recipientService.RecordEvent(messageID, recipient, models.EventTypeUnsubscribe, eventData)

	case "complained":
		// Record complaint event
		eventData := map[string]interface{}{
			"timestamp": event.EventData.Timestamp,
		}
		return mwh.recipientService.RecordEvent(messageID, recipient, models.EventTypeComplaint, eventData)

	default:
		// Unknown event type, log but don't error
		return nil
	}
}

// extractMessageID extracts the message ID from user variables
func (mwh *MailgunWebhookHandler) extractMessageID(event *MailgunWebhookEvent) string {
	if event.EventData.UserVars == nil {
		return ""
	}

	// Try various possible keys for message ID
	keys := []string{"message_id", "messageId", "msg_id", "id"}
	for _, key := range keys {
		if val, exists := event.EventData.UserVars[key]; exists {
			if strVal, ok := val.(string); ok && strVal != "" {
				return strVal
			}
		}
	}

	return ""
}

// buildBounceReason builds a human-readable bounce reason
func (mwh *MailgunWebhookHandler) buildBounceReason(event *MailgunWebhookEvent) string {
	parts := make([]string, 0, 3)

	if event.EventData.Reason != "" {
		parts = append(parts, event.EventData.Reason)
	}

	if event.EventData.Description != "" {
		parts = append(parts, event.EventData.Description)
	}

	if len(parts) == 0 {
		return fmt.Sprintf("Mailgun %s event", event.EventData.Event)
	}

	return strings.Join(parts, ": ")
}

// MailgunWebhookStats represents statistics about webhook processing
type MailgunWebhookStats struct {
	TotalProcessed int64            `json:"total_processed"`
	EventCounts    map[string]int64 `json:"event_counts"`
	ErrorCount     int64            `json:"error_count"`
	LastProcessed  *time.Time       `json:"last_processed,omitempty"`
	ProcessingTime time.Duration    `json:"average_processing_time"`
}

// GetStats returns webhook processing statistics (would be implemented with actual tracking)
func (mwh *MailgunWebhookHandler) GetStats() MailgunWebhookStats {
	// This would be implemented with actual statistics tracking
	return MailgunWebhookStats{
		TotalProcessed: 0,
		EventCounts:    make(map[string]int64),
		ErrorCount:     0,
	}
}

// ConvertToMandrillEvent converts a Mailgun webhook event to Mandrill format for compatibility
func (mwh *MailgunWebhookHandler) ConvertToMandrillEvent(event *MailgunWebhookEvent) models.MandrillWebhookEvent {
	// Convert Mailgun event to Mandrill-compatible format
	mandrillEvent := models.MandrillWebhookEvent{
		Event:     mwh.mapEventType(event.EventData.Event),
		TS:        int64(event.EventData.Timestamp),
		ID:        event.EventData.ID,
		IP:        event.EventData.IP,
		URL:       event.EventData.URL,
		UserAgent: event.EventData.UserAgent,
		Msg: models.MandrillMessage{
			ID:       mwh.extractMessageID(event),
			Email:    event.EventData.Recipient,
			State:    mwh.mapEventToState(event.EventData.Event),
			Tags:     event.EventData.Tags,
			Metadata: make(map[string]interface{}),
		},
	}

	// Copy user variables to metadata
	if event.EventData.UserVars != nil {
		mandrillEvent.Msg.Metadata = event.EventData.UserVars
	}

	return mandrillEvent
}

// mapEventType maps Mailgun event types to Mandrill event types
func (mwh *MailgunWebhookHandler) mapEventType(mailgunEvent string) string {
	switch mailgunEvent {
	case "delivered":
		return "send"
	case "bounced":
		return "hard_bounce"
	case "failed":
		return "reject"
	case "rejected":
		return "reject"
	case "opened":
		return "open"
	case "clicked":
		return "click"
	case "unsubscribed":
		return "unsub"
	case "complained":
		return "spam"
	default:
		return mailgunEvent
	}
}

// mapEventToState maps Mailgun event types to Mandrill message states
func (mwh *MailgunWebhookHandler) mapEventToState(mailgunEvent string) string {
	switch mailgunEvent {
	case "delivered":
		return "sent"
	case "bounced":
		return "bounced"
	case "failed", "rejected":
		return "rejected"
	default:
		return "sent"
	}
}
