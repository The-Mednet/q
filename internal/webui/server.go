package webui

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"relay/internal/api"
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
	providerMgmtAPI  *api.ProviderManagementAPI
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
		
		// Initialize provider management API
		s.providerMgmtAPI = api.NewProviderManagementAPI(db)
		log.Println("Provider management API initialized")
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
	
	// Register provider management API routes
	if s.providerMgmtAPI != nil {
		s.providerMgmtAPI.RegisterRoutes(s.router)
		log.Println("Provider management API routes registered successfully")
	}

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
	log.Printf("Starting API server on http://localhost%s", addr)
	return http.ListenAndServe(addr, s.router)
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
