package config

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"relay/internal/gateway"

	"github.com/joho/godotenv"
)

type WorkspaceConfig struct {
	ID           string                    `json:"id"`
	Domain       string                    `json:"domain,omitempty"`      // Deprecated: Use Domains instead
	Domains      []string                  `json:"domains,omitempty"`     // Multiple domains per workspace
	DisplayName  string                    `json:"display_name"`
	RateLimits   WorkspaceRateLimitConfig  `json:"rate_limits,omitempty"`
	LoadBalancing *WorkspaceLoadBalancingConfig `json:"load_balancing,omitempty"` // Load balancing configuration

	// Gateway configurations - at least one must be specified
	Gmail    *WorkspaceGmailConfig    `json:"gmail,omitempty"`
	Mailgun  *WorkspaceMailgunConfig  `json:"mailgun,omitempty"`
	Mandrill *WorkspaceMandrillConfig `json:"mandrill,omitempty"`
}

// GetPrimaryDomain returns the primary domain for this workspace
func (w *WorkspaceConfig) GetPrimaryDomain() string {
	if len(w.Domains) > 0 {
		return w.Domains[0]
	}
	return w.Domain // Fallback to legacy single domain
}

// GetCanRouteDomains returns the routing patterns for all domains
func (w *WorkspaceConfig) GetCanRouteDomains() []string {
	var routes []string
	
	// Add all domains from Domains field
	for _, domain := range w.Domains {
		routes = append(routes, "@"+domain)
	}
	
	// Add legacy Domain field if no Domains specified
	if len(routes) == 0 && w.Domain != "" {
		routes = append(routes, "@"+w.Domain)
	}
	
	return routes
}

// IsLoadBalancingEnabled returns true if this workspace participates in load balancing
func (w *WorkspaceConfig) IsLoadBalancingEnabled() bool {
	return w.LoadBalancing != nil && w.LoadBalancing.Enabled
}

// GetLoadBalancingPools returns the pools this workspace can participate in
func (w *WorkspaceConfig) GetLoadBalancingPools() []string {
	if w.LoadBalancing == nil {
		return nil
	}
	return w.LoadBalancing.Pools
}

// GetLoadBalancingWeight returns the default weight for this workspace in pools
func (w *WorkspaceConfig) GetLoadBalancingWeight() float64 {
	if w.LoadBalancing == nil || w.LoadBalancing.DefaultWeight <= 0 {
		return 1.0 // Default weight
	}
	return w.LoadBalancing.DefaultWeight
}

// GetMinCapacityThreshold returns the minimum capacity threshold for this workspace
func (w *WorkspaceConfig) GetMinCapacityThreshold() float64 {
	if w.LoadBalancing == nil || w.LoadBalancing.MinCapacityThreshold <= 0 {
		return 0.01 // Default to 1% minimum capacity
	}
	return w.LoadBalancing.MinCapacityThreshold
}

// GetLoadBalancingPriority returns the priority level for this workspace
func (w *WorkspaceConfig) GetLoadBalancingPriority() int {
	if w.LoadBalancing == nil {
		return 0 // Default priority
	}
	return w.LoadBalancing.Priority
}

// IsHealthCheckEnabled returns true if health checking is enabled for this workspace
func (w *WorkspaceConfig) IsHealthCheckEnabled() bool {
	if w.LoadBalancing == nil {
		return true // Default to enabled
	}
	return w.LoadBalancing.HealthCheckEnabled
}

// GetMaxConcurrentSelections returns the maximum concurrent selections for this workspace
func (w *WorkspaceConfig) GetMaxConcurrentSelections() int {
	if w.LoadBalancing == nil || w.LoadBalancing.MaxConcurrentSelections <= 0 {
		return 10 // Default to 10 concurrent selections
	}
	return w.LoadBalancing.MaxConcurrentSelections
}

// WorkspaceGmailConfig contains Gmail-specific settings for a workspace
type WorkspaceGmailConfig struct {
	ServiceAccountFile string                        `json:"service_account_file,omitempty"` // Path to service account JSON file
	ServiceAccountEnv  string                        `json:"service_account_env,omitempty"`  // Environment variable containing service account JSON
	Enabled            bool                          `json:"enabled"`
	DefaultSender      string                        `json:"default_sender,omitempty"` // Fallback sender when impersonation fails
	RequireValidSender bool                          `json:"require_valid_sender,omitempty"` // Whether to validate sender emails
	HeaderRewrite      WorkspaceGmailHeaderRewrite   `json:"header_rewrite,omitempty"`
	EnableWebhooks     bool                          `json:"enable_webhooks"` // Enable webhook notifications
}

// WorkspaceGmailHeaderRewrite configures header rewriting for Gmail workspaces
type WorkspaceGmailHeaderRewrite struct {
	Enabled bool                           `json:"enabled"`
	Rules   []WorkspaceHeaderRewriteRule   `json:"rules,omitempty"`
}

// WorkspaceMailgunConfig contains Mailgun-specific settings for a workspace
type WorkspaceMailgunConfig struct {
	APIKey         string                        `json:"api_key"`
	BaseURL        string                        `json:"base_url,omitempty"`
	Region         string                        `json:"region,omitempty"`
	Enabled        bool                          `json:"enabled"`
	Tracking       WorkspaceMailgunTracking      `json:"tracking,omitempty"`
	Tags           []string                      `json:"default_tags,omitempty"`
	HeaderRewrite  WorkspaceMailgunHeaderRewrite `json:"header_rewrite,omitempty"`
	EnableWebhooks bool                          `json:"enable_webhooks"` // Enable webhook notifications
}

// WorkspaceMailgunTracking configures Mailgun tracking for a workspace
type WorkspaceMailgunTracking struct {
	Opens       bool `json:"opens"`
	Clicks      bool `json:"clicks"`
	Unsubscribe bool `json:"unsubscribe"`
}

// WorkspaceMailgunHeaderRewrite configures header rewriting for Mailgun workspaces
type WorkspaceMailgunHeaderRewrite struct {
	Enabled bool                           `json:"enabled"`
	Rules   []WorkspaceHeaderRewriteRule   `json:"rules,omitempty"`
}

// WorkspaceMandrillConfig contains Mandrill-specific settings for a workspace
type WorkspaceMandrillConfig struct {
	APIKey         string                         `json:"api_key"`
	BaseURL        string                         `json:"base_url,omitempty"`  // Default: https://mandrillapp.com/api/1.0
	Enabled        bool                           `json:"enabled"`
	Subaccount     string                         `json:"subaccount,omitempty"` // Optional Mandrill subaccount
	Tags           []string                       `json:"default_tags,omitempty"`
	Tracking       WorkspaceMandrillTracking      `json:"tracking,omitempty"`
	HeaderRewrite  WorkspaceMandrillHeaderRewrite `json:"header_rewrite,omitempty"`
	EnableWebhooks bool                           `json:"enable_webhooks"` // Enable webhook notifications
}

// WorkspaceMandrillTracking configures Mandrill tracking for a workspace
type WorkspaceMandrillTracking struct {
	Opens       bool `json:"opens"`
	Clicks      bool `json:"clicks"`
	AutoText    bool `json:"auto_text"`    // Automatically generate text part from HTML
	AutoHtml    bool `json:"auto_html"`    // Automatically generate HTML part from text
	InlineCss   bool `json:"inline_css"`   // Inline CSS styles in HTML emails
	UrlStripQs  bool `json:"url_strip_qs"` // Strip query strings from URLs when tracking
}

// WorkspaceMandrillHeaderRewrite configures header rewriting for Mandrill workspaces
type WorkspaceMandrillHeaderRewrite struct {
	Enabled bool                           `json:"enabled"`
	Rules   []WorkspaceHeaderRewriteRule   `json:"rules,omitempty"`
}

// WorkspaceHeaderRewriteRule defines a header rewriting rule
type WorkspaceHeaderRewriteRule struct {
	HeaderName string `json:"header_name"` // e.g., "List-Unsubscribe"
	NewValue   string `json:"new_value"`   // new header value to replace the original
}

type WorkspaceRateLimitConfig struct {
	// Daily limit for entire workspace (optional - overrides global default)
	WorkspaceDaily int `json:"workspace_daily,omitempty"`

	// Daily limit per user in this workspace (optional - overrides workspace and global)
	PerUserDaily int `json:"per_user_daily,omitempty"`

	// Custom per-user limits (email -> daily limit)
	CustomUserLimits map[string]int `json:"custom_user_limits,omitempty"`
}

// WorkspaceLoadBalancingConfig contains load balancing settings for a workspace
type WorkspaceLoadBalancingConfig struct {
	// Enabled indicates if this workspace participates in load balancing pools
	Enabled bool `json:"enabled"`
	
	// Pools is a list of pool IDs this workspace can participate in
	Pools []string `json:"pools,omitempty"`
	
	// DefaultWeight is the default weight for this workspace in pools (can be overridden per pool)
	DefaultWeight float64 `json:"default_weight,omitempty"`
	
	// MinCapacityThreshold is the minimum capacity percentage to be eligible for selection (0.0-1.0)
	MinCapacityThreshold float64 `json:"min_capacity_threshold,omitempty"`
	
	// Priority sets the priority level for this workspace (higher = preferred in priority-based selection)
	Priority int `json:"priority,omitempty"`
	
	// HealthCheckEnabled enables health checking for this workspace
	HealthCheckEnabled bool `json:"health_check_enabled,omitempty"`
	
	// MaxConcurrentSelections limits the number of concurrent email processing for this workspace
	MaxConcurrentSelections int `json:"max_concurrent_selections,omitempty"`
}

type Config struct {
	SMTP    SMTPConfig
	Gmail   GmailConfig
	Gateway *EnhancedGatewayConfig // New gateway configuration
	Queue   QueueConfig
	Webhook WebhookConfig
	LLM     LLMConfig
	Server  ServerConfig
	MySQL   MySQLConfig
	Blaster BlasterConfig
}

type SMTPConfig struct {
	Host         string
	Port         int
	MaxSize      int64
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

type GmailConfig struct {
	Workspaces    []WorkspaceConfig
	LegacyDomains []string // domains that get randomly assigned to workspaces
}

type QueueConfig struct {
	ProcessInterval time.Duration
	BatchSize       int
	MaxRetries      int
	StoragePath     string
	DailyRateLimit  int
}

type WebhookConfig struct {
	MandrillURL string
	Timeout     time.Duration
	MaxRetries  int
}

type LLMConfig struct {
	Enabled      bool
	Provider     string
	APIKey       string
	Model        string
	Timeout      time.Duration
	PromptPrefix string
}

type ServerConfig struct {
	MetricsPort int
	LogLevel    string
	WebUIPort   int
}

type MySQLConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
}

// GetDSN returns the MySQL Data Source Name for database connections
func (m *MySQLConfig) GetDSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci",
		m.User, m.Password, m.Host, m.Port, m.Database)
}

type BlasterConfig struct {
	BaseURL string
	APIKey  string
}


func Load() (*Config, error) {
	godotenv.Load()

	cfg := &Config{
		SMTP: SMTPConfig{
			Host:         getEnvString("SMTP_HOST", "0.0.0.0"),
			Port:         getEnvInt("SMTP_PORT", 2525),
			MaxSize:      getEnvInt64("SMTP_MAX_SIZE", 10*1024*1024),
			ReadTimeout:  getEnvDuration("SMTP_READ_TIMEOUT", 10*time.Second),
			WriteTimeout: getEnvDuration("SMTP_WRITE_TIMEOUT", 10*time.Second),
		},
		Gmail:   loadGmailConfig(),
		Gateway: loadGatewayConfig(),
		Queue: QueueConfig{
			ProcessInterval: getEnvDuration("QUEUE_PROCESS_INTERVAL", 30*time.Second),
			BatchSize:       getEnvInt("QUEUE_BATCH_SIZE", 10),
			MaxRetries:      getEnvInt("QUEUE_MAX_RETRIES", 3),
			StoragePath:     getEnvString("QUEUE_STORAGE_PATH", "./data/queue"),
			DailyRateLimit:  getEnvInt("QUEUE_DAILY_RATE_LIMIT", 2000),
		},
		Webhook: WebhookConfig{
			MandrillURL: getEnvString("MANDRILL_WEBHOOK_URL", ""),
			Timeout:     getEnvDuration("WEBHOOK_TIMEOUT", 30*time.Second),
			MaxRetries:  getEnvInt("WEBHOOK_MAX_RETRIES", 3),
		},
		LLM: LLMConfig{
			Enabled:      getEnvBool("LLM_ENABLED", false),
			Provider:     getEnvString("LLM_PROVIDER", "openai"),
			APIKey:       getEnvString("LLM_API_KEY", ""),
			Model:        getEnvString("LLM_MODEL", "gpt-3.5-turbo"),
			Timeout:      getEnvDuration("LLM_TIMEOUT", 30*time.Second),
			PromptPrefix: getEnvString("LLM_PROMPT_PREFIX", "Personalize this email:"),
		},
		Server: ServerConfig{
			MetricsPort: getEnvInt("METRICS_PORT", 9090),
			LogLevel:    getEnvString("LOG_LEVEL", "info"),
			WebUIPort:   getEnvInt("WEB_UI_PORT", 8080),
		},
		MySQL: MySQLConfig{
			Host:     getEnvString("MYSQL_HOST", "localhost"),
			Port:     getEnvInt("MYSQL_PORT", 3306),
			User:     getEnvString("MYSQL_USER", "root"),
			Password: getEnvString("MYSQL_PASSWORD", ""),
			Database: getEnvString("MYSQL_DATABASE", "relay"),
		},
		Blaster: BlasterConfig{
			BaseURL: getEnvString("BLASTER_BASE_URL", "http://localhost:3034"),
			APIKey:  getEnvString("BLASTER_API_KEY", ""),
		},
	}

	return cfg, nil
}

func getEnvString(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		var i int
		if err := json.Unmarshal([]byte(value), &i); err == nil {
			return i
		}
	}
	return defaultValue
}

func getEnvInt64(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		var i int64
		if err := json.Unmarshal([]byte(value), &i); err == nil {
			return i
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		var b bool
		if err := json.Unmarshal([]byte(value), &b); err == nil {
			return b
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
	}
	return defaultValue
}

func loadGmailConfig() GmailConfig {
	// Workspace configuration is now loaded from database only
	// This function returns an empty config as workspaces are managed separately
	return GmailConfig{
		Workspaces:    []WorkspaceConfig{},
		LegacyDomains: []string{},
	}
}


// loadGatewayConfig loads the enhanced gateway configuration
func loadGatewayConfig() *EnhancedGatewayConfig {
	// Gateway configuration is now managed separately from workspaces
	// Return default configuration for legacy compatibility
	log.Printf("Using default gateway configuration")
	return &EnhancedGatewayConfig{
		Gateways: []GatewayConfig{},
		GlobalDefaults: GlobalGatewayDefaults{
			RateLimits: GatewayRateLimitConfig{
				WorkspaceDaily: 2000,
				PerUserDaily:   200,
				PerHour:        50,
				BurstLimit:     10,
			},
			CircuitBreaker: CircuitBreakerConfig{
				Enabled:          true,
				FailureThreshold: 10,
				SuccessThreshold: 5,
				Timeout:          "60s",
				MaxRequests:      100,
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
}
