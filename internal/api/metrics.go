package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

type MetricsAPI struct {
	db *sql.DB
}

func NewMetricsAPI(db *sql.DB) *MetricsAPI {
	return &MetricsAPI{db: db}
}

type StatsResponse struct {
	TotalMessages      int64          `json:"total_messages"`
	MessagesQueued     int64          `json:"messages_queued"`
	MessagesProcessing int64          `json:"messages_processing"`
	MessagesSent       int64          `json:"messages_sent"`
	MessagesFailed     int64          `json:"messages_failed"`
	MessagesToday      int64          `json:"messages_today"`
	SuccessRate        float64        `json:"success_rate"`
	HourlyStats        []HourlyStat   `json:"hourly_stats"`
	ProviderStats      []ProviderStat `json:"provider_stats"`
}

type HourlyStat struct {
	Hour              string  `json:"hour"`
	Sent              int64   `json:"sent"`
	Failed            int64   `json:"failed"`
	Queued            int64   `json:"queued"`
	AvgProcessingTime float64 `json:"avg_processing_time"`
}

type ProviderStat struct {
	Provider string `json:"provider"`
	Sent     int64  `json:"sent"`
	Failed   int64  `json:"failed"`
}

type RateLimitsResponse struct {
	WorkspaceLimits []WorkspaceLimit `json:"workspace_limits"`
	UserLimits      []UserLimit      `json:"user_limits"`
}

type WorkspaceLimit struct {
	ProviderID string `json:"provider_id"`
	Used        int64  `json:"used"`
	Limit       int64  `json:"limit"`
	ResetAt     string `json:"reset_at"`
}

type UserLimit struct {
	Email   string `json:"email"`
	Used    int64  `json:"used"`
	Limit   int64  `json:"limit"`
	ResetAt string `json:"reset_at"`
}

type HealthResponse struct {
	Healthy        bool             `json:"healthy"`
	ProviderStatus []ProviderHealth `json:"provider_status"`
	Errors         []string         `json:"errors,omitempty"`
}

type ProviderHealth struct {
	Name    string `json:"name"`
	Healthy bool   `json:"healthy"`
	Error   string `json:"error,omitempty"`
}

func (api *MetricsAPI) RegisterRoutes(router *mux.Router) {
	router.HandleFunc("/api/stats", api.GetStats).Methods("GET")
	router.HandleFunc("/api/rate-limits", api.GetRateLimits).Methods("GET")
	router.HandleFunc("/api/health", api.GetHealth).Methods("GET")
}

func (api *MetricsAPI) GetStats(w http.ResponseWriter, r *http.Request) {
	stats := StatsResponse{}

	// Get message counts by status
	statusQuery := `
		SELECT status, COUNT(*) 
		FROM messages 
		GROUP BY status
	`
	rows, err := api.db.Query(statusQuery)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var status string
			var count int64
			if err := rows.Scan(&status, &count); err == nil {
				switch status {
				case "queued":
					stats.MessagesQueued = count
				case "processing":
					stats.MessagesProcessing = count
				case "sent":
					stats.MessagesSent = count
				case "failed", "auth_error":
					stats.MessagesFailed += count
				}
			}
		}
	}

	stats.TotalMessages = stats.MessagesQueued + stats.MessagesProcessing + stats.MessagesSent + stats.MessagesFailed

	// Calculate success rate
	if stats.MessagesSent+stats.MessagesFailed > 0 {
		stats.SuccessRate = float64(stats.MessagesSent) / float64(stats.MessagesSent+stats.MessagesFailed)
	}

	// Get today's message count
	todayQuery := `
		SELECT COUNT(*) 
		FROM messages 
		WHERE DATE(queued_at) = CURDATE()
	`
	api.db.QueryRow(todayQuery).Scan(&stats.MessagesToday)

	// Get hourly stats for the last 24 hours
	hourlyQuery := `
		SELECT 
			DATE_FORMAT(queued_at, '%H:00') as hour,
			SUM(CASE WHEN status = 'sent' THEN 1 ELSE 0 END) as sent,
			SUM(CASE WHEN status IN ('failed', 'auth_error') THEN 1 ELSE 0 END) as failed,
			SUM(CASE WHEN status = 'queued' THEN 1 ELSE 0 END) as queued,
			AVG(TIMESTAMPDIFF(MICROSECOND, queued_at, sent_at) / 1000) as avg_processing_time
		FROM messages
		WHERE queued_at >= NOW() - INTERVAL 24 HOUR
		GROUP BY DATE_FORMAT(queued_at, '%Y-%m-%d %H')
		ORDER BY MIN(queued_at)
	`
	rows, err = api.db.Query(hourlyQuery)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var stat HourlyStat
			var avgTime sql.NullFloat64
			if err := rows.Scan(&stat.Hour, &stat.Sent, &stat.Failed, &stat.Queued, &avgTime); err == nil {
				if avgTime.Valid {
					stat.AvgProcessingTime = avgTime.Float64
				}
				stats.HourlyStats = append(stats.HourlyStats, stat)
			}
		}
	}

	// Get provider stats
	providerQuery := `
		SELECT 
			COALESCE(provider_id, 'unassigned') as provider,
			SUM(CASE WHEN status = 'sent' THEN 1 ELSE 0 END) as sent,
			SUM(CASE WHEN status IN ('failed', 'auth_error') THEN 1 ELSE 0 END) as failed
		FROM messages
		WHERE queued_at >= NOW() - INTERVAL 24 HOUR
		GROUP BY provider_id
	`
	rows, err = api.db.Query(providerQuery)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var stat ProviderStat
			if err := rows.Scan(&stat.Provider, &stat.Sent, &stat.Failed); err == nil {
				stats.ProviderStats = append(stats.ProviderStats, stat)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (api *MetricsAPI) GetRateLimits(w http.ResponseWriter, r *http.Request) {
	response := RateLimitsResponse{}
	today := time.Now().Format("2006-01-02")

	// Get workspace rate limits
	workspaceQuery := `
		SELECT 
			w.id,
			w.rate_limit_workspace_daily,
			COALESCE(SUM(r.message_count), 0) as used
		FROM providers w
		LEFT JOIN rate_limit_usage r ON w.id = r.provider_id 
			AND r.date_bucket = ? AND r.user_email IS NULL
		WHERE w.enabled = true
		GROUP BY w.id, w.rate_limit_workspace_daily
	`
	rows, err := api.db.Query(workspaceQuery, today)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var limit WorkspaceLimit
			if err := rows.Scan(&limit.ProviderID, &limit.Limit, &limit.Used); err == nil {
				limit.ResetAt = time.Now().Add(24 * time.Hour).Format(time.RFC3339)
				response.WorkspaceLimits = append(response.WorkspaceLimits, limit)
			}
		}
	}

	// Get user rate limits
	userQuery := `
		SELECT 
			r.user_email,
			w.rate_limit_per_user_daily,
			COALESCE(SUM(r.message_count), 0) as used
		FROM rate_limit_usage r
		JOIN workspaces w ON r.provider_id = w.id
		WHERE r.date_bucket = ? AND r.user_email IS NOT NULL
		GROUP BY r.user_email, w.rate_limit_per_user_daily
		LIMIT 50
	`
	rows, err = api.db.Query(userQuery, today)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var limit UserLimit
			if err := rows.Scan(&limit.Email, &limit.Limit, &limit.Used); err == nil {
				limit.ResetAt = time.Now().Add(24 * time.Hour).Format(time.RFC3339)
				response.UserLimits = append(response.UserLimits, limit)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (api *MetricsAPI) GetHealth(w http.ResponseWriter, r *http.Request) {
	response := HealthResponse{
		Healthy:        true,
		ProviderStatus: []ProviderHealth{},
		Errors:         []string{},
	}

	// Check database connectivity
	if err := api.db.Ping(); err != nil {
		response.Healthy = false
		response.Errors = append(response.Errors, "Database connection failed")
	}

	// Check provider health
	providerQuery := `
		SELECT w.id, w.display_name, ph.healthy, ph.error_message
		FROM providers w
		LEFT JOIN provider_health ph ON w.id = ph.provider_id
		WHERE w.enabled = true
	`
	rows, err := api.db.Query(providerQuery)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id, name string
			var healthy sql.NullBool
			var errorMsg sql.NullString

			if err := rows.Scan(&id, &name, &healthy, &errorMsg); err == nil {
				status := ProviderHealth{
					Name:    name,
					Healthy: healthy.Valid && healthy.Bool,
				}
				if errorMsg.Valid {
					status.Error = errorMsg.String
				}
				if !status.Healthy {
					response.Healthy = false
				}
				response.ProviderStatus = append(response.ProviderStatus, status)
			}
		}
	}

	// Check queue health (messages stuck in processing for too long)
	var stuckCount int
	stuckQuery := `
		SELECT COUNT(*) 
		FROM messages 
		WHERE status = 'processing' 
		AND processed_at < NOW() - INTERVAL 10 MINUTE
	`
	api.db.QueryRow(stuckQuery).Scan(&stuckCount)
	if stuckCount > 0 {
		response.Healthy = false
		response.Errors = append(response.Errors, "Messages stuck in processing state")
	}

	w.Header().Set("Content-Type", "application/json")
	if !response.Healthy {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	json.NewEncoder(w).Encode(response)
}

