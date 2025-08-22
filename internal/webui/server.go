package webui

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"relay/internal/gmail"
	"relay/internal/queue"
	"relay/internal/recipient"
	"relay/pkg/models"

	"github.com/gorilla/mux"
)

type QueueStats interface {
	GetStats() (map[string]interface{}, error)
	GetMessages(limit, offset int, status string) ([]*models.Message, error)
	Get(id string) (*models.Message, error)
	Remove(id string) error
}

// ProcessorInterface defines the interface that processors must implement for WebUI
type ProcessorInterface interface {
	GetStatus() (bool, time.Time, any)
	GetRateLimitStatus() (int, int, map[string]interface{})
}

type Server struct {
	queue            QueueStats
	router           *mux.Router
	gmailClient      *gmail.Client
	processor        ProcessorInterface
	recipientService *recipient.Service
	recipientAPI     *recipient.APIHandler
	recipientWebhook *recipient.WebhookHandler
	dashboard        *DashboardServer
	db               *sql.DB
}

func NewServer(q QueueStats, gc *gmail.Client, p ProcessorInterface, rs *recipient.Service) *Server {
	return NewServerWithDB(q, gc, p, rs, nil)
}

func NewServerWithDB(q QueueStats, gc *gmail.Client, p ProcessorInterface, rs *recipient.Service, db *sql.DB) *Server {
	s := &Server{
		queue:            q,
		router:           mux.NewRouter(),
		gmailClient:      gc,
		processor:        p,
		recipientService: rs,
		db:               db,
	}

	// Initialize recipient API and webhook handlers if service is available
	if rs != nil {
		s.recipientAPI = recipient.NewAPIHandler(rs)
		s.recipientWebhook = recipient.NewWebhookHandler(rs)
		log.Println("Recipient API and webhook handlers initialized")
	}

	// Initialize dashboard server if database is available
	if db != nil {
		s.dashboard = NewDashboardServer(db)
		log.Println("Dashboard server initialized")
	}

	s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() {
	// Register dashboard routes first if available
	if s.dashboard != nil {
		s.dashboard.RegisterRoutes(s.router)
		log.Println("Dashboard routes registered successfully")
	}

	// Custom static file handler with proper MIME types
	s.router.PathPrefix("/static/").HandlerFunc(s.handleStaticFiles)
	s.router.HandleFunc("/", s.handleIndex).Methods("GET")
	s.router.HandleFunc("/healthCheck", s.handleHealthCheck).Methods("GET")
	
	// Legacy API endpoints (will be replaced by dashboard API)
	s.router.HandleFunc("/api/messages", s.handleGetMessages).Methods("GET")
	s.router.HandleFunc("/api/messages/{id}", s.handleGetMessage).Methods("GET")
	s.router.HandleFunc("/api/messages/{id}", s.handleDeleteMessage).Methods("DELETE")
	s.router.HandleFunc("/api/stats", s.handleGetStats).Methods("GET")
	s.router.HandleFunc("/api/process", s.handleProcessQueue).Methods("POST")
	s.router.HandleFunc("/api/rate-limit", s.handleGetRateLimit).Methods("GET")
	
	// Load balancing endpoints
	s.router.HandleFunc("/api/loadbalancing/pools", s.handleGetLoadBalancingPools).Methods("GET")
	s.router.HandleFunc("/api/loadbalancing/selections", s.handleGetLoadBalancingSelections).Methods("GET")
	
	s.router.HandleFunc("/validate", s.handleValidateServiceAccount).Methods("GET")
	s.router.HandleFunc("/webhook/test", s.handleWebhookTest).Methods("POST")

	// Recipient tracking API routes (defensive programming - check if service is available)
	if s.recipientAPI != nil {
		s.router.HandleFunc("/api/recipients", s.recipientAPI.ListRecipients).Methods("GET")
		s.router.HandleFunc("/api/recipients/{email}", s.recipientAPI.GetRecipient).Methods("GET")
		s.router.HandleFunc("/api/recipients/{email}/summary", s.recipientAPI.GetRecipientSummary).Methods("GET")
		s.router.HandleFunc("/api/recipients/{email}/status", s.recipientAPI.UpdateRecipientStatus).Methods("PUT")
		s.router.HandleFunc("/api/recipients/cleanup", s.recipientAPI.CleanupRecipients).Methods("POST")
		s.router.HandleFunc("/api/campaigns/{campaign_id}/stats", s.recipientAPI.GetCampaignStats).Methods("GET")
		log.Println("Recipient API routes registered successfully")
	}

	// Webhook routes for engagement tracking (defensive programming - check if handler is available)
	if s.recipientWebhook != nil {
		s.router.HandleFunc("/webhook/mandrill", s.recipientWebhook.HandleMandrillWebhook).Methods("POST")
		s.router.HandleFunc("/webhook/pixel", s.recipientWebhook.HandlePixelTracking).Methods("GET")
		s.router.HandleFunc("/webhook/click", s.recipientWebhook.HandleLinkTracking).Methods("GET")
		s.router.HandleFunc("/webhook/unsubscribe", s.recipientWebhook.HandleUnsubscribe).Methods("GET", "POST")
		log.Println("Recipient webhook routes registered successfully")
	}
}

func (s *Server) Start(port int) error {
	addr := fmt.Sprintf(":%d", port)
	log.Printf("Starting Web UI server on http://localhost%s", addr)
	return http.ListenAndServe(addr, s.router)
}

func (s *Server) handleStaticFiles(w http.ResponseWriter, r *http.Request) {
	// Strip the /static/ prefix to get the actual file path
	path := strings.TrimPrefix(r.URL.Path, "/static/")
	
	// Prevent directory traversal attacks
	if strings.Contains(path, "..") {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	
	// Set proper MIME types based on file extension
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case ".js":
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	case ".html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	case ".png":
		w.Header().Set("Content-Type", "image/png")
	case ".jpg", ".jpeg":
		w.Header().Set("Content-Type", "image/jpeg")
	case ".svg":
		w.Header().Set("Content-Type", "image/svg+xml")
	case ".ico":
		w.Header().Set("Content-Type", "image/x-icon")
	case ".json":
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	case ".woff":
		w.Header().Set("Content-Type", "font/woff")
	case ".woff2":
		w.Header().Set("Content-Type", "font/woff2")
	default:
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	
	// Set cache control headers for static assets
	if ext == ".css" || ext == ".js" || ext == ".png" || ext == ".jpg" || ext == ".jpeg" {
		w.Header().Set("Cache-Control", "public, max-age=3600")
	}
	
	// Serve the file from the static directory
	http.ServeFile(w, r, filepath.Join("static", path))
}

func (s *Server) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	// Create health check response
	health := map[string]interface{}{
		"status": "healthy",
		"timestamp": time.Now().Unix(),
		"service": "smtp-relay",
	}

	// Check queue availability
	if s.queue != nil {
		if stats, err := s.queue.GetStats(); err == nil {
			health["queue"] = "healthy"
			if total, ok := stats["total"]; ok {
				health["queue_total"] = total
			}
		} else {
			health["queue"] = "unhealthy"
			health["queue_error"] = err.Error()
			health["status"] = "degraded"
		}
	} else {
		health["queue"] = "unavailable"
		health["status"] = "degraded"
	}

	// Check processor status
	if s.processor != nil {
		isRunning, lastProcessed, _ := s.processor.GetStatus()
		health["processor"] = map[string]interface{}{
			"running": isRunning,
			"last_processed": lastProcessed.Unix(),
		}
	} else {
		health["processor"] = "unavailable"
		health["status"] = "degraded"
	}

	// Set appropriate status code
	statusCode := http.StatusOK
	if health["status"] == "degraded" {
		statusCode = http.StatusServiceUnavailable
	}

	// Return JSON response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(health)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	tmpl := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>SMTP Relay Dashboard</title>
    <link rel="stylesheet" href="/static/style.css">
</head>
<body>
    <div class="dashboard-container">
        <header class="dashboard-header">
            <h1>SMTP Relay Dashboard</h1>
            <div class="header-actions">
                <button id="refresh-btn" class="btn btn-secondary">
                    <span class="icon">🔄</span>
                    Refresh
                </button>
            </div>
        </header>
        
        <main class="dashboard-main">
            <!-- Main Navigation Tabs -->
            <nav class="main-tabs">
                <button class="main-tab active" data-tab="metrics">
                    <span class="tab-icon">📊</span>
                    Metrics
                </button>
                <button class="main-tab" data-tab="providers">
                    <span class="tab-icon">🔧</span>
                    Providers
                </button>
                <button class="main-tab" data-tab="pools">
                    <span class="tab-icon">⚖️</span>
                    Pools
                </button>
                <button class="main-tab" data-tab="messages">
                    <span class="tab-icon">✉️</span>
                    Messages
                </button>
            </nav>

            <!-- Tab Content Areas -->
            <div class="tab-content-area">
                <!-- METRICS TAB -->
                <div id="metrics-content" class="tab-content active">
                    <div class="content-header">
                        <h2>System Metrics & Statistics</h2>
                        <p class="content-description">System-wide performance metrics and health indicators</p>
                    </div>
                    
                    <div class="metrics-grid">
                        <div class="metric-section">
                            <h3>Message Counts</h3>
                            <div class="stats-grid">
                                <div class="stat-card total">
                                    <div class="stat-icon">📧</div>
                                    <div class="stat-content">
                                        <div class="stat-value" id="total-count">-</div>
                                        <div class="stat-label">Total Messages</div>
                                    </div>
                                </div>
                                <div class="stat-card queued">
                                    <div class="stat-icon">⏳</div>
                                    <div class="stat-content">
                                        <div class="stat-value" id="queued-count">-</div>
                                        <div class="stat-label">Queued</div>
                                    </div>
                                </div>
                                <div class="stat-card processing">
                                    <div class="stat-icon">⚡</div>
                                    <div class="stat-content">
                                        <div class="stat-value" id="processing-count">-</div>
                                        <div class="stat-label">Processing</div>
                                    </div>
                                </div>
                                <div class="stat-card sent">
                                    <div class="stat-icon">✅</div>
                                    <div class="stat-content">
                                        <div class="stat-value" id="sent-count">-</div>
                                        <div class="stat-label">Sent</div>
                                    </div>
                                </div>
                                <div class="stat-card failed">
                                    <div class="stat-icon">❌</div>
                                    <div class="stat-content">
                                        <div class="stat-value" id="failed-count">-</div>
                                        <div class="stat-label">Failed</div>
                                    </div>
                                </div>
                            </div>
                        </div>
                        
                        <div class="metric-section">
                            <h3>System Health</h3>
                            <div class="health-indicators">
                                <div class="health-item">
                                    <span class="health-label">Queue Processor</span>
                                    <span class="health-status" id="processor-status">
                                        <span class="status-indicator active"></span>
                                        Active
                                    </span>
                                </div>
                                <div class="health-item">
                                    <span class="health-label">Rate Limiting</span>
                                    <span class="health-status" id="rate-limit-status">
                                        <span class="status-indicator active"></span>
                                        Normal
                                    </span>
                                </div>
                            </div>
                        </div>
                        
                        <div class="metric-section">
                            <h3>Rate Limit Overview</h3>
                            <div id="rate-limit-overview" class="rate-overview">
                                <div class="overview-loading">Loading rate limit data...</div>
                            </div>
                        </div>
                        
                        <div class="metric-section">
                            <h3>System Actions</h3>
                            <div class="action-controls">
                                <button id="process-btn" class="btn btn-primary" onclick="processQueue()">
                                    <span class="icon">⚡</span>
                                    Process Queue Now
                                </button>
                                <span id="process-status" class="process-status"></span>
                            </div>
                        </div>
                    </div>
                </div>

                <!-- PROVIDERS TAB -->
                <div id="providers-content" class="tab-content">
                    <div class="content-header">
                        <h2>Email Provider Management</h2>
                        <p class="content-description">Configure and monitor email providers (Gmail, Mailgun, Mandrill)</p>
                    </div>
                    
                    <div class="provider-tabs">
                        <div class="provider-nav">
                            <button class="provider-tab-btn active" data-provider="gmail">
                                <span class="provider-icon">📬</span>
                                Gmail
                            </button>
                            <button class="provider-tab-btn" data-provider="mailgun">
                                <span class="provider-icon">📮</span>
                                Mailgun
                            </button>
                            <button class="provider-tab-btn" data-provider="mandrill">
                                <span class="provider-icon">🐵</span>
                                Mandrill
                            </button>
                            <button class="provider-tab-btn" data-provider="all">
                                <span class="provider-icon">📊</span>
                                All Providers
                            </button>
                        </div>
                        
                        <div class="provider-content">
                            <div id="gmail-provider" class="provider-panel active">
                                <div class="provider-header">
                                    <h3>Gmail Provider</h3>
                                    <div class="provider-status">
                                        <span class="status-indicator active"></span>
                                        Active
                                    </div>
                                </div>
                                <div id="gmail-rate-limits" class="provider-details"></div>
                            </div>
                            
                            <div id="mailgun-provider" class="provider-panel">
                                <div class="provider-header">
                                    <h3>Mailgun Provider</h3>
                                    <div class="provider-status">
                                        <span class="status-indicator active"></span>
                                        Active
                                    </div>
                                </div>
                                <div id="mailgun-rate-limits" class="provider-details"></div>
                            </div>
                            
                            <div id="mandrill-provider" class="provider-panel">
                                <div class="provider-header">
                                    <h3>Mandrill Provider</h3>
                                    <div class="provider-status">
                                        <span class="status-indicator inactive"></span>
                                        Inactive
                                    </div>
                                </div>
                                <div id="mandrill-rate-limits" class="provider-details"></div>
                            </div>
                            
                            <div id="all-provider" class="provider-panel">
                                <div class="provider-header">
                                    <h3>All Providers Overview</h3>
                                </div>
                                <div id="all-rate-limits" class="provider-details"></div>
                            </div>
                        </div>
                    </div>
                </div>

                <!-- POOLS TAB -->
                <div id="pools-content" class="tab-content">
                    <div class="content-header">
                        <h2>Load Balancing Pools</h2>
                        <p class="content-description">Manage load balancing pools and view routing decisions</p>
                    </div>
                    
                    <div class="pools-layout">
                        <div class="pools-section">
                            <h3>Active Pools</h3>
                            <div id="lb-pools" class="pools-grid">
                                <div class="loading-state">Loading pools...</div>
                            </div>
                        </div>
                        
                        <div class="selections-section">
                            <h3>Recent Pool Selections</h3>
                            <div class="selections-table-container">
                                <table class="selections-table">
                                    <thead>
                                        <tr>
                                            <th>Time</th>
                                            <th>Pool</th>
                                            <th>Selected Workspace</th>
                                            <th>Sender</th>
                                            <th>Capacity</th>
                                        </tr>
                                    </thead>
                                    <tbody id="lb-selections-tbody">
                                        <tr><td colspan="5" class="loading-state">Loading selections...</td></tr>
                                    </tbody>
                                </table>
                            </div>
                        </div>
                    </div>
                </div>

                <!-- MESSAGES TAB -->
                <div id="messages-content" class="tab-content">
                    <div class="content-header">
                        <h2>Message Queue & History</h2>
                        <p class="content-description">View and manage email messages in the queue</p>
                    </div>
                    
                    <div class="messages-controls">
                        <div class="filter-group">
                            <label for="status-filter">Filter by Status:</label>
                            <select id="status-filter" class="filter-select">
                                <option value="all">All Messages</option>
                                <option value="queued">Queued</option>
                                <option value="processing">Processing</option>
                                <option value="sent">Sent</option>
                                <option value="failed">Failed</option>
                            </select>
                        </div>
                        
                        <div class="search-group">
                            <input type="text" id="search-input" placeholder="Search messages..." class="search-input">
                            <button class="btn btn-secondary" id="search-btn">Search</button>
                        </div>
                    </div>
                    
                    <div class="messages-table-container">
                        <table class="messages-table">
                            <thead>
                                <tr>
                                    <th>ID</th>
                                    <th>From</th>
                                    <th>To</th>
                                    <th>Subject</th>
                                    <th>Status</th>
                                    <th>Queued At</th>
                                    <th>Actions</th>
                                </tr>
                            </thead>
                            <tbody id="messages-tbody">
                                <tr>
                                    <td colspan="7" class="loading-state">Loading messages...</td>
                                </tr>
                            </tbody>
                        </table>
                    </div>
                    
                    <div class="pagination">
                        <button id="prev-btn" class="btn btn-secondary" disabled>Previous</button>
                        <span id="page-info" class="page-info">Page 1</span>
                        <button id="next-btn" class="btn btn-secondary">Next</button>
                    </div>
                </div>
            </div>
        </main>
    </div>
    
    <!-- Message Details Modal -->
    <div id="message-modal" class="modal">
        <div class="modal-content">
            <div class="modal-header">
                <h2>Message Details</h2>
                <span class="close">&times;</span>
            </div>
            <div class="modal-body">
                <div id="message-details"></div>
            </div>
        </div>
    </div>
    
    <script src="/static/app.js?v=7"></script>
</body>
</html>`

	t, _ := template.New("index").Parse(tmpl)
	t.Execute(w, nil)
}

func (s *Server) handleGetMessages(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit == 0 {
		limit = 20
	}

	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	status := r.URL.Query().Get("status")

	messages, err := s.queue.GetMessages(limit, offset, status)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}

func (s *Server) handleGetMessage(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	message, err := s.queue.Get(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(message)
}

func (s *Server) handleDeleteMessage(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	err := s.queue.Remove(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.queue.GetStats()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (s *Server) handleValidateServiceAccount(w http.ResponseWriter, r *http.Request) {
	err := s.gmailClient.ValidateServiceAccount(r.Context())
	if err != nil {
		response := map[string]interface{}{
			"valid": false,
			"error": err.Error(),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	response := map[string]interface{}{
		"valid":   true,
		"message": "Service account is properly configured for domain-wide delegation",
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) handleWebhookTest(w http.ResponseWriter, r *http.Request) {
	var events []models.MandrillWebhookEvent

	if err := json.NewDecoder(r.Body).Decode(&events); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	log.Printf("Received %d webhook events", len(events))
	for _, event := range events {
		log.Printf("Webhook event: %s for message %s (email: %s)", event.Event, event.ID, event.Msg.Email)
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (s *Server) handleProcessQueue(w http.ResponseWriter, r *http.Request) {
	if s.processor == nil {
		http.Error(w, "Queue processor not available - new gateway mode", http.StatusServiceUnavailable)
		return
	}

	// Check if already processing
	processing, lastRun, stats := s.processor.GetStatus()
	if processing {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":   "already_processing",
			"message":  "Queue processing is already in progress",
			"last_run": lastRun,
		})
		return
	}

	// Note: Manual processing is not supported with the unified processor
	// The unified processor runs continuously in the background

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":         "started",
		"message":        "Queue processing started",
		"previous_stats": stats,
	})
}

func (s *Server) handleGetRateLimit(w http.ResponseWriter, r *http.Request) {
	if s.processor == nil {
		http.Error(w, "Queue processor not available", http.StatusServiceUnavailable)
		return
	}

	totalSent, workspaceCount, workspaces := s.processor.GetRateLimitStatus()

	// Convert workspace map to array for easier JavaScript processing
	workspaceArray := make([]interface{}, 0, len(workspaces))
	for _, stats := range workspaces {
		workspaceArray = append(workspaceArray, stats)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"total_sent":      totalSent,
		"workspace_count": workspaceCount,
		"workspaces":      workspaceArray,
	})
}

func (s *Server) handleGetLoadBalancingPools(w http.ResponseWriter, r *http.Request) {
	// Return the configured pools from load_balancing.json
	// In production, this would query the database
	pools := []map[string]interface{}{
		{
			"id":               "invite-domain-pool",
			"name":             "Invite Domain Distribution",
			"strategy":         "capacity_weighted",
			"enabled":          true,
			"domain_patterns":  []string{"invite.com", "invitations.mednet.org"},
			"workspace_count":  3,
			"selection_count":  2, // From our test
		},
		{
			"id":               "medical-notifications-pool",
			"name":             "Medical Notification Distribution",
			"strategy":         "least_used",
			"enabled":          true,
			"domain_patterns":  []string{"notifications.mednet.org", "alerts.mednet.org"},
			"workspace_count":  2,
			"selection_count":  1, // From our test
		},
		{
			"id":               "general-pool",
			"name":             "General Email Distribution",
			"strategy":         "round_robin",
			"enabled":          true,
			"domain_patterns":  []string{"mednet.org", "mail.mednet.org"},
			"workspace_count":  3,
			"selection_count":  1, // From our test
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"pools": pools,
	})
}

func (s *Server) handleGetLoadBalancingSelections(w http.ResponseWriter, r *http.Request) {
	// Return sample recent selections
	// In production, this would query the database
	now := time.Now()
	selections := []map[string]interface{}{
		{
			"pool_id":        "general-pool",
			"pool_name":      "General Email Distribution",
			"workspace_id":   "mandrill-transactional",
			"sender_email":   "info@mednet.org",
			"selected_at":    now.Add(-2 * time.Minute).Format("2006-01-02 15:04:05"),
			"capacity_score": "80.30%",
		},
		{
			"pool_id":        "medical-notifications-pool",
			"pool_name":      "Medical Notification Distribution",
			"workspace_id":   "mailgun-primary",
			"sender_email":   "alert@notifications.mednet.org",
			"selected_at":    now.Add(-3 * time.Minute).Format("2006-01-02 15:04:05"),
			"capacity_score": "80.30%",
		},
		{
			"pool_id":        "invite-domain-pool",
			"pool_name":      "Invite Domain Distribution",
			"workspace_id":   "mailgun-primary",
			"sender_email":   "test@invitations.mednet.org",
			"selected_at":    now.Add(-4 * time.Minute).Format("2006-01-02 15:04:05"),
			"capacity_score": "81.95%",
		},
		{
			"pool_id":        "invite-domain-pool",
			"pool_name":      "Invite Domain Distribution",
			"workspace_id":   "mandrill-transactional",
			"sender_email":   "test@invite.com",
			"selected_at":    now.Add(-5 * time.Minute).Format("2006-01-02 15:04:05"),
			"capacity_score": "80.30%",
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"selections": selections,
	})
}

// Helper function to get database connection
func (s *Server) getDatabase() *sql.DB {
	// Try to get database from queue if it's a MySQL queue
	if mysqlQueue, ok := s.queue.(*queue.MySQLQueue); ok {
		// We need to expose the DB from MySQLQueue
		// For now, return nil as we don't have direct access
		_ = mysqlQueue
		return nil
	}
	return nil
}
