package workspace

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"

	"relay/internal/config"
)

// Manager handles workspace configuration and management
type Manager struct {
	workspaces       map[string]*config.WorkspaceConfig
	domainToWorkspace map[string]string
	mu               sync.RWMutex
}

// NewManager creates a new workspace manager from a configuration file
func NewManager(configFile string) (*Manager, error) {
	manager := &Manager{
		workspaces:       make(map[string]*config.WorkspaceConfig),
		domainToWorkspace: make(map[string]string),
	}
	
	if err := manager.loadWorkspaces(configFile); err != nil {
		return nil, fmt.Errorf("failed to load workspaces: %w", err)
	}
	
	return manager, nil
}

// NewManagerFromJSON creates a new workspace manager from JSON data
func NewManagerFromJSON(jsonData []byte) (*Manager, error) {
	manager := &Manager{
		workspaces:       make(map[string]*config.WorkspaceConfig),
		domainToWorkspace: make(map[string]string),
	}
	
	var workspaces []config.WorkspaceConfig
	if err := json.Unmarshal(jsonData, &workspaces); err != nil {
		return nil, fmt.Errorf("failed to parse workspace config from JSON: %w", err)
	}
	
	// Process and store workspaces
	for _, ws := range workspaces {
		workspace := ws // Create a copy to avoid pointer issues
		manager.workspaces[workspace.ID] = &workspace
		
		// Map domain to workspace ID
		if workspace.Domain != "" {
			manager.domainToWorkspace[workspace.Domain] = workspace.ID
		}
		
		log.Printf("Loaded workspace: ID='%s', Domain='%s', Gmail=%v, Mailgun=%v",
			workspace.ID, workspace.Domain, 
			workspace.Gmail != nil, workspace.Mailgun != nil)
	}
	
	log.Printf("Successfully loaded %d workspaces", len(manager.workspaces))
	return manager, nil
}

// loadWorkspaces loads workspace configuration from file or environment
func (m *Manager) loadWorkspaces(configFile string) error {
	var workspaces []config.WorkspaceConfig
	
	// First try to load from environment variable (for production/container deployments)
	if envConfig := os.Getenv("WORKSPACE_CONFIG_JSON"); envConfig != "" {
		log.Println("Loading workspace configuration from environment variable")
		if err := json.Unmarshal([]byte(envConfig), &workspaces); err != nil {
			return fmt.Errorf("failed to parse workspace config from environment: %w", err)
		}
	} else if configFile != "" {
		// Fall back to file-based loading
		log.Printf("Loading workspace configuration from file: %s", configFile)
		data, err := os.ReadFile(configFile)
		if err != nil {
			return fmt.Errorf("failed to read workspace config file %s: %w", configFile, err)
		}
		
		if err := json.Unmarshal(data, &workspaces); err != nil {
			return fmt.Errorf("failed to parse workspace config file: %w", err)
		}
	} else {
		return fmt.Errorf("no workspace configuration provided")
	}
	
	if len(workspaces) == 0 {
		return fmt.Errorf("no workspaces found in configuration")
	}
	
	// Process and validate workspaces
	m.mu.Lock()
	defer m.mu.Unlock()
	
	for i := range workspaces {
		workspace := &workspaces[i]
		
		// Validate required fields
		if workspace.ID == "" {
			return fmt.Errorf("workspace %d missing required ID", i)
		}
		if workspace.Domain == "" {
			return fmt.Errorf("workspace %s missing required domain", workspace.ID)
		}
		
		// Ensure at least one provider is configured and enabled
		hasEnabledProvider := false
		if workspace.Gmail != nil && workspace.Gmail.Enabled {
			hasEnabledProvider = true
			
			// Validate Gmail configuration
			if workspace.Gmail.ServiceAccountFile == "" && workspace.Gmail.ServiceAccountEnv == "" {
				return fmt.Errorf("workspace %s has Gmail enabled but no service account file or env specified", workspace.ID)
			}
			
			// Check if service account file exists (only if using file-based config)
			if workspace.Gmail.ServiceAccountFile != "" {
				if _, err := os.Stat(workspace.Gmail.ServiceAccountFile); os.IsNotExist(err) {
					log.Printf("Warning: Service account file for workspace %s does not exist: %s", 
						workspace.ID, workspace.Gmail.ServiceAccountFile)
				}
			}
			
			// Check if environment variable is set (only if using env-based config)
			if workspace.Gmail.ServiceAccountEnv != "" {
				if os.Getenv(workspace.Gmail.ServiceAccountEnv) == "" {
					log.Printf("Warning: Service account env var %s for workspace %s is not set", 
						workspace.Gmail.ServiceAccountEnv, workspace.ID)
				}
			}
		}
		
		if workspace.Mailgun != nil && workspace.Mailgun.Enabled {
			hasEnabledProvider = true
			
			// Validate Mailgun configuration
			if workspace.Mailgun.APIKey == "" {
				return fmt.Errorf("workspace %s has Mailgun enabled but no API key specified", workspace.ID)
			}
			if workspace.Mailgun.Domain == "" {
				return fmt.Errorf("workspace %s has Mailgun enabled but no domain specified", workspace.ID)
			}
			
			// Set default base URL if not specified
			if workspace.Mailgun.BaseURL == "" {
				workspace.Mailgun.BaseURL = "https://api.mailgun.net/v3"
			}
		}
		
		if !hasEnabledProvider {
			return fmt.Errorf("workspace %s has no enabled providers (Gmail or Mailgun)", workspace.ID)
		}
		
		// Set default display name if not provided
		if workspace.DisplayName == "" {
			workspace.DisplayName = fmt.Sprintf("%s Workspace", workspace.Domain)
		}
		
		// Set default rate limits if not specified
		if workspace.RateLimits.WorkspaceDaily == 0 {
			workspace.RateLimits.WorkspaceDaily = 2000 // Default daily limit
		}
		if workspace.RateLimits.PerUserDaily == 0 {
			workspace.RateLimits.PerUserDaily = 200 // Default per-user limit
		}
		
		// Store workspace
		m.workspaces[workspace.ID] = workspace
		m.domainToWorkspace[workspace.Domain] = workspace.ID
		
		log.Printf("Loaded workspace: ID='%s', Domain='%s', Gmail=%t, Mailgun=%t", 
			workspace.ID, workspace.Domain, 
			workspace.Gmail != nil && workspace.Gmail.Enabled,
			workspace.Mailgun != nil && workspace.Mailgun.Enabled)
	}
	
	log.Printf("Successfully loaded %d workspaces", len(m.workspaces))
	return nil
}

// GetWorkspaceByDomain returns a workspace for the given domain
func (m *Manager) GetWorkspaceByDomain(domain string) (*config.WorkspaceConfig, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	workspaceID, exists := m.domainToWorkspace[domain]
	if !exists {
		return nil, fmt.Errorf("no workspace found for domain: %s", domain)
	}
	
	workspace, exists := m.workspaces[workspaceID]
	if !exists {
		return nil, fmt.Errorf("workspace data not found for ID: %s", workspaceID)
	}
	
	return workspace, nil
}

// GetWorkspaceByID returns a workspace for the given ID
func (m *Manager) GetWorkspaceByID(workspaceID string) (*config.WorkspaceConfig, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	workspace, exists := m.workspaces[workspaceID]
	if !exists {
		return nil, fmt.Errorf("workspace not found: %s", workspaceID)
	}
	
	return workspace, nil
}

// GetAllWorkspaces returns all configured workspaces
func (m *Manager) GetAllWorkspaces() map[string]*config.WorkspaceConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	// Return a copy to prevent external modification
	result := make(map[string]*config.WorkspaceConfig)
	for k, v := range m.workspaces {
		result[k] = v
	}
	
	return result
}

// GetWorkspaceIDs returns all workspace IDs
func (m *Manager) GetWorkspaceIDs() []string {
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
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	domains := make([]string, 0, len(m.domainToWorkspace))
	for domain := range m.domainToWorkspace {
		domains = append(domains, domain)
	}
	
	return domains
}

// GetWorkspaceForSender determines which workspace should handle a message from the given sender
func (m *Manager) GetWorkspaceForSender(senderEmail string) (*config.WorkspaceConfig, error) {
	if senderEmail == "" {
		return nil, fmt.Errorf("sender email cannot be empty")
	}
	
	// Extract domain from sender email
	atIndex := len(senderEmail) - 1
	for i := len(senderEmail) - 1; i >= 0; i-- {
		if senderEmail[i] == '@' {
			atIndex = i
			break
		}
	}
	
	if atIndex == len(senderEmail) - 1 || atIndex == 0 {
		return nil, fmt.Errorf("invalid sender email format: %s", senderEmail)
	}
	
	domain := senderEmail[atIndex+1:]
	return m.GetWorkspaceByDomain(domain)
}

// ValidateConfiguration checks if all workspaces are properly configured
func (m *Manager) ValidateConfiguration() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if len(m.workspaces) == 0 {
		return fmt.Errorf("no workspaces configured")
	}
	
	for id, workspace := range m.workspaces {
		// Check Gmail configuration
		if workspace.Gmail != nil && workspace.Gmail.Enabled {
			// Check file-based config
			if workspace.Gmail.ServiceAccountFile != "" {
				if _, err := os.Stat(workspace.Gmail.ServiceAccountFile); os.IsNotExist(err) {
					return fmt.Errorf("workspace %s Gmail service account file does not exist: %s", 
						id, workspace.Gmail.ServiceAccountFile)
				}
			}
			// Check env-based config
			if workspace.Gmail.ServiceAccountEnv != "" {
				if os.Getenv(workspace.Gmail.ServiceAccountEnv) == "" {
					return fmt.Errorf("workspace %s Gmail service account env var %s is not set", 
						id, workspace.Gmail.ServiceAccountEnv)
				}
			}
			// Ensure at least one method is configured
			if workspace.Gmail.ServiceAccountFile == "" && workspace.Gmail.ServiceAccountEnv == "" {
				return fmt.Errorf("workspace %s has Gmail enabled but no service account configuration", id)
			}
		}
		
		// Check Mailgun configuration
		if workspace.Mailgun != nil && workspace.Mailgun.Enabled {
			if workspace.Mailgun.APIKey == "" {
				return fmt.Errorf("workspace %s Mailgun API key is empty", id)
			}
			if workspace.Mailgun.Domain == "" {
				return fmt.Errorf("workspace %s Mailgun domain is empty", id)
			}
		}
		
		// Ensure at least one provider is enabled
		hasEnabledProvider := (workspace.Gmail != nil && workspace.Gmail.Enabled) ||
							 (workspace.Mailgun != nil && workspace.Mailgun.Enabled)
		if !hasEnabledProvider {
			return fmt.Errorf("workspace %s has no enabled providers", id)
		}
	}
	
	return nil
}

// GetStats returns statistics about the workspace configuration
func (m *Manager) GetStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	gmailWorkspaces := 0
	mailgunWorkspaces := 0
	bothProviders := 0
	
	for _, workspace := range m.workspaces {
		hasGmail := workspace.Gmail != nil && workspace.Gmail.Enabled
		hasMailgun := workspace.Mailgun != nil && workspace.Mailgun.Enabled
		
		if hasGmail {
			gmailWorkspaces++
		}
		if hasMailgun {
			mailgunWorkspaces++
		}
		if hasGmail && hasMailgun {
			bothProviders++
		}
	}
	
	return map[string]interface{}{
		"total_workspaces":    len(m.workspaces),
		"gmail_workspaces":    gmailWorkspaces,
		"mailgun_workspaces":  mailgunWorkspaces,
		"dual_provider_workspaces": bothProviders,
		"total_domains":       len(m.domainToWorkspace),
	}
}