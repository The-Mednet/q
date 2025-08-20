package gateway

import (
	"context"
	"relay/pkg/models"
	"time"
)

// GatewayType represents the type of email gateway
type GatewayType string

const (
	GatewayTypeGoogleWorkspace GatewayType = "google_workspace"
	GatewayTypeMailgun         GatewayType = "mailgun"
	GatewayTypeSendGrid        GatewayType = "sendgrid"
	GatewayTypeAmazonSES       GatewayType = "amazon_ses"
)

// GatewayStatus represents the health status of a gateway
type GatewayStatus string

const (
	GatewayStatusHealthy   GatewayStatus = "healthy"
	GatewayStatusDegraded  GatewayStatus = "degraded"
	GatewayStatusUnhealthy GatewayStatus = "unhealthy"
	GatewayStatusDisabled  GatewayStatus = "disabled"
)

// GatewayInterface defines the contract that all email gateways must implement
type GatewayInterface interface {
	// Core functionality
	SendMessage(ctx context.Context, msg *models.Message) (*SendResult, error)
	GetType() GatewayType
	GetID() string

	// Health and status
	HealthCheck(ctx context.Context) error
	GetStatus() GatewayStatus
	GetLastError() error

	// Rate limiting support
	GetRateLimit() RateLimit
	CanSend(ctx context.Context, senderEmail string) (bool, error)

	// Configuration and routing
	CanRoute(senderEmail string) bool
	GetPriority() int
	GetWeight() int

	// Analytics and metrics
	GetMetrics() GatewayMetrics
	GetSupportedFeatures() []GatewayFeature
}

// SendResult contains the result of sending a message through a gateway
type SendResult struct {
	MessageID   string                 `json:"message_id"`
	GatewayID   string                 `json:"gateway_id"`
	GatewayType GatewayType            `json:"gateway_type"`
	Status      string                 `json:"status"`
	QueueTime   *time.Duration         `json:"queue_time,omitempty"`
	SendTime    time.Duration          `json:"send_time"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Error       *string                `json:"error,omitempty"`
}

// RateLimit represents the rate limiting configuration for a gateway
type RateLimit struct {
	DailyLimit   int            `json:"daily_limit"`
	PerUserLimit int            `json:"per_user_limit"`
	PerHourLimit int            `json:"per_hour_limit,omitempty"`
	BurstLimit   int            `json:"burst_limit,omitempty"`
	CustomLimits map[string]int `json:"custom_limits,omitempty"`
	ResetTime    time.Time      `json:"reset_time"`
}

// GatewayMetrics represents performance metrics for a gateway
type GatewayMetrics struct {
	TotalSent        int64         `json:"total_sent"`
	TotalFailed      int64         `json:"total_failed"`
	TotalRateLimited int64         `json:"total_rate_limited"`
	SuccessRate      float64       `json:"success_rate"`
	AverageLatency   time.Duration `json:"average_latency"`
	LastSent         *time.Time    `json:"last_sent,omitempty"`
	Uptime           time.Duration `json:"uptime"`
	ErrorRate        float64       `json:"error_rate"`
}

// GatewayFeature represents features supported by a gateway
type GatewayFeature string

const (
	FeatureTracking    GatewayFeature = "tracking"
	FeatureTags        GatewayFeature = "tags"
	FeatureMetadata    GatewayFeature = "metadata"
	FeatureTemplating  GatewayFeature = "templating"
	FeatureScheduling  GatewayFeature = "scheduling"
	FeatureAttachments GatewayFeature = "attachments"
	FeatureWebhooks    GatewayFeature = "webhooks"
	FeatureDomainKeys  GatewayFeature = "domain_keys"
	FeatureAnalytics   GatewayFeature = "analytics"
)

// GatewayRouter handles routing messages to appropriate gateways
type GatewayRouter interface {
	// Route a message to the best available gateway
	RouteMessage(ctx context.Context, msg *models.Message) (GatewayInterface, error)

	// Get all available gateways for a sender
	GetAvailableGateways(senderEmail string) []GatewayInterface

	// Health management
	UpdateGatewayHealth(gatewayID string, status GatewayStatus, err error)
	GetHealthyGateways() []GatewayInterface

	// Configuration
	GetRoutingStrategy() RoutingStrategy
	SetRoutingStrategy(strategy RoutingStrategy)
}

// RoutingStrategy defines how messages are routed across gateways
type RoutingStrategy string

const (
	StrategyRoundRobin  RoutingStrategy = "round_robin"
	StrategyWeighted    RoutingStrategy = "weighted"
	StrategyPriority    RoutingStrategy = "priority"
	StrategyFailover    RoutingStrategy = "failover"
	StrategyLeastLoaded RoutingStrategy = "least_loaded"
	StrategyDomainBased RoutingStrategy = "domain_based"
)

// GatewayManager manages all gateways and their lifecycle
type GatewayManager interface {
	// Gateway lifecycle
	RegisterGateway(gateway GatewayInterface) error
	UnregisterGateway(gatewayID string) error
	GetGateway(gatewayID string) (GatewayInterface, error)
	GetAllGateways() []GatewayInterface

	// Health monitoring
	StartHealthMonitoring(ctx context.Context, interval time.Duration)
	StopHealthMonitoring()

	// Statistics
	GetAggregateMetrics() AggregateMetrics
	GetGatewayStats() map[string]GatewayMetrics
}

// AggregateMetrics represents system-wide metrics across all gateways
type AggregateMetrics struct {
	TotalMessages      int64                     `json:"total_messages"`
	TotalSent          int64                     `json:"total_sent"`
	TotalFailed        int64                     `json:"total_failed"`
	OverallSuccessRate float64                   `json:"overall_success_rate"`
	GatewayStats       map[string]GatewayMetrics `json:"gateway_stats"`
	HealthyGateways    int                       `json:"healthy_gateways"`
	TotalGateways      int                       `json:"total_gateways"`
	LastProcessed      *time.Time                `json:"last_processed,omitempty"`
}

// CircuitBreakerInterface defines circuit breaker functionality
type CircuitBreakerInterface interface {
	Execute(ctx context.Context, fn func() error) error
	GetState() CircuitBreakerState
	GetMetrics() CircuitBreakerMetrics
	Reset()
}

// CircuitBreakerState represents the state of a circuit breaker
type CircuitBreakerState string

const (
	CircuitBreakerClosed   CircuitBreakerState = "closed"
	CircuitBreakerOpen     CircuitBreakerState = "open"
	CircuitBreakerHalfOpen CircuitBreakerState = "half_open"
)

// CircuitBreakerMetrics represents circuit breaker metrics
type CircuitBreakerMetrics struct {
	State           CircuitBreakerState `json:"state"`
	FailureCount    int64               `json:"failure_count"`
	SuccessCount    int64               `json:"success_count"`
	LastFailureTime *time.Time          `json:"last_failure_time,omitempty"`
	LastSuccessTime *time.Time          `json:"last_success_time,omitempty"`
	NextAttemptTime *time.Time          `json:"next_attempt_time,omitempty"`
}
