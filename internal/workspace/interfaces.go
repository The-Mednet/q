package workspace

import (
	"context"
	"relay/internal/config"
)

// LoadBalancer interface for workspace selection - matches the subset of methods from loadbalancer package
type LoadBalancer interface {
	// SelectWorkspace selects workspace based on sender domain patterns
	SelectWorkspace(ctx context.Context, senderEmail string) (*config.WorkspaceConfig, error)
	
	// SelectFromDefaultPool selects from the default pool when no domain match
	SelectFromDefaultPool(ctx context.Context) (*config.WorkspaceConfig, error)
}