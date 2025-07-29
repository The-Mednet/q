package models

import (
	"time"
)

type Message struct {
	ID          string                 `json:"id"`
	From        string                 `json:"from"`
	To          []string               `json:"to"`
	CC          []string               `json:"cc,omitempty"`
	BCC         []string               `json:"bcc,omitempty"`
	Subject     string                 `json:"subject"`
	HTML        string                 `json:"html,omitempty"`
	Text        string                 `json:"text,omitempty"`
	Headers     map[string]string      `json:"headers,omitempty"`
	Attachments []Attachment           `json:"attachments,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	
	// Campaign and user tracking
	CampaignID  string `json:"campaign_id,omitempty"`
	UserID      string `json:"user_id,omitempty"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	
	Status      MessageStatus          `json:"status"`
	QueuedAt    time.Time              `json:"queued_at"`
	ProcessedAt *time.Time             `json:"processed_at,omitempty"`
	Error       string                 `json:"error,omitempty"`
}

type Attachment struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Content     []byte `json:"content"`
	ContentType string `json:"content_type"`
}

type MessageStatus string

const (
	StatusQueued     MessageStatus = "queued"
	StatusProcessing MessageStatus = "processing"
	StatusSent       MessageStatus = "sent"
	StatusFailed     MessageStatus = "failed"
	StatusAuthError  MessageStatus = "auth_error"
)

type MandrillWebhookEvent struct {
	Event   string                 `json:"event"`
	Msg     MandrillMessage        `json:"msg"`
	TS      int64                  `json:"ts"`
	ID      string                 `json:"_id"`
	IP      string                 `json:"ip,omitempty"`
	URL     string                 `json:"url,omitempty"`
	UserAgent string               `json:"user_agent,omitempty"`
}

type MandrillMessage struct {
	ID       string                 `json:"_id"`
	State    string                 `json:"state"`
	Email    string                 `json:"email"`
	Subject  string                 `json:"subject"`
	Sender   string                 `json:"sender"`
	Tags     []string               `json:"tags"`
	Opens    int                    `json:"opens"`
	Clicks   int                    `json:"clicks"`
	Metadata map[string]interface{} `json:"metadata"`
}