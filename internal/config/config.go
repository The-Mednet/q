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
	ID          string                   `json:"id"`
	Domain      string                   `json:"domain"`
	DisplayName string                   `json:"display_name"`
	RateLimits  WorkspaceRateLimitConfig `json:"rate_limits,omitempty"`

	// Gateway configurations - at least one must be specified
	Gmail   *WorkspaceGmailConfig   `json:"gmail,omitempty"`
	Mailgun *WorkspaceMailgunConfig `json:"mailgun,omitempty"`
}

// WorkspaceGmailConfig contains Gmail-specific settings for a workspace
type WorkspaceGmailConfig struct {
	ServiceAccountFile string                        `json:"service_account_file"`
	Enabled            bool                          `json:"enabled"`
	DefaultSender      string                        `json:"default_sender,omitempty"` // Fallback sender when impersonation fails
	RequireValidSender bool                          `json:"require_valid_sender,omitempty"` // Whether to validate sender emails
	HeaderRewrite      WorkspaceGmailHeaderRewrite   `json:"header_rewrite,omitempty"`
}

// WorkspaceGmailHeaderRewrite configures header rewriting for Gmail workspaces
type WorkspaceGmailHeaderRewrite struct {
	Enabled bool                           `json:"enabled"`
	Rules   []WorkspaceHeaderRewriteRule   `json:"rules,omitempty"`
}

// WorkspaceMailgunConfig contains Mailgun-specific settings for a workspace
type WorkspaceMailgunConfig struct {
	APIKey        string                        `json:"api_key"`
	Domain        string                        `json:"domain"`
	BaseURL       string                        `json:"base_url,omitempty"`
	Region        string                        `json:"region,omitempty"`
	Enabled       bool                          `json:"enabled"`
	Tracking      WorkspaceMailgunTracking      `json:"tracking,omitempty"`
	Tags          []string                      `json:"default_tags,omitempty"`
	HeaderRewrite WorkspaceMailgunHeaderRewrite `json:"header_rewrite,omitempty"`
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
	// Try to load workspace config from JSON file first
	workspacesFile := getEnvString("GMAIL_WORKSPACES_FILE", "")
	if workspacesFile != "" {
		if workspaces := loadWorkspacesFromFile(workspacesFile); len(workspaces) > 0 {
			return GmailConfig{
				Workspaces:    workspaces,
				LegacyDomains: getEnvStringArray("GMAIL_LEGACY_DOMAINS", []string{"mednet.org", "themednet.org"}),
			}
		}
	}

	// Fall back to single workspace from env vars (backward compatibility)
	serviceAccountFile := getEnvString("GMAIL_SERVICE_ACCOUNT_FILE", "credentials/service-account.json")
	domain := getEnvString("GMAIL_DOMAIN", "joinmednet.org")

	if serviceAccountFile != "" && domain != "" {
		return GmailConfig{
			Workspaces: []WorkspaceConfig{
				{
					ID:          domain,
					Domain:      domain,
					DisplayName: domain,
					Gmail: &WorkspaceGmailConfig{
						ServiceAccountFile: serviceAccountFile,
						Enabled:            true,
					},
				},
			},
			LegacyDomains: getEnvStringArray("GMAIL_LEGACY_DOMAINS", []string{"mednet.org", "themednet.org"}),
		}
	}

	return GmailConfig{}
}

func loadWorkspacesFromFile(filename string) []WorkspaceConfig {
	// First try to load from environment variable (for AWS Secrets Manager, etc.)
	if envConfig := loadWorkspacesFromEnv(); len(envConfig) > 0 {
		log.Println("Loaded workspace configuration from environment variable")
		return envConfig
	}

	// Fall back to file-based loading
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil
	}

	var workspaces []WorkspaceConfig
	if err := json.Unmarshal(data, &workspaces); err != nil {
		return nil
	}

	log.Printf("Loaded workspace configuration from file: %s", filename)
	return workspaces
}

// loadWorkspacesFromEnv loads workspace configuration from environment variable
func loadWorkspacesFromEnv() []WorkspaceConfig {
	// Check for workspace configuration in environment variable
	envConfig := getEnvString("GMAIL_WORKSPACES_JSON", "")
	if envConfig == "" {
		return nil
	}

	var workspaces []WorkspaceConfig
	if err := json.Unmarshal([]byte(envConfig), &workspaces); err != nil {
		log.Printf("Warning: Failed to parse GMAIL_WORKSPACES_JSON: %v", err)
		return nil
	}

	return workspaces
}

func getEnvStringArray(key string, defaultValue []string) []string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}

	var result []string
	if err := json.Unmarshal([]byte(value), &result); err != nil {
		// If JSON parsing fails, treat as comma-separated string
		return []string{value}
	}

	return result
}

// loadGatewayConfig loads the enhanced gateway configuration
func loadGatewayConfig() *EnhancedGatewayConfig {
	// Try to load from file first
	gatewayConfigFile := getEnvString("GATEWAY_CONFIG_FILE", "")
	if gatewayConfigFile != "" {
		if config, err := LoadGatewayConfig(gatewayConfigFile); err == nil {
			log.Printf("Loaded gateway configuration from %s", gatewayConfigFile)
			return config
		} else {
			log.Printf("Warning: Failed to load gateway configuration from %s: %v", gatewayConfigFile, err)
		}
	}

	// Try to load legacy workspaces file for backward compatibility
	workspacesFile := getEnvString("GMAIL_WORKSPACES_FILE", "")
	if workspacesFile != "" {
		if config, err := LoadGatewayConfig(workspacesFile); err == nil {
			log.Printf("Loaded legacy workspace configuration from %s (converted to gateway format)", workspacesFile)
			return config
		}
	}

	// Default configuration for new deployments
	log.Printf("No gateway configuration file found, using defaults")
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
