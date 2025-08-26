package loadbalancer

import (
	"context"
	"time"

	"relay/internal/config"
)

// SelectionStrategy defines the algorithm used to select workspaces from a pool
type SelectionStrategy string

const (
	StrategyCapacityWeighted SelectionStrategy = "capacity_weighted"
	StrategyRoundRobin      SelectionStrategy = "round_robin"
	StrategyLeastUsed       SelectionStrategy = "least_used"
	StrategyRandomWeighted  SelectionStrategy = "random_weighted"
)

// LoadBalancer defines the main interface for load balancing across workspaces
type LoadBalancer interface {
	// SelectWorkspace selects the optimal workspace from available pools for the given sender
	SelectWorkspace(ctx context.Context, senderEmail string) (*config.WorkspaceConfig, error)
	
	// RecordSelection records the outcome of a selection decision for analytics
	RecordSelection(ctx context.Context, poolID, workspaceID, senderEmail string, success bool, capacityScore float64) error
	
	// GetPoolStatus returns the current status and health of a pool
	GetPoolStatus(poolID string) (*PoolStatus, error)
	
	// RefreshPools reloads pool configuration from the database
	RefreshPools(ctx context.Context) error
	
	// Shutdown gracefully shuts down the load balancer
	Shutdown(ctx context.Context) error
}

// LoadBalancingPool represents a pool of workspaces for load balancing
type LoadBalancingPool struct {
	ID             string           `json:"id" db:"id"`
	Name           string           `json:"name" db:"name"`
	DomainPatterns []string         `json:"domain_patterns" db:"domain_patterns"` // JSON in DB
	Strategy       SelectionStrategy `json:"strategy" db:"strategy"`
	Enabled        bool             `json:"enabled" db:"enabled"`
	Workspaces     []PoolWorkspace  `json:"workspaces"`
	CreatedAt      time.Time        `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at" db:"updated_at"`
}

// PoolWorkspace represents a workspace within a load balancing pool
type PoolWorkspace struct {
	ProviderID         string  `json:"provider_id" db:"provider_id"`
	Weight              float64 `json:"weight" db:"weight"`
	Enabled             bool    `json:"enabled" db:"enabled"`
	MinCapacityThreshold float64 `json:"min_capacity_threshold,omitempty"` // Minimum capacity % to be eligible
}

// PoolStatus represents the current status and health information of a pool
type PoolStatus struct {
	PoolID              string              `json:"pool_id"`
	Name                string              `json:"name"`
	Strategy            SelectionStrategy   `json:"strategy"`
	Enabled             bool                `json:"enabled"`
	TotalWorkspaces     int                 `json:"total_workspaces"`
	HealthyWorkspaces   int                 `json:"healthy_workspaces"`
	LastSelectionTime   *time.Time          `json:"last_selection_time,omitempty"`
	WorkspaceStatuses   []WorkspaceStatus   `json:"workspace_statuses"`
	DomainPatterns      []string            `json:"domain_patterns"`
}

// WorkspaceStatus represents the status of a workspace within a pool
type WorkspaceStatus struct {
	ProviderID     string    `json:"provider_id"`
	Weight          float64   `json:"weight"`
	Enabled         bool      `json:"enabled"`
	Healthy         bool      `json:"healthy"`
	CapacityInfo    *CapacityInfo `json:"capacity_info,omitempty"`
	LastHealthCheck *time.Time    `json:"last_health_check,omitempty"`
	LastError       *string       `json:"last_error,omitempty"`
}

// CapacityInfo represents the current capacity status of a workspace
type CapacityInfo struct {
	WorkspaceRemaining    int           `json:"workspace_remaining"`
	UserRemaining         int           `json:"user_remaining"`
	WorkspaceLimit        int           `json:"workspace_limit"`
	UserLimit             int           `json:"user_limit"`
	RemainingPercentage   float64       `json:"remaining_percentage"`
	TimeToReset           time.Duration `json:"time_to_reset"`
	EffectiveRemaining    int           `json:"effective_remaining"`
	EffectiveLimit        int           `json:"effective_limit"`
}

// WorkspaceCandidate represents a workspace being considered for selection
type WorkspaceCandidate struct {
	Workspace      PoolWorkspace      `json:"workspace"`
	Config         *config.WorkspaceConfig `json:"config"`
	Score          float64            `json:"score"`
	Capacity       *CapacityInfo      `json:"capacity"`
	HealthScore    float64            `json:"health_score"`
	SelectionReason string            `json:"selection_reason"`
}

// LoadBalancingSelection represents a recorded selection decision
type LoadBalancingSelection struct {
	ID            int64     `json:"id" db:"id"`
	PoolID        string    `json:"pool_id" db:"pool_id"`
	ProviderID   string    `json:"provider_id" db:"provider_id"`
	SenderEmail   string    `json:"sender_email" db:"sender_email"`
	SelectedAt    time.Time `json:"selected_at" db:"selected_at"`
	Success       bool      `json:"success" db:"success"`
	CapacityScore float64   `json:"capacity_score" db:"capacity_score"`
	SelectionReason string  `json:"selection_reason,omitempty"`
}

// CapacityProvider defines the interface for getting workspace capacity information
type CapacityProvider interface {
	// GetWorkspaceCapacity returns capacity information for a workspace and specific sender
	GetWorkspaceCapacity(workspaceID, senderEmail string) (*CapacityInfo, error)
	
	// GetWorkspaceStatus returns workspace-level capacity information
	GetWorkspaceStatus(workspaceID string) (sent int, remaining int, resetTime time.Time, err error)
	
	// GetUserStatus returns user-level capacity information within a workspace
	GetUserStatus(workspaceID, senderEmail string) (sent int, remaining int, resetTime time.Time, err error)
}

// WorkspaceProvider defines the interface for getting workspace configurations
type WorkspaceProvider interface {
	// GetWorkspaceByID returns a workspace configuration by ID
	GetWorkspaceByID(workspaceID string) (*config.WorkspaceConfig, error)
	
	// GetAllWorkspaces returns all configured workspaces
	GetAllWorkspaces() map[string]*config.WorkspaceConfig
}

// HealthChecker defines the interface for checking workspace health
type HealthChecker interface {
	// IsWorkspaceHealthy checks if a workspace is healthy and ready to receive emails
	IsWorkspaceHealthy(ctx context.Context, workspaceID string) (bool, error)
	
	// GetWorkspaceHealth returns detailed health information for a workspace
	GetWorkspaceHealth(ctx context.Context, workspaceID string) (*WorkspaceHealthInfo, error)
}

// WorkspaceHealthInfo represents detailed health information for a workspace
type WorkspaceHealthInfo struct {
	ProviderID     string     `json:"provider_id"`
	Healthy         bool       `json:"healthy"`
	LastCheckTime   time.Time  `json:"last_check_time"`
	LastError       *string    `json:"last_error,omitempty"`
	ResponseTime    *time.Duration `json:"response_time,omitempty"`
	ProviderStatus  string     `json:"provider_status"`
}

// Selector defines the interface for workspace selection algorithms
type Selector interface {
	// Select chooses a workspace from the given candidates
	Select(ctx context.Context, candidates []WorkspaceCandidate, senderEmail string) (*WorkspaceCandidate, error)
	
	// GetStrategy returns the strategy type this selector implements
	GetStrategy() SelectionStrategy
}

// LoadBalancerConfig represents configuration for the load balancer
type LoadBalancerConfig struct {
	// EnableCaching enables caching of pool configurations and capacity data
	EnableCaching bool `json:"enable_caching"`
	
	// CacheTTL is the time-to-live for cached data
	CacheTTL time.Duration `json:"cache_ttl"`
	
	// MaxCandidates limits the number of workspace candidates considered per selection
	MaxCandidates int `json:"max_candidates"`
	
	// HealthCheckInterval is how often workspace health is checked
	HealthCheckInterval time.Duration `json:"health_check_interval"`
	
	// SelectionTimeout is the maximum time allowed for a selection decision
	SelectionTimeout time.Duration `json:"selection_timeout"`
	
	// MetricsEnabled enables collection of selection metrics
	MetricsEnabled bool `json:"metrics_enabled"`
}

// DefaultLoadBalancerConfig returns default configuration values
func DefaultLoadBalancerConfig() *LoadBalancerConfig {
	return &LoadBalancerConfig{
		EnableCaching:       true,
		CacheTTL:           60 * time.Second,
		MaxCandidates:      50,
		HealthCheckInterval: 30 * time.Second,
		SelectionTimeout:   100 * time.Millisecond,
		MetricsEnabled:     true,
	}
}

// LoadBalancerError represents errors specific to load balancing operations
type LoadBalancerError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	PoolID  string `json:"pool_id,omitempty"`
	ProviderID string `json:"provider_id,omitempty"`
	Cause   error  `json:"-"`
}

func (e *LoadBalancerError) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

func (e *LoadBalancerError) Unwrap() error {
	return e.Cause
}

// Common error types
const (
	ErrorTypePoolNotFound       = "pool_not_found"
	ErrorTypeWorkspaceNotFound  = "workspace_not_found"
	ErrorTypeNoHealthyWorkspace = "no_healthy_workspace"
	ErrorTypeSelectionTimeout  = "selection_timeout"
	ErrorTypeInvalidConfig     = "invalid_config"
	ErrorTypeCapacityError     = "capacity_error"
)

// NewLoadBalancerError creates a new load balancer error
func NewLoadBalancerError(errType, message string, cause error) *LoadBalancerError {
	return &LoadBalancerError{
		Type:    errType,
		Message: message,
		Cause:   cause,
	}
}