package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"smtp_relay/internal/config"
	"smtp_relay/pkg/models"
)

type Client struct {
	config     *config.WebhookConfig
	httpClient *http.Client
}

func NewClient(cfg *config.WebhookConfig) *Client {
	return &Client{
		config: cfg,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

func (c *Client) SendEvent(ctx context.Context, msg *models.Message, eventType string, details map[string]interface{}) error {
	if c.config.MandrillURL == "" {
		return nil // No webhook URL configured
	}

	event := c.createMandrillEvent(msg, eventType, details)

	// Mandrill sends events in batches, but we'll send individually for simplicity
	payload := []models.MandrillWebhookEvent{event}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.config.MandrillURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create webhook request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Mandrill-Signature", c.generateSignature(jsonData))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
}

func (c *Client) SendSentEvent(ctx context.Context, msg *models.Message) error {
	return c.SendEvent(ctx, msg, "send", map[string]interface{}{
		"smtp_events": []map[string]interface{}{
			{
				"ts":             time.Now().Unix(),
				"type":           "sent",
				"diag":           "250 2.0.0 OK",
				"source_ip":      "127.0.0.1",
				"destination_ip": "gmail-smtp-in.l.google.com",
				"size":           len(msg.HTML) + len(msg.Text),
			},
		},
	})
}

func (c *Client) SendDeferredEvent(ctx context.Context, msg *models.Message, reason string) error {
	return c.SendEvent(ctx, msg, "deferral", map[string]interface{}{
		"smtp_events": []map[string]interface{}{
			{
				"ts":             time.Now().Unix(),
				"type":           "deferred",
				"diag":           reason,
				"source_ip":      "127.0.0.1",
				"destination_ip": "gmail-smtp-in.l.google.com",
				"size":           len(msg.HTML) + len(msg.Text),
			},
		},
	})
}

func (c *Client) SendBounceEvent(ctx context.Context, msg *models.Message, reason string) error {
	return c.SendEvent(ctx, msg, "hard_bounce", map[string]interface{}{
		"bounce_description": reason,
		"diag":               reason,
	})
}

func (c *Client) SendRejectEvent(ctx context.Context, msg *models.Message, reason string) error {
	return c.SendEvent(ctx, msg, "reject", map[string]interface{}{
		"reject": map[string]interface{}{
			"reason":      "hard-bounce",
			"detail":      reason,
			"last_event":  "failed",
			"description": reason,
		},
	})
}

func (c *Client) createMandrillEvent(msg *models.Message, eventType string, details map[string]interface{}) models.MandrillWebhookEvent {
	// Extract the first recipient for the event
	email := ""
	if len(msg.To) > 0 {
		email = msg.To[0]
	}

	mandrillMsg := models.MandrillMessage{
		ID:       msg.ID,
		State:    c.mapEventTypeToState(eventType),
		Email:    email,
		Subject:  msg.Subject,
		Sender:   msg.From,
		Tags:     c.extractTags(msg),
		Opens:    0,
		Clicks:   0,
		Metadata: msg.Metadata,
	}

	// Merge additional details into the message
	if details != nil {
		for k, v := range details {
			switch k {
			case "bounce_description":
				mandrillMsg.Metadata["bounce_description"] = v
			case "reject":
				mandrillMsg.Metadata["reject"] = v
			case "smtp_events":
				mandrillMsg.Metadata["smtp_events"] = v
			}
		}
	}

	return models.MandrillWebhookEvent{
		Event: eventType,
		Msg:   mandrillMsg,
		TS:    time.Now().Unix(),
		ID:    msg.ID,
	}
}

func (c *Client) mapEventTypeToState(eventType string) string {
	switch eventType {
	case "send":
		return "sent"
	case "deferral":
		return "deferred"
	case "hard_bounce", "soft_bounce":
		return "bounced"
	case "reject":
		return "rejected"
	case "spam":
		return "spam"
	case "unsub":
		return "unsub"
	default:
		return eventType
	}
}

func (c *Client) extractTags(msg *models.Message) []string {
	tags := []string{}

	// Extract tags from headers or metadata
	if tagHeader, ok := msg.Headers["X-MC-Tags"]; ok {
		tags = append(tags, tagHeader)
	}

	if metaTags, ok := msg.Metadata["tags"].([]string); ok {
		tags = append(tags, metaTags...)
	}

	return tags
}

func (c *Client) generateSignature(data []byte) string {
	// In a real implementation, this would generate a proper HMAC signature
	// using a webhook key. For now, we'll return a placeholder.
	// Mandrill uses: base64(HMAC-SHA1(webhook_key, POST_data))
	return "placeholder-signature"
}

// Retry logic for webhook delivery
func (c *Client) SendEventWithRetry(ctx context.Context, msg *models.Message, eventType string, details map[string]interface{}) error {
	var lastErr error

	for i := 0; i < c.config.MaxRetries; i++ {
		err := c.SendEvent(ctx, msg, eventType, details)
		if err == nil {
			return nil
		}

		lastErr = err

		// Exponential backoff
		if i < c.config.MaxRetries-1 {
			select {
			case <-time.After(time.Duration(1<<uint(i)) * time.Second):
				// Continue to next retry
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	return fmt.Errorf("webhook failed after %d retries: %w", c.config.MaxRetries, lastErr)
}
