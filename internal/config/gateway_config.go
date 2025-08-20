package config

import (
	"encoding/json"
	"fmt"
	"os"
	"relay/internal/gateway"
	"time"
)

// GatewayConfig represents the enhanced configuration that supports multiple gateway types
type GatewayConfig struct {
	// Core identification
	ID          string              `json:"id"`
	Type        gateway.GatewayType `json:"type"`
	DisplayName string              `json:"display_name"`
	Domain      string              `json:"domain"`
	Enabled     bool                `json:"enabled"`

	// Routing and priority
	Priority int `json:"priority"` // Lower number = higher priority
	Weight   int `json:"weight"`   // For weighted load balancing

	// Provider-specific configurations
	GoogleWorkspace *GoogleWorkspaceConfig `json:"google_workspace,omitempty"`
	Mailgun         *MailgunConfig         `json:"mailgun,omitempty"`
	SendGrid        *SendGridConfig        `json:"sendgrid,omitempty"`
	AmazonSES       *AmazonSESConfig       `json:"amazon_ses,omitempty"`

	// Rate limiting configuration
	RateLimits GatewayRateLimitConfig `json:"rate_limits"`

	// Routing configuration
	Routing GatewayRoutingConfig `json:"routing"`

	// Circuit breaker configuration
	CircuitBreaker CircuitBreakerConfig `json:"circuit_breaker"`

	// Health check configuration
	HealthCheck *HealthCheckConfig `json:"health_check,omitempty"`
}

// GoogleWorkspaceConfig contains Google Workspace specific configuration
type GoogleWorkspaceConfig struct {
	ServiceAccountFile   string   `json:"service_account_file"`
	ImpersonationEnabled bool     `json:"impersonation_enabled"`
	Scopes               []string `json:"scopes"`
	SubjectEmail         *string  `json:"subject_email,omitempty"` // For non-impersonation mode
}

// MailgunConfig contains Mailgun specific configuration
type MailgunConfig struct {
	APIKey   string            `json:"api_key"`
	Domain   string            `json:"domain"`
	BaseURL  string            `json:"base_url"`
	Region   string            `json:"region"` // us, eu
	Tracking MailgunTracking   `json:"tracking"`
	Tags     MailgunTagsConfig `json:"tags"`
}

// MailgunTracking configures tracking features
type MailgunTracking struct {
	Clicks      bool `json:"clicks"`
	Opens       bool `json:"opens"`
	Unsubscribe bool `json:"unsubscribe"`
}

// MailgunTagsConfig configures tagging behavior
type MailgunTagsConfig struct {
	Default            []string `json:"default"`
	CampaignTagEnabled bool     `json:"campaign_tag_enabled"`
	UserTagEnabled     bool     `json:"user_tag_enabled"`
}

// SendGridConfig contains SendGrid specific configuration (for future implementation)
type SendGridConfig struct {
	APIKey    string `json:"api_key"`
	FromEmail string `json:"from_email,omitempty"`
	FromName  string `json:"from_name,omitempty"`
	Tracking  bool   `json:"tracking"`
}

// AmazonSESConfig contains Amazon SES specific configuration (for future implementation)
type AmazonSESConfig struct {
	AccessKeyID      string `json:"access_key_id"`
	SecretAccessKey  string `json:"secret_access_key"`
	Region           string `json:"region"`
	ConfigurationSet string `json:"configuration_set,omitempty"`
}

// GatewayRateLimitConfig extends the existing rate limit configuration
type GatewayRateLimitConfig struct {
	// Daily limits
	WorkspaceDaily int `json:"workspace_daily,omitempty"` // Total daily limit for this gateway
	PerUserDaily   int `json:"per_user_daily,omitempty"`  // Per user daily limit

	// Hourly limits (new)
	PerHour    int `json:"per_hour,omitempty"`    // Per user hourly limit
	BurstLimit int `json:"burst_limit,omitempty"` // Burst capacity

	// Custom limits
	CustomUserLimits map[string]int `json:"custom_user_limits,omitempty"`

	// Override behavior
	InheritGlobalLimits bool `json:"inherit_global_limits,omitempty"` // Whether to inherit from global config
}

// GatewayRoutingConfig defines routing rules for a gateway
type GatewayRoutingConfig struct {
	CanRoute        []string `json:"can_route"`                 // Patterns this gateway can handle (* for all)
	ExcludePatterns []string `json:"exclude_patterns"`          // Patterns to exclude
	FailoverTo      []string `json:"failover_to"`               // Gateway IDs to failover to
	FallbackDomain  string   `json:"fallback_domain,omitempty"` // Domain to use for rewriting
}

// CircuitBreakerConfig defines circuit breaker settings
type CircuitBreakerConfig struct {
	Enabled          bool   `json:"enabled"`
	FailureThreshold int    `json:"failure_threshold"`      // Failures before opening circuit
	SuccessThreshold int    `json:"success_threshold"`      // Successes before closing circuit
	Timeout          string `json:"timeout"`                // Time before attempting half-open
	MaxRequests      int    `json:"max_requests,omitempty"` // Max requests in half-open state
}

// ParseTimeout converts the timeout string to time.Duration
func (cb CircuitBreakerConfig) ParseTimeout() (time.Duration, error) {
	return time.ParseDuration(cb.Timeout)
}

// HealthCheckConfig defines health check settings
type HealthCheckConfig struct {
	Enabled          bool   `json:"enabled"`
	Interval         string `json:"interval"`          // How often to check
	Timeout          string `json:"timeout"`           // Timeout for each check
	FailureThreshold int    `json:"failure_threshold"` // Consecutive failures before marking unhealthy
	SuccessThreshold int    `json:"success_threshold"` // Consecutive successes before marking healthy
}

// ParseInterval converts the interval string to time.Duration
func (hc HealthCheckConfig) ParseInterval() (time.Duration, error) {
	return time.ParseDuration(hc.Interval)
}

// ParseTimeout converts the timeout string to time.Duration
func (hc HealthCheckConfig) ParseTimeout() (time.Duration, error) {
	return time.ParseDuration(hc.Timeout)
}

// EnhancedGatewayConfig represents the new configuration structure
type EnhancedGatewayConfig struct {
	Gateways       []GatewayConfig       `json:"gateways"`
	LegacyDomains  []string              `json:"legacy_domains,omitempty"`
	GlobalDefaults GlobalGatewayDefaults `json:"global_defaults"`
	Routing        GlobalRoutingConfig   `json:"routing"`
}

// GlobalGatewayDefaults provides default settings for all gateways
type GlobalGatewayDefaults struct {
	RateLimits      GatewayRateLimitConfig  `json:"rate_limits"`
	CircuitBreaker  CircuitBreakerConfig    `json:"circuit_breaker"`
	HealthCheck     HealthCheckConfig       `json:"health_check"`
	RoutingStrategy gateway.RoutingStrategy `json:"routing_strategy"`
}

// GlobalRoutingConfig defines global routing behavior
type GlobalRoutingConfig struct {
	Strategy               gateway.RoutingStrategy `json:"strategy"`
	FailoverEnabled        bool                    `json:"failover_enabled"`
	LoadBalancingEnabled   bool                    `json:"load_balancing_enabled"`
	HealthCheckRequired    bool                    `json:"health_check_required"`
	CircuitBreakerRequired bool                    `json:"circuit_breaker_required"`
}

// LoadGatewayConfig loads the enhanced gateway configuration
func LoadGatewayConfig(filename string) (*EnhancedGatewayConfig, error) {
	// First try to load as enhanced gateway config
	if config, err := loadEnhancedConfig(filename); err == nil {
		return config, nil
	}

	// Fall back to loading as legacy workspace config and converting
	return loadLegacyWorkspaceConfig(filename)
}

// loadEnhancedConfig loads the new enhanced configuration format
func loadEnhancedConfig(filename string) (*EnhancedGatewayConfig, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Expand environment variables in the JSON
	expandedData := os.ExpandEnv(string(data))

	// Try to parse as enhanced config first
	var config EnhancedGatewayConfig
	if err := json.Unmarshal([]byte(expandedData), &config); err == nil && len(config.Gateways) > 0 {
		// Validate that all required fields are present for enhanced config
		for _, gw := range config.Gateways {
			if gw.Type == "" {
				return nil, fmt.Errorf("gateway config missing type field, not enhanced format")
			}
		}
		return &config, nil
	}

	return nil, fmt.Errorf("not an enhanced gateway config")
}

// loadLegacyWorkspaceConfig loads legacy workspace config and converts to new format
func loadLegacyWorkspaceConfig(filename string) (*EnhancedGatewayConfig, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var legacyWorkspaces []WorkspaceConfig
	if err := json.Unmarshal(data, &legacyWorkspaces); err != nil {
		return nil, fmt.Errorf("failed to parse legacy workspace config: %w", err)
	}

	// Convert legacy workspaces to gateway config
	config := &EnhancedGatewayConfig{
		Gateways: make([]GatewayConfig, len(legacyWorkspaces)),
		GlobalDefaults: GlobalGatewayDefaults{
			RateLimits: GatewayRateLimitConfig{
				WorkspaceDaily: 2000,
				PerUserDaily:   200,
			},
			CircuitBreaker: CircuitBreakerConfig{
				Enabled:          true,
				FailureThreshold: 10,
				SuccessThreshold: 5,
				Timeout:          "60s",
			},
			HealthCheck: HealthCheckConfig{
				Enabled:          true,
				Interval:         "30s",
				Timeout:          "10s",
				FailureThreshold: 3,
				SuccessThreshold: 2,
			},
			RoutingStrategy: gateway.StrategyPriority,
		},
		Routing: GlobalRoutingConfig{
			Strategy:               gateway.StrategyPriority,
			FailoverEnabled:        true,
			LoadBalancingEnabled:   false,
			HealthCheckRequired:    true,
			CircuitBreakerRequired: true,
		},
	}

	// Convert legacy workspaces to gateway configs, only handling Mailgun for now
	// Gmail workspaces will be handled by the legacy processor until Google Workspace gateway is implemented
	var gateways []GatewayConfig
	
	for i, ws := range legacyWorkspaces {
		// Skip Gmail workspaces for now - they'll be handled by legacy processor
		// TODO: Re-enable when Google Workspace gateway is implemented
		/*
		if ws.Gmail != nil && ws.Gmail.Enabled {
			gateways = append(gateways, GatewayConfig{...})
		}
		*/
		
		// Create Mailgun gateway if configured
		if ws.Mailgun != nil && ws.Mailgun.Enabled {
			gateways = append(gateways, GatewayConfig{
				ID:          ws.ID,
				Type:        gateway.GatewayTypeMailgun,
				DisplayName: ws.DisplayName,
				Domain:      ws.Domain,
				Enabled:     true,
				Priority:    i + 1, // Convert index to priority
				Weight:      100,
				Mailgun: &MailgunConfig{
					APIKey:  ws.Mailgun.APIKey,
					Domain:  ws.Mailgun.Domain,
					BaseURL: ws.Mailgun.BaseURL,
					Region:  ws.Mailgun.Region,
					Tracking: MailgunTracking{
						Opens:       ws.Mailgun.Tracking.Opens,
						Clicks:      ws.Mailgun.Tracking.Clicks,
						Unsubscribe: ws.Mailgun.Tracking.Unsubscribe,
					},
					Tags: MailgunTagsConfig{
						Default: ws.Mailgun.Tags,
					},
				},
				RateLimits: GatewayRateLimitConfig{
					WorkspaceDaily:   ws.RateLimits.WorkspaceDaily,
					PerUserDaily:     ws.RateLimits.PerUserDaily,
					CustomUserLimits: ws.RateLimits.CustomUserLimits,
				},
				Routing: GatewayRoutingConfig{
					CanRoute:        []string{"@" + ws.Domain},
					ExcludePatterns: []string{},
				},
				CircuitBreaker: config.GlobalDefaults.CircuitBreaker,
			})
		}
	}
	
	config.Gateways = gateways

	return config, nil
}

// Validate validates the gateway configuration
func (gc *GatewayConfig) Validate() error {
	if gc.ID == "" {
		return fmt.Errorf("gateway ID is required")
	}

	if gc.Type == "" {
		return fmt.Errorf("gateway type is required")
	}

	if gc.Domain == "" {
		return fmt.Errorf("gateway domain is required")
	}

	// Validate provider-specific config
	switch gc.Type {
	case gateway.GatewayTypeGoogleWorkspace:
		if gc.GoogleWorkspace == nil {
			return fmt.Errorf("google_workspace configuration is required for Google Workspace gateways")
		}
		if gc.GoogleWorkspace.ServiceAccountFile == "" {
			return fmt.Errorf("service_account_file is required for Google Workspace gateways")
		}

	case gateway.GatewayTypeMailgun:
		if gc.Mailgun == nil {
			return fmt.Errorf("mailgun configuration is required for Mailgun gateways")
		}
		if gc.Mailgun.APIKey == "" {
			return fmt.Errorf("api_key is required for Mailgun gateways")
		}
		if gc.Mailgun.Domain == "" {
			return fmt.Errorf("domain is required for Mailgun gateways")
		}
	}

	// Validate circuit breaker timeout
	if gc.CircuitBreaker.Enabled {
		if _, err := gc.CircuitBreaker.ParseTimeout(); err != nil {
			return fmt.Errorf("invalid circuit breaker timeout: %w", err)
		}
	}

	return nil
}

// GetEffectiveRateLimits returns the effective rate limits for this gateway,
// considering inheritance from global defaults
func (gc *GatewayConfig) GetEffectiveRateLimits(globalDefaults *GlobalGatewayDefaults) GatewayRateLimitConfig {
	effective := gc.RateLimits

	if globalDefaults != nil && gc.RateLimits.InheritGlobalLimits {
		// Inherit values that are not explicitly set
		if effective.WorkspaceDaily == 0 {
			effective.WorkspaceDaily = globalDefaults.RateLimits.WorkspaceDaily
		}
		if effective.PerUserDaily == 0 {
			effective.PerUserDaily = globalDefaults.RateLimits.PerUserDaily
		}
		if effective.PerHour == 0 {
			effective.PerHour = globalDefaults.RateLimits.PerHour
		}
		if effective.BurstLimit == 0 {
			effective.BurstLimit = globalDefaults.RateLimits.BurstLimit
		}
	}

	return effective
}
