package config

import (
	"encoding/json"
	"os"
	"time"

	"github.com/joho/godotenv"
)

type WorkspaceConfig struct {
	ID                 string                   `json:"id"`
	Domain             string                   `json:"domain"`
	ServiceAccountFile string                   `json:"service_account_file"`
	DisplayName        string                   `json:"display_name"`
	RateLimits         WorkspaceRateLimitConfig `json:"rate_limits,omitempty"`
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
	SMTP           SMTPConfig
	Gmail          GmailConfig
	Queue          QueueConfig
	Webhook        WebhookConfig
	LLM            LLMConfig
	Server         ServerConfig
	MySQL          MySQLConfig
}

type SMTPConfig struct {
	Host         string
	Port         int
	MaxSize      int64
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

type GmailConfig struct {
	Workspaces         []WorkspaceConfig
	LegacyDomains      []string // domains that get randomly assigned to workspaces
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
		Gmail: loadGmailConfig(),
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
			Database: getEnvString("MYSQL_DATABASE", "smtp_relay"),
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
					ID:                 domain,
					Domain:             domain,
					ServiceAccountFile: serviceAccountFile,
					DisplayName:        domain,
				},
			},
			LegacyDomains: getEnvStringArray("GMAIL_LEGACY_DOMAINS", []string{"mednet.org", "themednet.org"}),
		}
	}

	return GmailConfig{}
}

func loadWorkspacesFromFile(filename string) []WorkspaceConfig {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil
	}

	var workspaces []WorkspaceConfig
	if err := json.Unmarshal(data, &workspaces); err != nil {
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