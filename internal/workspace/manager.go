package workspace

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"relay/internal/config"
)

// Manager manages workspace configurations and routing
type Manager struct {
	workspaces        map[string]*config.WorkspaceConfig
	domainToWorkspace map[string]string // Maps domain to workspace ID
	loadBalancer      LoadBalancer // Optional load balancer for advanced routing
	mu                sync.RWMutex
}

// JSON loading functions removed - using database only

// GetWorkspaceByDomain returns a workspace for the given domain
func (m *Manager) GetWorkspaceByDomain(domain string) (*config.WorkspaceConfig, error) {
	// Defensive programming: validate manager and input
	if m == nil {
		return nil, fmt.Errorf("workspace manager is nil")
	}
	if domain == "" {
		return nil, fmt.Errorf("domain cannot be empty")
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	// Try direct domain match first
	if workspaceID, exists := m.domainToWorkspace[domain]; exists {
		workspace := m.workspaces[workspaceID]
		if workspace == nil {
			// Defensive: workspace ID exists in map but workspace is nil
			log.Printf("Warning: workspace ID %s exists in domain map but workspace is nil", workspaceID)
			return nil, fmt.Errorf("workspace configuration corrupted for domain %s", domain)
		}
		return workspace, nil
	}

	// Try pattern matching for wildcard domains
	for configuredDomain, workspaceID := range m.domainToWorkspace {
		if strings.HasPrefix(configuredDomain, "*.") {
			// Wildcard domain like *.example.com
			baseDomain := configuredDomain[2:] // Remove *.
			if strings.HasSuffix(domain, baseDomain) {
				workspace := m.workspaces[workspaceID]
				if workspace == nil {
					// Defensive: workspace ID exists but workspace is nil
					log.Printf("Warning: workspace ID %s exists for wildcard but workspace is nil", workspaceID)
					continue
				}
				return workspace, nil
			}
		}
	}

	return nil, fmt.Errorf("no workspace found for domain: %s", domain)
}

// GetWorkspaceByID returns a workspace by its ID
func (m *Manager) GetWorkspaceByID(workspaceID string) (*config.WorkspaceConfig, error) {
	// Defensive programming: validate manager and input
	if m == nil {
		return nil, fmt.Errorf("workspace manager is nil")
	}
	if workspaceID == "" {
		return nil, fmt.Errorf("workspace ID cannot be empty")
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	workspace, exists := m.workspaces[workspaceID]
	if !exists {
		return nil, fmt.Errorf("workspace not found: %s", workspaceID)
	}
	
	// Defensive: check if workspace is nil even though it exists in map
	if workspace == nil {
		log.Printf("Warning: workspace %s exists in map but is nil", workspaceID)
		return nil, fmt.Errorf("workspace configuration is nil for ID: %s", workspaceID)
	}

	return workspace, nil
}

// GetAllWorkspaces returns all configured workspaces
func (m *Manager) GetAllWorkspaces() map[string]*config.WorkspaceConfig {
	// Defensive programming: handle nil manager
	if m == nil {
		log.Printf("Warning: GetAllWorkspaces called on nil manager")
		return make(map[string]*config.WorkspaceConfig)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	// Create a copy to avoid concurrent modification
	result := make(map[string]*config.WorkspaceConfig)
	for id, workspace := range m.workspaces {
		// Defensive: skip nil workspaces
		if workspace == nil {
			log.Printf("Warning: workspace %s is nil in manager", id)
			continue
		}
		result[id] = workspace
	}

	return result
}

// GetWorkspaceIDs returns all workspace IDs
func (m *Manager) GetWorkspaceIDs() []string {
	// Defensive programming: handle nil manager
	if m == nil {
		log.Printf("Warning: GetWorkspaceIDs called on nil manager")
		return []string{}
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]string, 0, len(m.workspaces))
	for id := range m.workspaces {
		ids = append(ids, id)
	}
	return ids
}

// GetDomains returns all configured domains
func (m *Manager) GetDomains() []string {
	// Defensive programming: handle nil manager
	if m == nil {
		log.Printf("Warning: GetDomains called on nil manager")
		return []string{}
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	domains := make([]string, 0, len(m.domainToWorkspace))
	for domain := range m.domainToWorkspace {
		domains = append(domains, domain)
	}
	return domains
}

// WorkspaceSelectionResult contains the result of workspace selection including rewrite information
type WorkspaceSelectionResult struct {
	Workspace          *config.WorkspaceConfig
	NeedsDomainRewrite bool
	OriginalDomain     string
	RewrittenDomain    string
}

// GetWorkspaceForSenderWithRewrite determines the workspace for a sender and whether domain rewriting is needed
func (m *Manager) GetWorkspaceForSenderWithRewrite(senderEmail string) (*WorkspaceSelectionResult, error) {
	// Defensive programming: validate manager and input
	if m == nil {
		return nil, fmt.Errorf("workspace manager is nil")
	}
	if senderEmail == "" {
		return nil, fmt.Errorf("sender email cannot be empty")
	}

	// Extract domain from sender email
	atIndex := strings.LastIndex(senderEmail, "@")
	if atIndex < 0 {
		return nil, fmt.Errorf("invalid sender email format: %s", senderEmail)
	}
	domain := senderEmail[atIndex+1:]

	// First, try direct domain matching
	workspace, err := m.GetWorkspaceByDomain(domain)
	if err == nil && workspace != nil {
		log.Printf("Direct domain match found: workspace %s for domain %s", workspace.ID, domain)
		return &WorkspaceSelectionResult{
			Workspace:          workspace,
			NeedsDomainRewrite: false,
			OriginalDomain:     domain,
			RewrittenDomain:    domain,
		}, nil
	}

	// If we have a load balancer, try pool-based selection
	if m.loadBalancer != nil {
		// Try load balancer selection for specific pools
		log.Printf("No direct domain match for %s, checking load balancing pools", domain)
		workspace, err := m.loadBalancer.SelectWorkspace(context.Background(), senderEmail)
		if err == nil && workspace != nil {
			log.Printf("Load balancer selected workspace %s for sender %s", workspace.ID, senderEmail)
			// For load-balanced selection from pools, domain stays the same
			return &WorkspaceSelectionResult{
				Workspace:          workspace,
				NeedsDomainRewrite: false,
				OriginalDomain:     domain,
				RewrittenDomain:    domain,
			}, nil
		}

		// No specific pool match, try default pool
		log.Printf("No specific pool match for %s: %v", senderEmail, err)
		workspace, err = m.loadBalancer.SelectFromDefaultPool(context.Background())
		if err == nil && workspace != nil {
			// Get the primary domain from the selected workspace
			primaryDomain := workspace.GetPrimaryDomain()
			if primaryDomain == "" && len(workspace.Domains) > 0 {
				primaryDomain = workspace.Domains[0]
			}
			
			log.Printf("Using default pool: selected workspace %s for sender %s (domain will be rewritten)", workspace.ID, senderEmail)
			return &WorkspaceSelectionResult{
				Workspace:          workspace,
				NeedsDomainRewrite: true,
				OriginalDomain:     domain,
				RewrittenDomain:    primaryDomain,
			}, nil
		}
	}

	return nil, fmt.Errorf("no workspace found for sender: %s", senderEmail)
}

// domainMatchesWorkspace checks if a domain matches any of the workspace's configured domains
func (m *Manager) domainMatchesWorkspace(domain string, workspace *config.WorkspaceConfig) bool {
	// Defensive programming: validate inputs
	if workspace == nil {
		log.Printf("Warning: domainMatchesWorkspace called with nil workspace")
		return false
	}
	if domain == "" {
		return false
	}

	for _, wsDomain := range workspace.Domains {
		if wsDomain == domain {
			return true
		}
		// Check wildcard domains
		if strings.HasPrefix(wsDomain, "*.") {
			baseDomain := wsDomain[2:]
			if strings.HasSuffix(domain, baseDomain) {
				return true
			}
		}
	}
	return false
}

// GetWorkspaceForSender determines the appropriate workspace for a given sender email
func (m *Manager) GetWorkspaceForSender(senderEmail string) (*config.WorkspaceConfig, error) {
	// Defensive programming: validate manager and input
	if m == nil {
		return nil, fmt.Errorf("workspace manager is nil")
	}
	if senderEmail == "" {
		return nil, fmt.Errorf("sender email cannot be empty")
	}

	// Extract domain from sender email
	atIndex := strings.LastIndex(senderEmail, "@")
	if atIndex < 0 {
		return nil, fmt.Errorf("invalid sender email format: %s", senderEmail)
	}
	domain := senderEmail[atIndex+1:]

	// First, try direct domain matching
	workspace, err := m.GetWorkspaceByDomain(domain)
	if err == nil && workspace != nil {
		return workspace, nil
	}

	// If we have a load balancer, use it for advanced routing
	if m.loadBalancer != nil {
		log.Printf("No direct domain match for %s, using load balancer", domain)
		workspace, err := m.loadBalancer.SelectWorkspace(context.Background(), senderEmail)
		// Defensive: check both error and workspace to prevent nil pointer dereference
		if err == nil && workspace != nil {
			log.Printf("Load balancer selected workspace %s for sender %s", workspace.ID, senderEmail)
			return workspace, nil
		}
		if err != nil {
			log.Printf("Load balancer selection failed for %s: %v", senderEmail, err)
		} else {
			log.Printf("Load balancer returned nil workspace for %s", senderEmail)
		}
	}

	return nil, fmt.Errorf("no workspace found for sender: %s", senderEmail)
}

// SetLoadBalancer sets the load balancer for advanced routing
func (m *Manager) SetLoadBalancer(lb LoadBalancer) {
	// Defensive programming: validate manager
	if m == nil {
		log.Printf("Warning: SetLoadBalancer called on nil manager")
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.loadBalancer = lb
	if lb != nil {
		log.Println("Load balancer integrated with workspace manager")
	} else {
		log.Println("Load balancer removed from workspace manager")
	}
}

// GetLoadBalancer returns the current load balancer
func (m *Manager) GetLoadBalancer() LoadBalancer {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.loadBalancer
}

// HasLoadBalancer checks if a load balancer is configured
func (m *Manager) HasLoadBalancer() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.loadBalancer != nil
}

// GetWorkspaceForSenderWithContext is like GetWorkspaceForSender but accepts a context
func (m *Manager) GetWorkspaceForSenderWithContext(ctx context.Context, senderEmail string) (*config.WorkspaceConfig, error) {
	// Defensive programming: validate manager and input
	if m == nil {
		return nil, fmt.Errorf("workspace manager is nil")
	}
	if senderEmail == "" {
		return nil, fmt.Errorf("sender email cannot be empty")
	}

	// Extract domain from sender email
	atIndex := strings.LastIndex(senderEmail, "@")
	if atIndex < 0 {
		return nil, fmt.Errorf("invalid sender email format: %s", senderEmail)
	}
	domain := senderEmail[atIndex+1:]

	// First, try direct domain matching
	workspace, err := m.GetWorkspaceByDomain(domain)
	if err == nil && workspace != nil {
		return workspace, nil
	}

	// If we have a load balancer, use it for advanced routing
	if m.loadBalancer != nil {
		workspace, err := m.loadBalancer.SelectWorkspace(ctx, senderEmail)
		// Defensive: check both error and workspace to prevent nil pointer dereference
		if err == nil && workspace != nil {
			return workspace, nil
		}
		if err != nil {
			log.Printf("Load balancer selection failed for %s: %v", senderEmail, err)
		} else {
			log.Printf("Load balancer returned nil workspace for %s", senderEmail)
		}
	}

	return nil, fmt.Errorf("no workspace found for sender: %s", senderEmail)
}

// ValidateConfiguration validates all workspace configurations
func (m *Manager) ValidateConfiguration() error {
	// Defensive programming: validate manager
	if m == nil {
		return fmt.Errorf("workspace manager is nil")
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.workspaces) == 0 {
		return fmt.Errorf("no workspaces configured")
	}

	// Check for duplicate domains
	domainCount := make(map[string][]string)
	for domain, workspaceID := range m.domainToWorkspace {
		domainCount[domain] = append(domainCount[domain], workspaceID)
	}

	for domain, workspaceIDs := range domainCount {
		if len(workspaceIDs) > 1 {
			return fmt.Errorf("domain %s is configured in multiple workspaces: %v", domain, workspaceIDs)
		}
	}

	// Validate each workspace
	for id, workspace := range m.workspaces {
		// Defensive: check for nil workspace
		if workspace == nil {
			return fmt.Errorf("workspace %s is nil", id)
		}

		if len(workspace.Domains) == 0 {
			return fmt.Errorf("workspace %s has no domains configured", id)
		}

		// Check if at least one provider is enabled
		hasEnabledProvider := false
		if workspace.Gmail != nil && workspace.Gmail.Enabled {
			hasEnabledProvider = true
		}
		if workspace.Mailgun != nil && workspace.Mailgun.Enabled {
			hasEnabledProvider = true
		}
		if workspace.Mandrill != nil && workspace.Mandrill.Enabled {
			hasEnabledProvider = true
		}

		if !hasEnabledProvider {
			return fmt.Errorf("workspace %s has no enabled providers", id)
		}
	}

	return nil
}

// GetStats returns statistics about the workspace manager
func (m *Manager) GetStats() map[string]interface{} {
	// Defensive programming: handle nil manager
	if m == nil {
		log.Printf("Warning: GetStats called on nil manager")
		return map[string]interface{}{
			"error": "manager is nil",
		}
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := map[string]interface{}{
		"total_workspaces": len(m.workspaces),
		"total_domains":    len(m.domainToWorkspace),
		"has_load_balancer": m.loadBalancer != nil,
	}

	// Count provider types
	gmailCount := 0
	mailgunCount := 0
	mandrillCount := 0

	for _, workspace := range m.workspaces {
		// Defensive: skip nil workspaces
		if workspace == nil {
			continue
		}

		if workspace.Gmail != nil && workspace.Gmail.Enabled {
			gmailCount++
		}
		if workspace.Mailgun != nil && workspace.Mailgun.Enabled {
			mailgunCount++
		}
		if workspace.Mandrill != nil && workspace.Mandrill.Enabled {
			mandrillCount++
		}
	}

	stats["gmail_providers"] = gmailCount
	stats["mailgun_providers"] = mailgunCount
	stats["mandrill_providers"] = mandrillCount

	return stats
}