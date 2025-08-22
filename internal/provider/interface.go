package provider

import (
	"context"
	"time"

	"relay/pkg/models"
)

// Provider defines the interface that all email providers must implement
type Provider interface {
	// Core functionality
	SendMessage(ctx context.Context, msg *models.Message) error
	GetType() ProviderType
	GetID() string
	
	// Health and status monitoring
	HealthCheck(ctx context.Context) error
	IsHealthy() bool
	GetLastError() error
	
	// Configuration information
	CanSendFromDomain(domain string) bool
	GetSupportedDomains() []string
	GetProviderInfo() ProviderInfo
}

// ProviderType represents the type of email provider
type ProviderType string

const (
	ProviderTypeGmail    ProviderType = "gmail"
	ProviderTypeMailgun  ProviderType = "mailgun"
	ProviderTypeMandrill ProviderType = "mandrill"
)

// ProviderInfo contains metadata about a provider
type ProviderInfo struct {
	ID           string            `json:"id"`
	Type         ProviderType      `json:"type"`
	DisplayName  string            `json:"display_name"`
	Domains      []string          `json:"domains"`
	Enabled      bool              `json:"enabled"`
	LastHealthy  *time.Time        `json:"last_healthy,omitempty"`
	LastError    *string           `json:"last_error,omitempty"`
	Capabilities []string          `json:"capabilities"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// SendResult contains the result of sending a message through a provider
type SendResult struct {
	MessageID   string            `json:"message_id"`
	ProviderID  string            `json:"provider_id"`
	Status      SendStatus        `json:"status"`
	SendTime    time.Duration     `json:"send_time"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Error       *string           `json:"error,omitempty"`
}

// SendStatus represents the status of a send operation
type SendStatus string

const (
	SendStatusSent   SendStatus = "sent"
	SendStatusFailed SendStatus = "failed"
	SendStatusDeferred SendStatus = "deferred"
)

// ProviderFactory is responsible for creating provider instances
type ProviderFactory interface {
	CreateGmailProvider(workspaceID string, config interface{}) (Provider, error)
	CreateMailgunProvider(workspaceID string, config interface{}) (Provider, error)
}