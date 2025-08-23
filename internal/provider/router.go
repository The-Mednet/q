package provider

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"relay/internal/config"
	"relay/internal/workspace"
	"relay/pkg/models"
)

// Router handles routing messages to the appropriate email provider
type Router struct {
	workspaceManager *workspace.Manager
	providers        map[string]Provider // keyed by provider ID
	providersByDomain map[string][]Provider // keyed by domain
	mu               sync.RWMutex
}

// NewRouter creates a new provider router
func NewRouter(workspaceManager *workspace.Manager) *Router {
	// Defensive programming: validate input
	if workspaceManager == nil {
		log.Printf("Warning: WorkspaceManager is nil - provider routing will be limited")
	}
	
	return &Router{
		workspaceManager: workspaceManager,
		providers:        make(map[string]Provider),
		providersByDomain: make(map[string][]Provider),
	}
}

// InitializeProviders creates and registers providers based on workspace configuration
func (r *Router) InitializeProviders() error {
	// Defensive programming: validate router and components
	if r == nil {
		return fmt.Errorf("router is nil")
	}
	if r.workspaceManager == nil {
		return fmt.Errorf("workspace manager is nil - cannot initialize providers")
	}
	
	r.mu.Lock()
	defer r.mu.Unlock()
	
	workspaces := r.workspaceManager.GetAllWorkspaces()
	if len(workspaces) == 0 {
		return fmt.Errorf("no workspaces configured")
	}
	
	log.Printf("Initializing providers for %d workspaces", len(workspaces))
	
	for workspaceID, workspace := range workspaces {
		// Get all domains for this workspace
		domains := workspace.Domains
		if len(domains) == 0 && workspace.Domain != "" {
			// Backward compatibility
			domains = []string{workspace.Domain}
		}
		
		// Use first domain as primary (for legacy providers that need a single domain)
		primaryDomain := ""
		if len(domains) > 0 {
			primaryDomain = domains[0]
		}
		
		// Initialize Gmail provider if configured and enabled
		if workspace.Gmail != nil && workspace.Gmail.Enabled {
			provider, err := NewGmailProvider(workspaceID, domains, workspace.Gmail)
			if err != nil {
				log.Printf("Warning: Failed to create Gmail provider for workspace %s: %v", workspaceID, err)
				continue
			}
			
			providerID := provider.GetID()
			r.providers[providerID] = provider
			
			// Add provider for all domains
			for _, domain := range domains {
				r.addProviderForDomain(domain, provider)
			}
			
			log.Printf("Initialized Gmail provider %s for domains %v", providerID, domains)
		}
		
		// Initialize Mailgun provider if configured and enabled
		if workspace.Mailgun != nil && workspace.Mailgun.Enabled {
			provider, err := NewMailgunProvider(workspaceID, domains, workspace.Mailgun)
			if err != nil {
				log.Printf("Warning: Failed to create Mailgun provider for workspace %s: %v", workspaceID, err)
				continue
			}
			
			providerID := provider.GetID()
			r.providers[providerID] = provider
			
			// Add provider for all domains
			for _, domain := range domains {
				r.addProviderForDomain(domain, provider)
			}
			
			log.Printf("Initialized Mailgun provider %s for domains %v", providerID, domains)
		}
		
		// Initialize Mandrill provider if configured and enabled
		if workspace.Mandrill != nil && workspace.Mandrill.Enabled {
			provider, err := NewMandrillProvider(workspace.Mandrill)
			if err != nil {
				log.Printf("Warning: Failed to create Mandrill provider for workspace %s: %v", workspaceID, err)
				continue
			}
			
			// Create a wrapped provider for consistent interface
			wrappedProvider := &MandrillProviderWrapper{
				provider:    provider,
				workspaceID: workspaceID,
				domain:      primaryDomain,
				domains:     domains,  // Store all domains
			}
			
			providerID := fmt.Sprintf("mandrill_%s", workspaceID)
			r.providers[providerID] = wrappedProvider
			
			// Add provider for all domains
			for _, domain := range domains {
				r.addProviderForDomain(domain, wrappedProvider)
			}
			
			log.Printf("Initialized Mandrill provider %s for domains %v", providerID, domains)
		}
	}
	
	if len(r.providers) == 0 {
		return fmt.Errorf("no providers could be initialized")
	}
	
	log.Printf("Successfully initialized %d providers", len(r.providers))
	return nil
}

// addProviderForDomain adds a provider to the domain mapping
func (r *Router) addProviderForDomain(domain string, provider Provider) {
	if r.providersByDomain[domain] == nil {
		r.providersByDomain[domain] = make([]Provider, 0)
	}
	r.providersByDomain[domain] = append(r.providersByDomain[domain], provider)
	log.Printf("DEBUG: Added provider %s for domain %s (total providers for domain: %d)", provider.GetID(), domain, len(r.providersByDomain[domain]))
}

// RouteMessage routes a message to the appropriate provider based on sender domain
func (r *Router) RouteMessage(ctx context.Context, msg *models.Message) (Provider, error) {
	// Defensive programming: validate router and inputs
	if r == nil {
		return nil, fmt.Errorf("router is nil")
	}
	if msg == nil {
		return nil, fmt.Errorf("message cannot be nil")
	}
	if msg.From == "" {
		return nil, fmt.Errorf("sender email is required for routing")
	}
	if r.workspaceManager == nil {
		return nil, fmt.Errorf("workspace manager is nil - cannot route message")
	}
	
	// Extract domain from sender email
	domain, err := r.extractDomainFromEmail(msg.From)
	if err != nil {
		return nil, fmt.Errorf("failed to extract domain from sender email %s: %w", msg.From, err)
	}
	
	// Find providers for this domain
	r.mu.RLock()
	providers, exists := r.providersByDomain[domain]
	// Debug: log all registered domains
	log.Printf("DEBUG: Looking for providers for domain %s", domain)
	log.Printf("DEBUG: Registered domains: %v", func() []string {
		domains := make([]string, 0, len(r.providersByDomain))
		for d := range r.providersByDomain {
			domains = append(domains, d)
		}
		return domains
	}())
	r.mu.RUnlock()
	
	if !exists || len(providers) == 0 {
		return nil, fmt.Errorf("no providers configured for domain: %s", domain)
	}
	
	// Get workspace configuration to determine provider preference
	workspace, err := r.workspaceManager.GetWorkspaceByDomain(domain)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace for domain %s: %w", domain, err)
	}
	
	// Defensive check for nil workspace
	if workspace == nil {
		log.Printf("Warning: GetWorkspaceByDomain returned nil workspace for domain %s", domain)
		return nil, fmt.Errorf("workspace configuration is nil for domain %s", domain)
	}
	
	// Set workspace ID on message
	msg.WorkspaceID = workspace.ID
	
	// Route based on provider preference and availability
	provider, err := r.selectProvider(providers, workspace)
	if err != nil {
		return nil, fmt.Errorf("failed to select provider for domain %s: %w", domain, err)
	}
	
	log.Printf("Routed message from %s to provider %s (%s)", msg.From, provider.GetID(), provider.GetType())
	return provider, nil
}

// selectProvider selects the best provider based on configuration and health
func (r *Router) selectProvider(providers []Provider, workspace *config.WorkspaceConfig) (Provider, error) {
	// Defensive programming: validate inputs
	if len(providers) == 0 {
		return nil, fmt.Errorf("no providers available")
	}
	if workspace == nil {
		log.Printf("Warning: Workspace is nil in selectProvider - using first available provider")
		// Fallback to first healthy provider
		for _, provider := range providers {
			if provider != nil && provider.IsHealthy() {
				return provider, nil
			}
		}
		// If no healthy providers, use the first non-nil one
		for _, provider := range providers {
			if provider != nil {
				log.Printf("Warning: Using unhealthy provider %s as fallback", provider.GetID())
				return provider, nil
			}
		}
		return nil, fmt.Errorf("all providers are nil")
	}
	
	// If only one provider, use it if healthy
	if len(providers) == 1 {
		provider := providers[0]
		if provider.IsHealthy() {
			return provider, nil
		}
		// Even if unhealthy, try it - it might recover
		log.Printf("Warning: Using unhealthy provider %s as it's the only option", provider.GetID())
		return provider, nil
	}
	
	// Multiple providers available - use preference order and health
	var gmailProvider, mailgunProvider Provider
	
	for _, provider := range providers {
		// Defensive check for nil provider
		if provider == nil {
			log.Printf("Warning: Skipping nil provider in selection")
			continue
		}
		
		switch provider.GetType() {
		case ProviderTypeGmail:
			gmailProvider = provider
		case ProviderTypeMailgun:
			mailgunProvider = provider
		}
	}
	
	// Determine preference based on workspace configuration
	// If both providers are configured, prefer Gmail by default unless Mailgun is specifically prioritized
	preferGmail := workspace.Gmail != nil && workspace.Gmail.Enabled
	preferMailgun := workspace.Mailgun != nil && workspace.Mailgun.Enabled
	
	// If both are enabled, prefer the healthy one, with Gmail as default preference
	if preferGmail && preferMailgun {
		// Check Gmail first (default preference)
		if gmailProvider != nil && gmailProvider.IsHealthy() {
			return gmailProvider, nil
		}
		
		// Fallback to Mailgun if Gmail is unhealthy
		if mailgunProvider != nil && mailgunProvider.IsHealthy() {
			log.Printf("Using Mailgun provider as fallback for workspace %s", workspace.ID)
			return mailgunProvider, nil
		}
		
		// If both are unhealthy, prefer Gmail (it might recover)
		if gmailProvider != nil {
			log.Printf("Warning: Using unhealthy Gmail provider for workspace %s", workspace.ID)
			return gmailProvider, nil
		}
		
		if mailgunProvider != nil {
			log.Printf("Warning: Using unhealthy Mailgun provider for workspace %s", workspace.ID)
			return mailgunProvider, nil
		}
	}
	
	// Single provider preference
	if preferGmail && gmailProvider != nil {
		return gmailProvider, nil
	}
	
	if preferMailgun && mailgunProvider != nil {
		return mailgunProvider, nil
	}
	
	// Fallback to any available provider
	for _, provider := range providers {
		// Defensive check for nil provider
		if provider == nil {
			log.Printf("Warning: Skipping nil provider in fallback selection")
			continue
		}
		
		if provider.IsHealthy() {
			log.Printf("Using fallback provider %s for workspace %s", provider.GetID(), workspace.ID)
			return provider, nil
		}
	}
	
	// Last resort - use any provider even if unhealthy
	for _, provider := range providers {
		if provider != nil {
			log.Printf("Warning: All providers unhealthy, using provider %s for workspace %s", provider.GetID(), workspace.ID)
			return provider, nil
		}
	}
	
	log.Printf("Error: All providers are nil for workspace %s", workspace.ID)
	return nil, fmt.Errorf("all providers are nil")
}

// extractDomainFromEmail extracts the domain part from an email address
func (r *Router) extractDomainFromEmail(email string) (string, error) {
	if email == "" {
		return "", fmt.Errorf("email address is empty")
	}
	
	// Find the @ symbol
	atIndex := strings.LastIndex(email, "@")
	if atIndex == -1 || atIndex == len(email)-1 {
		return "", fmt.Errorf("invalid email format: %s", email)
	}
	
	domain := email[atIndex+1:]
	if domain == "" {
		return "", fmt.Errorf("domain part is empty in email: %s", email)
	}
	
	return domain, nil
}

// GetProvider returns a provider by ID
func (r *Router) GetProvider(providerID string) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	provider, exists := r.providers[providerID]
	if !exists {
		return nil, fmt.Errorf("provider not found: %s", providerID)
	}
	
	return provider, nil
}

// GetAllProviders returns all registered providers
func (r *Router) GetAllProviders() []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	providers := make([]Provider, 0, len(r.providers))
	for _, provider := range r.providers {
		providers = append(providers, provider)
	}
	
	return providers
}

// GetProvidersByDomain returns providers for a specific domain
func (r *Router) GetProvidersByDomain(domain string) ([]Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	providers, exists := r.providersByDomain[domain]
	if !exists {
		return nil, fmt.Errorf("no providers found for domain: %s", domain)
	}
	
	return providers, nil
}

// HealthCheckAll performs health checks on all providers
func (r *Router) HealthCheckAll(ctx context.Context) map[string]error {
	r.mu.RLock()
	providers := make([]Provider, 0, len(r.providers))
	for _, provider := range r.providers {
		providers = append(providers, provider)
	}
	r.mu.RUnlock()
	
	results := make(map[string]error)
	
	for _, provider := range providers {
		err := provider.HealthCheck(ctx)
		results[provider.GetID()] = err
		
		if err != nil {
			log.Printf("Health check failed for provider %s: %v", provider.GetID(), err)
		} else {
			log.Printf("Health check passed for provider %s", provider.GetID())
		}
	}
	
	return results
}

// GetStats returns statistics about the router and its providers
func (r *Router) GetStats() map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	healthyProviders := 0
	providersByType := make(map[string]int)
	providerInfo := make([]map[string]interface{}, 0, len(r.providers))
	
	for _, provider := range r.providers {
		if provider.IsHealthy() {
			healthyProviders++
		}
		
		providerType := string(provider.GetType())
		providersByType[providerType]++
		
		info := provider.GetProviderInfo()
		providerInfo = append(providerInfo, map[string]interface{}{
			"id":           info.ID,
			"type":         info.Type,
			"display_name": info.DisplayName,
			"domains":      info.Domains,
			"enabled":      info.Enabled,
			"healthy":      provider.IsHealthy(),
			"last_error":   info.LastError,
			"capabilities": info.Capabilities,
		})
	}
	
	return map[string]interface{}{
		"total_providers":       len(r.providers),
		"healthy_providers":     healthyProviders,
		"unhealthy_providers":   len(r.providers) - healthyProviders,
		"providers_by_type":     providersByType,
		"configured_domains":    len(r.providersByDomain),
		"provider_details":      providerInfo,
	}
}

// Shutdown gracefully shuts down all providers
func (r *Router) Shutdown(ctx context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	log.Printf("Shutting down %d providers", len(r.providers))
	
	// For now, we don't have specific shutdown logic for providers
	// In the future, we might add cleanup logic for cached connections, etc.
	
	// Clear provider maps
	r.providers = make(map[string]Provider)
	r.providersByDomain = make(map[string][]Provider)
	
	log.Println("Provider router shutdown complete")
}