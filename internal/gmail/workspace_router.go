package gmail

import (
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"smtp_relay/internal/config"
)

// WorkspaceRouter determines which workspace to use for sending emails
type WorkspaceRouter struct {
	workspaces    map[string]*config.WorkspaceConfig
	legacyDomains []string
	mu            sync.RWMutex
	rand          *rand.Rand
}

func NewWorkspaceRouter(cfg *config.GmailConfig) *WorkspaceRouter {
	workspaces := make(map[string]*config.WorkspaceConfig)
	for i := range cfg.Workspaces {
		ws := &cfg.Workspaces[i]
		workspaces[ws.Domain] = ws
	}

	return &WorkspaceRouter{
		workspaces:    workspaces,
		legacyDomains: cfg.LegacyDomains,
		rand:          rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// RouteMessage determines which workspace should handle this message
func (wr *WorkspaceRouter) RouteMessage(fromEmail string) (*config.WorkspaceConfig, error) {
	if fromEmail == "" {
		return nil, fmt.Errorf("sender email is required")
	}

	// Extract domain from email
	parts := strings.Split(fromEmail, "@")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid email format: %s", fromEmail)
	}
	domain := parts[1]

	wr.mu.RLock()
	defer wr.mu.RUnlock()

	// Check for direct workspace match
	if workspace, exists := wr.workspaces[domain]; exists {
		return workspace, nil
	}

	// Check if this is a legacy domain that needs random routing
	for _, legacyDomain := range wr.legacyDomains {
		if domain == legacyDomain {
			return wr.selectRandomWorkspace()
		}
	}

	return nil, fmt.Errorf("no workspace configured for domain: %s", domain)
}

func (wr *WorkspaceRouter) selectRandomWorkspace() (*config.WorkspaceConfig, error) {
	if len(wr.workspaces) == 0 {
		return nil, fmt.Errorf("no workspaces configured")
	}

	// Convert map to slice for random selection
	workspaces := make([]*config.WorkspaceConfig, 0, len(wr.workspaces))
	for _, ws := range wr.workspaces {
		workspaces = append(workspaces, ws)
	}

	// Select random workspace
	idx := wr.rand.Intn(len(workspaces))
	return workspaces[idx], nil
}

// GetAllWorkspaces returns all configured workspaces
func (wr *WorkspaceRouter) GetAllWorkspaces() map[string]*config.WorkspaceConfig {
	wr.mu.RLock()
	defer wr.mu.RUnlock()

	result := make(map[string]*config.WorkspaceConfig)
	for k, v := range wr.workspaces {
		result[k] = v
	}
	return result
}

// GetLegacyDomains returns domains that get randomly routed
func (wr *WorkspaceRouter) GetLegacyDomains() []string {
	wr.mu.RLock()
	defer wr.mu.RUnlock()

	result := make([]string, len(wr.legacyDomains))
	copy(result, wr.legacyDomains)
	return result
}
