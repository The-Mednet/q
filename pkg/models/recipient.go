package models

import (
	"database/sql/driver"
	"fmt"
	"time"
)

// RecipientStatus represents the status of a recipient
type RecipientStatus string

const (
	RecipientStatusActive       RecipientStatus = "ACTIVE"
	RecipientStatusInactive     RecipientStatus = "INACTIVE"
	RecipientStatusBounced      RecipientStatus = "BOUNCED"
	RecipientStatusUnsubscribed RecipientStatus = "UNSUBSCRIBED"
)

func (rs RecipientStatus) String() string {
	return string(rs)
}

func (rs *RecipientStatus) Scan(value interface{}) error {
	if value == nil {
		*rs = RecipientStatusActive
		return nil
	}
	switch s := value.(type) {
	case string:
		*rs = RecipientStatus(s)
	case []byte:
		*rs = RecipientStatus(s)
	default:
		return fmt.Errorf("cannot scan %T into RecipientStatus", value)
	}
	return nil
}

func (rs RecipientStatus) Value() (driver.Value, error) {
	return string(rs), nil
}

// BounceType represents the type of bounce
type BounceType string

const (
	BounceTypeSoft BounceType = "SOFT"
	BounceTypeHard BounceType = "HARD"
)

func (bt BounceType) String() string {
	return string(bt)
}

func (bt *BounceType) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	switch s := value.(type) {
	case string:
		*bt = BounceType(s)
	case []byte:
		*bt = BounceType(s)
	default:
		return fmt.Errorf("cannot scan %T into BounceType", value)
	}
	return nil
}

func (bt BounceType) Value() (driver.Value, error) {
	if bt == "" {
		return nil, nil
	}
	return string(bt), nil
}

// Recipient represents a recipient in the system
type Recipient struct {
	ID               int64                  `json:"id" db:"id"`
	EmailAddress     string                 `json:"email_address" db:"email_address"`
	WorkspaceID      string                 `json:"workspace_id" db:"workspace_id"`
	UserID           *string                `json:"user_id,omitempty" db:"user_id"`
	CampaignID       *string                `json:"campaign_id,omitempty" db:"campaign_id"`
	FirstName        *string                `json:"first_name,omitempty" db:"first_name"`
	LastName         *string                `json:"last_name,omitempty" db:"last_name"`
	Status           RecipientStatus        `json:"status" db:"status"`
	OptInDate        *time.Time             `json:"opt_in_date,omitempty" db:"opt_in_date"`
	OptOutDate       *time.Time             `json:"opt_out_date,omitempty" db:"opt_out_date"`
	BounceCount      int                    `json:"bounce_count" db:"bounce_count"`
	LastBounceDate   *time.Time             `json:"last_bounce_date,omitempty" db:"last_bounce_date"`
	BounceType       *BounceType            `json:"bounce_type,omitempty" db:"bounce_type"`
	Metadata         map[string]interface{} `json:"metadata,omitempty" db:"metadata"`
	CreatedAt        time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at" db:"updated_at"`
}

// RecipientType represents the type of recipient (TO, CC, BCC)
type RecipientType string

const (
	RecipientTypeTo  RecipientType = "TO"
	RecipientTypeCc  RecipientType = "CC"
	RecipientTypeBcc RecipientType = "BCC"
)

func (rt RecipientType) String() string {
	return string(rt)
}

func (rt *RecipientType) Scan(value interface{}) error {
	if value == nil {
		*rt = RecipientTypeTo
		return nil
	}
	switch s := value.(type) {
	case string:
		*rt = RecipientType(s)
	case []byte:
		*rt = RecipientType(s)
	default:
		return fmt.Errorf("cannot scan %T into RecipientType", value)
	}
	return nil
}

func (rt RecipientType) Value() (driver.Value, error) {
	return string(rt), nil
}

// DeliveryStatus represents the delivery status of a message to a recipient
type DeliveryStatus string

const (
	DeliveryStatusPending  DeliveryStatus = "PENDING"
	DeliveryStatusSent     DeliveryStatus = "SENT"
	DeliveryStatusBounced  DeliveryStatus = "BOUNCED"
	DeliveryStatusFailed   DeliveryStatus = "FAILED"
	DeliveryStatusDeferred DeliveryStatus = "DEFERRED"
)

func (ds DeliveryStatus) String() string {
	return string(ds)
}

func (ds *DeliveryStatus) Scan(value interface{}) error {
	if value == nil {
		*ds = DeliveryStatusPending
		return nil
	}
	switch s := value.(type) {
	case string:
		*ds = DeliveryStatus(s)
	case []byte:
		*ds = DeliveryStatus(s)
	default:
		return fmt.Errorf("cannot scan %T into DeliveryStatus", value)
	}
	return nil
}

func (ds DeliveryStatus) Value() (driver.Value, error) {
	return string(ds), nil
}

// MessageRecipient represents the junction between messages and recipients
type MessageRecipient struct {
	ID                  int64          `json:"id" db:"id"`
	MessageID           string         `json:"message_id" db:"message_id"`
	RecipientID         int64          `json:"recipient_id" db:"recipient_id"`
	RecipientType       RecipientType  `json:"recipient_type" db:"recipient_type"`
	DeliveryStatus      DeliveryStatus `json:"delivery_status" db:"delivery_status"`
	SentAt              *time.Time     `json:"sent_at,omitempty" db:"sent_at"`
	BounceReason        *string        `json:"bounce_reason,omitempty" db:"bounce_reason"`
	
	// Gateway tracking fields
	GatewayID           *string        `json:"gateway_id,omitempty" db:"gateway_id"`
	GatewayType         *string        `json:"gateway_type,omitempty" db:"gateway_type"`
	SendAttemptCount    int            `json:"send_attempt_count" db:"send_attempt_count"`
	LastSendAttempt     *time.Time     `json:"last_send_attempt,omitempty" db:"last_send_attempt"`
	
	Opens               int            `json:"opens" db:"opens"`
	Clicks              int            `json:"clicks" db:"clicks"`
	LastOpenAt          *time.Time     `json:"last_open_at,omitempty" db:"last_open_at"`
	LastClickAt         *time.Time     `json:"last_click_at,omitempty" db:"last_click_at"`
	CreatedAt           time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at" db:"updated_at"`
}

// EventType represents the type of recipient engagement event
type EventType string

const (
	EventTypeOpen        EventType = "OPEN"
	EventTypeClick       EventType = "CLICK"
	EventTypeUnsubscribe EventType = "UNSUBSCRIBE"
	EventTypeComplaint   EventType = "COMPLAINT"
	EventTypeBounce      EventType = "BOUNCE"
)

func (et EventType) String() string {
	return string(et)
}

func (et *EventType) Scan(value interface{}) error {
	if value == nil {
		return fmt.Errorf("EventType cannot be null")
	}
	switch s := value.(type) {
	case string:
		*et = EventType(s)
	case []byte:
		*et = EventType(s)
	default:
		return fmt.Errorf("cannot scan %T into EventType", value)
	}
	return nil
}

func (et EventType) Value() (driver.Value, error) {
	return string(et), nil
}

// RecipientEvent represents an engagement event for a recipient
type RecipientEvent struct {
	ID                   int64                  `json:"id" db:"id"`
	MessageRecipientID   int64                  `json:"message_recipient_id" db:"message_recipient_id"`
	EventType            EventType              `json:"event_type" db:"event_type"`
	EventData            map[string]interface{} `json:"event_data,omitempty" db:"event_data"`
	IPAddress            *string                `json:"ip_address,omitempty" db:"ip_address"`
	UserAgent            *string                `json:"user_agent,omitempty" db:"user_agent"`
	CreatedAt            time.Time              `json:"created_at" db:"created_at"`
}

// RecipientList represents a named list of recipients
type RecipientList struct {
	ID          int64     `json:"id" db:"id"`
	Name        string    `json:"name" db:"name"`
	Description *string   `json:"description,omitempty" db:"description"`
	WorkspaceID string    `json:"workspace_id" db:"workspace_id"`
	UserID      string    `json:"user_id" db:"user_id"`
	IsActive    bool      `json:"is_active" db:"is_active"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// RecipientListMember represents membership in a recipient list
type RecipientListMember struct {
	ID          int64     `json:"id" db:"id"`
	ListID      int64     `json:"list_id" db:"list_id"`
	RecipientID int64     `json:"recipient_id" db:"recipient_id"`
	AddedAt     time.Time `json:"added_at" db:"added_at"`
	AddedBy     *string   `json:"added_by,omitempty" db:"added_by"`
}

// RecipientSummary provides aggregated stats for a recipient
type RecipientSummary struct {
	Recipient       *Recipient `json:"recipient"`
	TotalMessages   int        `json:"total_messages"`
	TotalOpens      int        `json:"total_opens"`
	TotalClicks     int        `json:"total_clicks"`
	LastActivity    *time.Time `json:"last_activity,omitempty"`
	EngagementRate  float64    `json:"engagement_rate"`
}

// CampaignRecipientStats provides aggregated stats for a campaign
type CampaignRecipientStats struct {
	CampaignID      string  `json:"campaign_id"`
	TotalRecipients int     `json:"total_recipients"`
	Sent            int     `json:"sent"`
	Bounced         int     `json:"bounced"`
	Opened          int     `json:"opened"`
	Clicked         int     `json:"clicked"`
	Unsubscribed    int     `json:"unsubscribed"`
	OpenRate        float64 `json:"open_rate"`
	ClickRate       float64 `json:"click_rate"`
	BounceRate      float64 `json:"bounce_rate"`
}

// GatewayUsageStats represents aggregated usage statistics for a gateway
type GatewayUsageStats struct {
	ID                      int64     `json:"id" db:"id"`
	GatewayID               string    `json:"gateway_id" db:"gateway_id"`
	GatewayType             string    `json:"gateway_type" db:"gateway_type"`
	DateBucket              time.Time `json:"date_bucket" db:"date_bucket"`
	HourBucket              *int      `json:"hour_bucket,omitempty" db:"hour_bucket"`
	TotalAttempts           int       `json:"total_attempts" db:"total_attempts"`
	TotalSent               int       `json:"total_sent" db:"total_sent"`
	TotalBounced            int       `json:"total_bounced" db:"total_bounced"`
	TotalFailed             int       `json:"total_failed" db:"total_failed"`
	TotalDeferred           int       `json:"total_deferred" db:"total_deferred"`
	AverageLatencyMs        *int      `json:"average_latency_ms,omitempty" db:"average_latency_ms"`
	SuccessRate             float64   `json:"success_rate" db:"success_rate"`
	RateLimitHits           int       `json:"rate_limit_hits" db:"rate_limit_hits"`
	CircuitBreakerTrips     int       `json:"circuit_breaker_trips" db:"circuit_breaker_trips"`
	TotalOpens              int       `json:"total_opens" db:"total_opens"`
	TotalClicks             int       `json:"total_clicks" db:"total_clicks"`
	UniqueOpens             int       `json:"unique_opens" db:"unique_opens"`
	UniqueClicks            int       `json:"unique_clicks" db:"unique_clicks"`
	CreatedAt               time.Time `json:"created_at" db:"created_at"`
	UpdatedAt               time.Time `json:"updated_at" db:"updated_at"`
}

// GatewayHealthStatus represents real-time health status of a gateway
type GatewayHealthStatus struct {
	GatewayID                    string     `json:"gateway_id" db:"gateway_id"`
	GatewayType                  string     `json:"gateway_type" db:"gateway_type"`
	Status                       string     `json:"status" db:"status"` // HEALTHY, DEGRADED, UNHEALTHY, DISABLED
	LastHealthCheck              time.Time  `json:"last_health_check" db:"last_health_check"`
	ConsecutiveFailures          int        `json:"consecutive_failures" db:"consecutive_failures"`
	ConsecutiveSuccesses         int        `json:"consecutive_successes" db:"consecutive_successes"`
	LastError                    *string    `json:"last_error,omitempty" db:"last_error"`
	CircuitBreakerState          *string    `json:"circuit_breaker_state,omitempty" db:"circuit_breaker_state"`
	CircuitBreakerFailureCount   int        `json:"circuit_breaker_failure_count" db:"circuit_breaker_failure_count"`
	CircuitBreakerLastFailure    *time.Time `json:"circuit_breaker_last_failure,omitempty" db:"circuit_breaker_last_failure"`
	RateLimitRemaining           *int       `json:"rate_limit_remaining,omitempty" db:"rate_limit_remaining"`
	RateLimitResetTime           *time.Time `json:"rate_limit_reset_time,omitempty" db:"rate_limit_reset_time"`
	CreatedAt                    time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt                    time.Time  `json:"updated_at" db:"updated_at"`
}

// GatewayConfigAudit represents audit trail for gateway configuration changes
type GatewayConfigAudit struct {
	ID           int64                  `json:"id" db:"id"`
	GatewayID    string                 `json:"gateway_id" db:"gateway_id"`
	GatewayType  string                 `json:"gateway_type" db:"gateway_type"`
	ChangeType   string                 `json:"change_type" db:"change_type"` // CREATED, UPDATED, DELETED, ENABLED, DISABLED
	OldConfig    map[string]interface{} `json:"old_config,omitempty" db:"old_config"`
	NewConfig    map[string]interface{} `json:"new_config,omitempty" db:"new_config"`
	ChangedBy    *string                `json:"changed_by,omitempty" db:"changed_by"`
	ChangeReason *string                `json:"change_reason,omitempty" db:"change_reason"`
	CreatedAt    time.Time              `json:"created_at" db:"created_at"`
}

// GatewayPerformanceSummary represents aggregate performance metrics for a gateway
type GatewayPerformanceSummary struct {
	GatewayID      string  `json:"gateway_id" db:"gateway_id"`
	GatewayType    string  `json:"gateway_type" db:"gateway_type"`
	TotalMessages  int     `json:"total_messages" db:"total_messages"`
	SentCount      int     `json:"sent_count" db:"sent_count"`
	BouncedCount   int     `json:"bounced_count" db:"bounced_count"`
	FailedCount    int     `json:"failed_count" db:"failed_count"`
	DeferredCount  int     `json:"deferred_count" db:"deferred_count"`
	SuccessRate    float64 `json:"success_rate" db:"success_rate"`
	TotalOpens     int     `json:"total_opens" db:"total_opens"`
	TotalClicks    int     `json:"total_clicks" db:"total_clicks"`
	UniqueOpens    int     `json:"unique_opens" db:"unique_opens"`
	UniqueClicks   int     `json:"unique_clicks" db:"unique_clicks"`
}

// DailyGatewayStats represents daily statistics for a gateway
type DailyGatewayStats struct {
	GatewayID         string    `json:"gateway_id" db:"gateway_id"`
	GatewayType       string    `json:"gateway_type" db:"gateway_type"`
	StatDate          time.Time `json:"stat_date" db:"stat_date"`
	DailyMessages     int       `json:"daily_messages" db:"daily_messages"`
	DailySent         int       `json:"daily_sent" db:"daily_sent"`
	DailyBounced      int       `json:"daily_bounced" db:"daily_bounced"`
	DailyFailed       int       `json:"daily_failed" db:"daily_failed"`
	DailySuccessRate  float64   `json:"daily_success_rate" db:"daily_success_rate"`
}