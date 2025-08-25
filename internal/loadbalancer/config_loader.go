package loadbalancer

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
)

// LoadBalancingConfig represents the overall configuration for load balancing
type LoadBalancingConfig struct {
	Pools        []*LoadBalancingPool `json:"load_balancing_pools"`
	Enabled      bool                 `json:"enabled"`
	Config       *LoadBalancerConfig  `json:"config,omitempty"`
	DefaultPoolID string              `json:"default_pool_id,omitempty"` // Pool to use when no domain match
}

// ConfigLoader handles loading and parsing load balancing configurations
type ConfigLoader struct {
	defaultConfig *LoadBalancerConfig
}

// NewConfigLoader creates a new configuration loader
func NewConfigLoader() *ConfigLoader {
	return &ConfigLoader{
		defaultConfig: DefaultLoadBalancerConfig(),
	}
}

// NewConfigLoaderWithDefaults creates a configuration loader with custom defaults
func NewConfigLoaderWithDefaults(defaultConfig *LoadBalancerConfig) *ConfigLoader {
	return &ConfigLoader{
		defaultConfig: defaultConfig,
	}
}

// LoadFromFile loads load balancing configuration from a JSON file
func (cl *ConfigLoader) LoadFromFile(filePath string) (*LoadBalancingConfig, error) {
	if filePath == "" {
		return nil, fmt.Errorf("configuration file path is required")
	}

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("configuration file does not exist: %s", filePath)
	}

	// Read file contents
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read configuration file %s: %w", filePath, err)
	}

	return cl.parseConfiguration(data, fmt.Sprintf("file: %s", filePath))
}

// LoadFromEnvironment loads load balancing configuration from environment variable
func (cl *ConfigLoader) LoadFromEnvironment(envVarName string) (*LoadBalancingConfig, error) {
	if envVarName == "" {
		envVarName = "LOAD_BALANCING_CONFIG" // Default environment variable name
	}

	configJSON := os.Getenv(envVarName)
	if configJSON == "" {
		return nil, fmt.Errorf("environment variable %s is not set or empty", envVarName)
	}

	return cl.parseConfiguration([]byte(configJSON), fmt.Sprintf("env: %s", envVarName))
}

// LoadFromString loads load balancing configuration from a JSON string
func (cl *ConfigLoader) LoadFromString(configJSON string) (*LoadBalancingConfig, error) {
	if configJSON == "" {
		return nil, fmt.Errorf("configuration string is empty")
	}

	return cl.parseConfiguration([]byte(configJSON), "string")
}

// LoadWithFallback loads configuration with multiple fallback sources
func (cl *ConfigLoader) LoadWithFallback(sources ...ConfigSource) (*LoadBalancingConfig, error) {
	var lastErr error

	for i, source := range sources {
		config, err := cl.loadFromSource(source)
		if err != nil {
			lastErr = err
			log.Printf("Failed to load load balancing config from source %d (%s): %v", i+1, source.Type, err)
			continue
		}

		log.Printf("Successfully loaded load balancing configuration from source %d (%s)", i+1, source.Type)
		return config, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("failed to load configuration from all sources, last error: %w", lastErr)
	}

	return nil, fmt.Errorf("no configuration sources provided")
}

// ConfigSource represents a configuration source
type ConfigSource struct {
	Type  string // "file", "env", "string"
	Value string // file path, env var name, or JSON string
}

// loadFromSource loads configuration from a specific source
func (cl *ConfigLoader) loadFromSource(source ConfigSource) (*LoadBalancingConfig, error) {
	switch strings.ToLower(source.Type) {
	case "file":
		return cl.LoadFromFile(source.Value)
	case "env", "environment":
		return cl.LoadFromEnvironment(source.Value)
	case "string", "json":
		return cl.LoadFromString(source.Value)
	default:
		return nil, fmt.Errorf("unsupported source type: %s", source.Type)
	}
}

// parseConfiguration parses JSON configuration data
func (cl *ConfigLoader) parseConfiguration(data []byte, source string) (*LoadBalancingConfig, error) {
	var config LoadBalancingConfig

	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse configuration from %s: %w", source, err)
	}

	// Apply defaults
	if config.Config == nil {
		config.Config = cl.defaultConfig
	} else {
		cl.applyConfigDefaults(config.Config)
	}

	// Validate configuration
	if err := cl.validateConfiguration(&config); err != nil {
		return nil, fmt.Errorf("invalid configuration from %s: %w", source, err)
	}

	log.Printf("Loaded %d load balancing pools from %s", len(config.Pools), source)
	return &config, nil
}

// validateConfiguration validates the loaded configuration
func (cl *ConfigLoader) validateConfiguration(config *LoadBalancingConfig) error {
	if config == nil {
		return fmt.Errorf("configuration is nil")
	}

	// Validate each pool
	poolIDs := make(map[string]bool)
	for i, pool := range config.Pools {
		if pool == nil {
			return fmt.Errorf("pool %d is nil", i)
		}

		// Check for duplicate pool IDs
		if poolIDs[pool.ID] {
			return fmt.Errorf("duplicate pool ID: %s", pool.ID)
		}
		poolIDs[pool.ID] = true

		// Validate individual pool
		if err := cl.validatePool(pool); err != nil {
			return fmt.Errorf("pool %s validation failed: %w", pool.ID, err)
		}
	}

	return nil
}

// validatePool validates a single pool configuration
func (cl *ConfigLoader) validatePool(pool *LoadBalancingPool) error {
	if pool.ID == "" {
		return fmt.Errorf("pool ID is required")
	}

	if pool.Name == "" {
		return fmt.Errorf("pool name is required")
	}

	if len(pool.DomainPatterns) == 0 {
		return fmt.Errorf("at least one domain pattern is required")
	}

	if len(pool.Workspaces) == 0 {
		return fmt.Errorf("at least one workspace is required")
	}

	// Validate strategy
	validStrategies := map[SelectionStrategy]bool{
		StrategyCapacityWeighted: true,
		StrategyRoundRobin:       true,
		StrategyLeastUsed:        true,
		StrategyRandomWeighted:   true,
	}

	if !validStrategies[pool.Strategy] {
		return fmt.Errorf("invalid selection strategy: %s", pool.Strategy)
	}

	// Validate workspaces
	workspaceIDs := make(map[string]bool)
	for i, ws := range pool.Workspaces {
		if ws.WorkspaceID == "" {
			return fmt.Errorf("workspace %d: workspace ID is required", i)
		}

		// Check for duplicate workspace IDs within pool
		if workspaceIDs[ws.WorkspaceID] {
			return fmt.Errorf("duplicate workspace ID in pool: %s", ws.WorkspaceID)
		}
		workspaceIDs[ws.WorkspaceID] = true

		if ws.Weight <= 0 {
			return fmt.Errorf("workspace %d (%s): weight must be positive", i, ws.WorkspaceID)
		}

		if ws.MinCapacityThreshold < 0 || ws.MinCapacityThreshold > 1 {
			return fmt.Errorf("workspace %d (%s): minimum capacity threshold must be between 0 and 1", i, ws.WorkspaceID)
		}
	}

	// Validate domain patterns
	for i, pattern := range pool.DomainPatterns {
		if pattern == "" {
			return fmt.Errorf("domain pattern %d: pattern cannot be empty", i)
		}

		// Clean pattern and basic validation
		cleanPattern := strings.TrimPrefix(strings.TrimSpace(pattern), "@")
		if cleanPattern == "" {
			return fmt.Errorf("domain pattern %d: invalid pattern '%s'", i, pattern)
		}

		// Basic domain format validation
		if strings.Contains(cleanPattern, " ") {
			return fmt.Errorf("domain pattern %d: pattern cannot contain spaces: '%s'", i, pattern)
		}
	}

	return nil
}

// applyConfigDefaults applies default values to configuration
func (cl *ConfigLoader) applyConfigDefaults(config *LoadBalancerConfig) {
	defaults := cl.defaultConfig

	if config.CacheTTL == 0 {
		config.CacheTTL = defaults.CacheTTL
	}

	if config.MaxCandidates == 0 {
		config.MaxCandidates = defaults.MaxCandidates
	}

	if config.HealthCheckInterval == 0 {
		config.HealthCheckInterval = defaults.HealthCheckInterval
	}

	if config.SelectionTimeout == 0 {
		config.SelectionTimeout = defaults.SelectionTimeout
	}
}

// CreateSampleConfiguration creates a sample configuration for reference
func (cl *ConfigLoader) CreateSampleConfiguration() *LoadBalancingConfig {
	return &LoadBalancingConfig{
		Enabled: true,
		Config:  cl.defaultConfig,
		Pools: []*LoadBalancingPool{
			{
				ID:             "invite-domain-pool",
				Name:           "Invite Domain Distribution",
				DomainPatterns: []string{"invite.com", "invitations.mednet.org"},
				Strategy:       StrategyCapacityWeighted,
				Enabled:        true,
				Workspaces: []PoolWorkspace{
					{
						WorkspaceID:          "gmail-workspace-1",
						Weight:               2.0,
						Enabled:              true,
						MinCapacityThreshold: 0.1, // 10% minimum capacity
					},
					{
						WorkspaceID:          "mailgun-workspace-1",
						Weight:               1.5,
						Enabled:              true,
						MinCapacityThreshold: 0.05, // 5% minimum capacity
					},
					{
						WorkspaceID:          "mandrill-workspace-1",
						Weight:               1.0,
						Enabled:              true,
						MinCapacityThreshold: 0.05, // 5% minimum capacity
					},
				},
			},
			{
				ID:             "medical-notifications-pool",
				Name:           "Medical Notification Distribution",
				DomainPatterns: []string{"notifications.mednet.org", "alerts.mednet.org"},
				Strategy:       StrategyLeastUsed,
				Enabled:        true,
				Workspaces: []PoolWorkspace{
					{
						WorkspaceID:          "gmail-medical-1",
						Weight:               1.0,
						Enabled:              true,
						MinCapacityThreshold: 0.2, // 20% minimum capacity for critical notifications
					},
					{
						WorkspaceID:          "mailgun-medical-1",
						Weight:               1.0,
						Enabled:              true,
						MinCapacityThreshold: 0.2,
					},
				},
			},
		},
	}
}

// SaveSampleConfiguration saves a sample configuration to a file
func (cl *ConfigLoader) SaveSampleConfiguration(filePath string) error {
	sampleConfig := cl.CreateSampleConfiguration()

	data, err := json.MarshalIndent(sampleConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal sample configuration: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write sample configuration to %s: %w", filePath, err)
	}

	log.Printf("Sample load balancing configuration saved to %s", filePath)
	return nil
}

// GetConfigurationSources returns common configuration sources to try
func (cl *ConfigLoader) GetConfigurationSources() []ConfigSource {
	return []ConfigSource{
		{Type: "env", Value: "LOAD_BALANCING_CONFIG_JSON"},      // Primary env var
		{Type: "env", Value: "LOAD_BALANCING_CONFIG"},           // Fallback env var
		{Type: "file", Value: "load_balancing_config.json"},     // Primary file
		{Type: "file", Value: "config/load_balancing.json"},     // Config directory
		{Type: "file", Value: "./load_balancing.json"},          // Current directory
	}
}

// LoadFromCommonSources loads configuration from common sources with fallback
func (cl *ConfigLoader) LoadFromCommonSources() (*LoadBalancingConfig, error) {
	sources := cl.GetConfigurationSources()
	return cl.LoadWithFallback(sources...)
}

// MergeConfigurations merges multiple configurations (pools from all configs)
func (cl *ConfigLoader) MergeConfigurations(configs ...*LoadBalancingConfig) (*LoadBalancingConfig, error) {
	if len(configs) == 0 {
		return &LoadBalancingConfig{
			Enabled: false,
			Config:  cl.defaultConfig,
			Pools:   []*LoadBalancingPool{},
		}, nil
	}

	merged := &LoadBalancingConfig{
		Enabled: false, // Will be set to true if any config is enabled
		Config:  cl.defaultConfig,
		Pools:   []*LoadBalancingPool{},
	}

	poolIDs := make(map[string]bool)

	for _, config := range configs {
		if config == nil {
			continue
		}

		if config.Enabled {
			merged.Enabled = true
		}

		// Merge configuration settings (last one wins)
		if config.Config != nil {
			merged.Config = config.Config
		}

		// Merge pools (skip duplicates)
		for _, pool := range config.Pools {
			if pool == nil {
				continue
			}

			if poolIDs[pool.ID] {
				log.Printf("Warning: Skipping duplicate pool ID during merge: %s", pool.ID)
				continue
			}

			poolIDs[pool.ID] = true
			merged.Pools = append(merged.Pools, pool)
		}
	}

	// Validate merged configuration
	if err := cl.validateConfiguration(merged); err != nil {
		return nil, fmt.Errorf("merged configuration validation failed: %w", err)
	}

	log.Printf("Merged %d configurations into %d pools", len(configs), len(merged.Pools))
	return merged, nil
}

// GetPoolByID finds a pool by ID in the configuration
func (config *LoadBalancingConfig) GetPoolByID(poolID string) *LoadBalancingPool {
	for _, pool := range config.Pools {
		if pool != nil && pool.ID == poolID {
			return pool
		}
	}
	return nil
}

// GetPoolsForDomain returns pools that can handle the specified domain
func (config *LoadBalancingConfig) GetPoolsForDomain(domain string) []*LoadBalancingPool {
	var matchingPools []*LoadBalancingPool

	for _, pool := range config.Pools {
		if pool == nil || !pool.Enabled {
			continue
		}

		for _, pattern := range pool.DomainPatterns {
			cleanPattern := strings.TrimPrefix(strings.TrimSpace(pattern), "@")
			if cleanPattern == domain {
				matchingPools = append(matchingPools, pool)
				break // Found match, no need to check other patterns for this pool
			}
		}
	}

	return matchingPools
}

// GetEnabledPools returns only enabled pools
func (config *LoadBalancingConfig) GetEnabledPools() []*LoadBalancingPool {
	var enabledPools []*LoadBalancingPool

	for _, pool := range config.Pools {
		if pool != nil && pool.Enabled {
			enabledPools = append(enabledPools, pool)
		}
	}

	return enabledPools
}

// GetStats returns statistics about the configuration
func (config *LoadBalancingConfig) GetStats() map[string]interface{} {
	totalPools := len(config.Pools)
	enabledPools := len(config.GetEnabledPools())
	totalWorkspaces := 0
	totalDomains := 0

	domainSet := make(map[string]bool)

	for _, pool := range config.Pools {
		if pool == nil {
			continue
		}

		totalWorkspaces += len(pool.Workspaces)
		
		for _, pattern := range pool.DomainPatterns {
			cleanPattern := strings.TrimPrefix(strings.TrimSpace(pattern), "@")
			domainSet[cleanPattern] = true
		}
	}

	totalDomains = len(domainSet)

	return map[string]interface{}{
		"total_pools":     totalPools,
		"enabled_pools":   enabledPools,
		"disabled_pools":  totalPools - enabledPools,
		"total_workspaces": totalWorkspaces,
		"total_domains":   totalDomains,
		"configuration_enabled": config.Enabled,
	}
}