package webui

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"

	"smtp_relay/internal/gmail"
	"smtp_relay/internal/processor"
	"smtp_relay/pkg/models"

	"github.com/gorilla/mux"
)

type QueueStats interface {
	GetStats() (map[string]interface{}, error)
	GetMessages(limit, offset int, status string) ([]*models.Message, error)
	Get(id string) (*models.Message, error)
	Remove(id string) error
}

type Server struct {
	queue       QueueStats
	router      *mux.Router
	gmailClient *gmail.Client
	processor   *processor.QueueProcessor
}

func NewServer(q QueueStats, gc *gmail.Client, p *processor.QueueProcessor) *Server {
	s := &Server{
		queue:       q,
		router:      mux.NewRouter(),
		gmailClient: gc,
		processor:   p,
	}

	s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() {
	s.router.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	s.router.HandleFunc("/", s.handleIndex).Methods("GET")
	s.router.HandleFunc("/api/messages", s.handleGetMessages).Methods("GET")
	s.router.HandleFunc("/api/messages/{id}", s.handleGetMessage).Methods("GET")
	s.router.HandleFunc("/api/messages/{id}", s.handleDeleteMessage).Methods("DELETE")
	s.router.HandleFunc("/api/stats", s.handleGetStats).Methods("GET")
	s.router.HandleFunc("/api/process", s.handleProcessQueue).Methods("POST")
	s.router.HandleFunc("/api/rate-limit", s.handleGetRateLimit).Methods("GET")
	s.router.HandleFunc("/validate", s.handleValidateServiceAccount).Methods("GET")
	s.router.HandleFunc("/webhook/test", s.handleWebhookTest).Methods("POST")
}

func (s *Server) Start(port int) error {
	addr := fmt.Sprintf(":%d", port)
	log.Printf("Starting Web UI server on http://localhost%s", addr)
	return http.ListenAndServe(addr, s.router)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	tmpl := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>SMTP Relay Queue Dashboard</title>
    <link rel="stylesheet" href="/static/style.css">
</head>
<body>
    <div class="container">
        <header>
            <h1>SMTP Relay Queue Dashboard</h1>
            <div class="stats" id="stats">
                <div class="stat-card">
                    <h3>Total</h3>
                    <p class="stat-value" id="total-count">-</p>
                </div>
                <div class="stat-card">
                    <h3>Queued</h3>
                    <p class="stat-value queued" id="queued-count">-</p>
                </div>
                <div class="stat-card">
                    <h3>Processing</h3>
                    <p class="stat-value processing" id="processing-count">-</p>
                </div>
                <div class="stat-card">
                    <h3>Sent</h3>
                    <p class="stat-value sent" id="sent-count">-</p>
                </div>
                <div class="stat-card">
                    <h3>Failed</h3>
                    <p class="stat-value failed" id="failed-count">-</p>
                </div>
            </div>
        </header>
        
        <main>
            <div class="controls">
                <select id="status-filter">
                    <option value="all">All Messages</option>
                    <option value="queued">Queued</option>
                    <option value="processing">Processing</option>
                    <option value="sent">Sent</option>
                    <option value="failed">Failed</option>
                </select>
                <button id="refresh-btn">Refresh</button>
                <button id="process-btn" onclick="processQueue()">Process Queue Now</button>
                <span id="process-status" class="process-status"></span>
            </div>
            
            <div id="rate-limit-info" class="rate-limit-container"></div>
            
            <div class="messages-table">
                <table>
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
                            <td colspan="7" class="loading">Loading...</td>
                        </tr>
                    </tbody>
                </table>
            </div>
            
            <div class="pagination">
                <button id="prev-btn" disabled>Previous</button>
                <span id="page-info">Page 1</span>
                <button id="next-btn">Next</button>
            </div>
        </main>
    </div>
    
    <div id="message-modal" class="modal">
        <div class="modal-content">
            <span class="close">&times;</span>
            <h2>Message Details</h2>
            <div id="message-details"></div>
        </div>
    </div>
    
    <script src="/static/app.js?v=4"></script>
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
		http.Error(w, "Queue processor not available", http.StatusServiceUnavailable)
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

	// Trigger processing in background
	go func() {
		err := s.processor.Process()
		if err != nil {
			log.Printf("Manual queue processing error: %v", err)
		}
	}()

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
