package provider

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"relay/pkg/models"
)

// MandrillProviderWrapper wraps the Mandrill provider to implement the Provider interface
type MandrillProviderWrapper struct {
	provider    *MandrillProvider
	workspaceID string
	domain      string   // Primary domain (for backward compatibility)
	domains     []string // All domains this provider serves
	healthy     bool
	lastError   error
	mu          sync.RWMutex
}

// GetID returns the provider ID
func (m *MandrillProviderWrapper) GetID() string {
	return fmt.Sprintf("mandrill_%s", m.workspaceID)
}

// GetType returns the provider type
func (m *MandrillProviderWrapper) GetType() ProviderType {
	return ProviderTypeMandrill
}

// IsEnabled returns whether the provider is enabled
func (m *MandrillProviderWrapper) IsEnabled() bool {
	return m.provider.IsEnabled()
}

// GetRateLimits returns the provider's rate limits
func (m *MandrillProviderWrapper) GetRateLimits() (daily int, perUser int) {
	return m.provider.GetRateLimits()
}

// HealthCheck verifies the provider is accessible
func (m *MandrillProviderWrapper) HealthCheck(ctx context.Context) error {
	err := m.provider.HealthCheck(ctx)
	
	m.mu.Lock()
	if err != nil {
		m.healthy = false
		m.lastError = err
	} else {
		m.healthy = true
		m.lastError = nil
	}
	m.mu.Unlock()
	
	return err
}

// SendMessage sends a message via the provider
func (m *MandrillProviderWrapper) SendMessage(ctx context.Context, msg *models.Message) error {
	err := m.provider.SendEmail(ctx, msg, nil)
	
	if err != nil {
		m.mu.Lock()
		m.lastError = err
		m.mu.Unlock()
	}
	
	return err
}

// GetLastError returns the last error encountered
func (m *MandrillProviderWrapper) GetLastError() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastError
}

// CanSendFromDomain checks if this provider can send from the given domain
func (m *MandrillProviderWrapper) CanSendFromDomain(domain string) bool {
	// Mandrill can send from any domain (doesn't require domain verification)
	// But we restrict it to the configured domains for consistency
	for _, d := range m.domains {
		if d == domain {
			return true
		}
	}
	// Also check legacy single domain field
	return domain == m.domain
}

// GetSupportedDomains returns the domains this provider supports
func (m *MandrillProviderWrapper) GetSupportedDomains() []string {
	if len(m.domains) > 0 {
		return m.domains
	}
	// Fallback to legacy single domain
	if m.domain != "" {
		return []string{m.domain}
	}
	return []string{}
}

// IsHealthy returns whether the provider is healthy
func (m *MandrillProviderWrapper) IsHealthy() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.healthy
}

// GetProviderInfo returns provider information
func (m *MandrillProviderWrapper) GetProviderInfo() ProviderInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	var lastErrorStr *string
	if m.lastError != nil {
		err := m.lastError.Error()
		lastErrorStr = &err
	}
	
	// Convert capabilities map to slice
	capabilities := make([]string, 0)
	for cap, enabled := range m.provider.GetCapabilities() {
		if enabled {
			capabilities = append(capabilities, cap)
		}
	}
	
	// Get display name with all domains
	displayName := "Mandrill"
	if len(m.domains) > 0 {
		displayName = fmt.Sprintf("Mandrill - %v", m.domains)
	} else if m.domain != "" {
		displayName = fmt.Sprintf("Mandrill - %s", m.domain)
	}
	
	return ProviderInfo{
		ID:           m.GetID(),
		Type:         m.GetType(),
		DisplayName:  displayName,
		Domains:      m.GetSupportedDomains(),
		Enabled:      m.IsEnabled(),
		LastError:    lastErrorStr,
		Capabilities: capabilities,
	}
}

// ValidateSender checks if a sender email is valid for this provider
func (m *MandrillProviderWrapper) ValidateSender(ctx context.Context, senderEmail string) error {
	return m.provider.ValidateSender(ctx, senderEmail)
}

// GetCapabilities returns the provider's capabilities
func (m *MandrillProviderWrapper) GetCapabilities() map[string]bool {
	return m.provider.GetCapabilities()
}

// GetStatus returns the current status of the provider
func (m *MandrillProviderWrapper) GetStatus(ctx context.Context) (map[string]interface{}, error) {
	status, err := m.provider.GetStatus(ctx)
	if err != nil {
		m.mu.Lock()
		m.lastError = err
		m.mu.Unlock()
		return nil, err
	}
	
	// Add wrapper-specific information
	status["provider_id"] = m.workspaceID
	status["domain"] = m.domain
	
	m.mu.RLock()
	status["healthy"] = m.healthy
	if m.lastError != nil {
		status["last_error"] = m.lastError.Error()
	}
	m.mu.RUnlock()
	
	return status, nil
}

// Shutdown performs any cleanup needed
func (m *MandrillProviderWrapper) Shutdown() error {
	log.Printf("Shutting down Mandrill provider for workspace %s", m.workspaceID)
	return m.provider.Shutdown()
}

// StartHealthCheck starts periodic health checking (if needed)
func (m *MandrillProviderWrapper) StartHealthCheck(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := m.HealthCheck(ctx); err != nil {
					log.Printf("Health check failed for Mandrill provider %s: %v", m.GetID(), err)
				}
			}
		}
	}()
}