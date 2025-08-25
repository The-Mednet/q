package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

// ProviderManagementAPI handles CRUD operations for provider configurations
type ProviderManagementAPI struct {
	db *sql.DB
}

// NewProviderManagementAPI creates a new provider management API instance
func NewProviderManagementAPI(db *sql.DB) *ProviderManagementAPI {
	if db == nil {
		log.Fatal("Database connection is required for ProviderManagementAPI")
	}
	return &ProviderManagementAPI{db: db}
}

// Database Models
type WorkspaceProvider struct {
	ID          int       `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	Type        string    `json:"type"`
	Enabled     bool      `json:"enabled"`
	Priority    int       `json:"priority"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Config      interface{} `json:"config,omitempty"`
}

type GmailProviderConfig struct {
	ID                int       `json:"id"`
	ProviderID        int       `json:"provider_id"`
	ServiceAccountFile string   `json:"service_account_file,omitempty"`
	ServiceAccountEnv  string   `json:"service_account_env,omitempty"`
	DefaultSender     string    `json:"default_sender"`
	DelegatedUser     *string   `json:"delegated_user,omitempty"`
	Scopes            []string  `json:"scopes"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type MailgunProviderConfig struct {
	ID         int       `json:"id"`
	ProviderID int       `json:"provider_id"`
	APIKey     string    `json:"api_key,omitempty"`
	APIKeyEnv  string    `json:"api_key_env,omitempty"`
	Domain     string    `json:"domain"`
	BaseURL    string    `json:"base_url"`
	TrackOpens bool      `json:"track_opens"`
	TrackClicks bool     `json:"track_clicks"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type MandrillProviderConfig struct {
	ID         int       `json:"id"`
	ProviderID int       `json:"provider_id"`
	APIKey     string    `json:"api_key,omitempty"`
	APIKeyEnv  string    `json:"api_key_env,omitempty"`
	BaseURL    string    `json:"base_url"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type WorkspaceRateLimit struct {
	WorkspaceID   string `json:"workspace_id"`
	Daily         int    `json:"daily"`
	Hourly        int    `json:"hourly"`
	PerUserDaily  int    `json:"per_user_daily"`
	PerUserHourly int    `json:"per_user_hourly"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type WorkspaceUserRateLimit struct {
	ID          int       `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	UserEmail   string    `json:"user_email"`
	Daily       int       `json:"daily"`
	Hourly      int       `json:"hourly"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type ProviderHeaderRewriteRule struct {
	ID         int       `json:"id"`
	ProviderID int       `json:"provider_id"`
	HeaderName string    `json:"header_name"`
	Action     string    `json:"action"` // add, replace, remove
	Value      *string   `json:"value,omitempty"`
	Condition  *string   `json:"condition,omitempty"`
	Enabled    bool      `json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Request/Response Models
type CreateProviderRequest struct {
	WorkspaceID string      `json:"workspace_id"`
	Type        string      `json:"type"`
	Enabled     bool        `json:"enabled"`
	Priority    int         `json:"priority"`
	Config      interface{} `json:"config"`
}

type UpdateProviderRequest struct {
	Enabled  bool        `json:"enabled"`
	Priority int         `json:"priority"`
	Config   interface{} `json:"config"`
}

type UpdateRateLimitsRequest struct {
	Daily         int `json:"daily"`
	Hourly        int `json:"hourly"`
	PerUserDaily  int `json:"per_user_daily"`
	PerUserHourly int `json:"per_user_hourly"`
}

type CreateUserRateLimitRequest struct {
	WorkspaceID string `json:"workspace_id"`
	UserEmail   string `json:"user_email"`
	Daily       int    `json:"daily"`
	Hourly      int    `json:"hourly"`
}

type CreateHeaderRuleRequest struct {
	ProviderID int     `json:"provider_id"`
	HeaderName string  `json:"header_name"`
	Action     string  `json:"action"`
	Value      *string `json:"value,omitempty"`
	Condition  *string `json:"condition,omitempty"`
	Enabled    bool    `json:"enabled"`
}

// RegisterRoutes registers all provider management routes
func (api *ProviderManagementAPI) RegisterRoutes(router *mux.Router) {
	// Provider CRUD endpoints
	router.HandleFunc("/api/workspaces/{id}/providers", api.ListWorkspaceProviders).Methods("GET")
	router.HandleFunc("/api/providers/{id}", api.GetProvider).Methods("GET")
	router.HandleFunc("/api/workspaces/{id}/providers", api.CreateProvider).Methods("POST")
	router.HandleFunc("/api/providers/{id}", api.UpdateProvider).Methods("PUT")
	router.HandleFunc("/api/providers/{id}", api.DeleteProvider).Methods("DELETE")
	
	// Rate limits endpoints
	router.HandleFunc("/api/workspaces/{id}/rate-limits", api.GetRateLimits).Methods("GET")
	router.HandleFunc("/api/workspaces/{id}/rate-limits", api.UpdateRateLimits).Methods("PUT")
	
	// User rate limits endpoints
	router.HandleFunc("/api/workspaces/{id}/user-rate-limits", api.ListUserRateLimits).Methods("GET")
	router.HandleFunc("/api/workspaces/{id}/user-rate-limits", api.CreateUserRateLimit).Methods("POST")
	router.HandleFunc("/api/user-rate-limits/{id}", api.DeleteUserRateLimit).Methods("DELETE")
	
	// Header rules endpoints
	router.HandleFunc("/api/providers/{id}/header-rules", api.ListHeaderRules).Methods("GET")
	router.HandleFunc("/api/providers/{id}/header-rules", api.CreateHeaderRule).Methods("POST")
	router.HandleFunc("/api/header-rules/{id}", api.DeleteHeaderRule).Methods("DELETE")
	
	log.Println("Provider management API routes registered successfully")
}

// Provider CRUD Operations
func (api *ProviderManagementAPI) ListWorkspaceProviders(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	workspaceID := vars["id"]
	
	// Defensive programming: validate workspace ID
	if err := validateWorkspaceID(workspaceID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	
	query := `
		SELECT id, workspace_id, provider_type, enabled, priority, created_at, updated_at
		FROM workspace_providers
		WHERE workspace_id = ?
		ORDER BY priority ASC, created_at DESC
	`
	
	rows, err := api.db.Query(query, workspaceID)
	if err != nil {
		log.Printf("Error querying workspace providers: %v", err)
		http.Error(w, "Failed to fetch providers", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	
	var providers []WorkspaceProvider
	for rows.Next() {
		var provider WorkspaceProvider
		err := rows.Scan(
			&provider.ID, &provider.WorkspaceID, &provider.Type,
			&provider.Enabled, &provider.Priority, &provider.CreatedAt, &provider.UpdatedAt,
		)
		if err != nil {
			log.Printf("Error scanning provider row: %v", err)
			http.Error(w, "Failed to process provider data", http.StatusInternalServerError)
			return
		}
		
		// Load provider-specific configuration
		if config, err := api.loadProviderConfig(provider.ID, provider.Type); err == nil {
			provider.Config = config
		} else {
			log.Printf("Warning: Failed to load config for provider %d (type: %s): %v", provider.ID, provider.Type, err)
		}
		
		providers = append(providers, provider)
	}
	
	if err = rows.Err(); err != nil {
		log.Printf("Error iterating provider rows: %v", err)
		http.Error(w, "Failed to fetch providers", http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(providers)
}

func (api *ProviderManagementAPI) GetProvider(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]
	
	// Defensive programming: validate provider ID
	providerID, err := validateAndParseProviderID(idStr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	
	query := `
		SELECT id, workspace_id, provider_type, enabled, priority, created_at, updated_at
		FROM workspace_providers
		WHERE id = ?
	`
	
	var provider WorkspaceProvider
	err = api.db.QueryRow(query, providerID).Scan(
		&provider.ID, &provider.WorkspaceID, &provider.Type,
		&provider.Enabled, &provider.Priority, &provider.CreatedAt, &provider.UpdatedAt,
	)
	
	if err == sql.ErrNoRows {
		http.Error(w, "Provider not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("Error fetching provider: %v", err)
		http.Error(w, "Failed to fetch provider", http.StatusInternalServerError)
		return
	}
	
	// Load provider-specific configuration
	if config, err := api.loadProviderConfig(provider.ID, provider.Type); err == nil {
		provider.Config = config
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(provider)
}

func (api *ProviderManagementAPI) CreateProvider(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	workspaceID := vars["id"]
	
	var req CreateProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	
	// Defensive programming: comprehensive input validation
	if err := validateWorkspaceID(workspaceID); err != nil {
		http.Error(w, fmt.Sprintf("Invalid workspace ID: %v", err), http.StatusBadRequest)
		return
	}
	
	if err := validateProviderType(req.Type); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	
	if err := validatePriority(req.Priority); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	
	req.WorkspaceID = workspaceID
	
	// Start transaction for atomic operation
	tx, err := api.db.Begin()
	if err != nil {
		log.Printf("Error starting transaction: %v", err)
		http.Error(w, "Failed to create provider", http.StatusInternalServerError)
		return
	}
	defer func() {
		if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
			log.Printf("Error rolling back transaction: %v", err)
		}
	}()
	
	// Create workspace provider record
	query := `
		INSERT INTO workspace_providers (workspace_id, provider_type, enabled, priority)
		VALUES (?, ?, ?, ?)
	`
	
	result, err := tx.Exec(query, req.WorkspaceID, req.Type, req.Enabled, req.Priority)
	if err != nil {
		log.Printf("Error creating provider: %v", err)
		http.Error(w, "Failed to create provider", http.StatusInternalServerError)
		return
	}
	
	providerID, err := result.LastInsertId()
	if err != nil {
		log.Printf("Error getting provider ID: %v", err)
		http.Error(w, "Failed to create provider", http.StatusInternalServerError)
		return
	}
	
	// Create provider-specific configuration
	if err := api.createProviderConfig(tx, int(providerID), req.Type, req.Config); err != nil {
		log.Printf("Error creating provider config: %v", err)
		http.Error(w, "Failed to create provider configuration", http.StatusInternalServerError)
		return
	}
	
	if err := tx.Commit(); err != nil {
		log.Printf("Error committing transaction: %v", err)
		http.Error(w, "Failed to create provider", http.StatusInternalServerError)
		return
	}
	
	// Return created provider
	provider := WorkspaceProvider{
		ID:          int(providerID),
		WorkspaceID: req.WorkspaceID,
		Type:        req.Type,
		Enabled:     req.Enabled,
		Priority:    req.Priority,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Config:      req.Config,
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(provider)
}

func (api *ProviderManagementAPI) UpdateProvider(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]
	
	providerID, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid provider ID", http.StatusBadRequest)
		return
	}
	
	var req UpdateProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	
	// Get current provider type for config update
	var providerType string
	err = api.db.QueryRow("SELECT provider_type FROM workspace_providers WHERE id = ?", providerID).Scan(&providerType)
	if err == sql.ErrNoRows {
		http.Error(w, "Provider not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("Error fetching provider type: %v", err)
		http.Error(w, "Failed to update provider", http.StatusInternalServerError)
		return
	}
	
	// Start transaction
	tx, err := api.db.Begin()
	if err != nil {
		log.Printf("Error starting transaction: %v", err)
		http.Error(w, "Failed to update provider", http.StatusInternalServerError)
		return
	}
	defer func() {
		if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
			log.Printf("Error rolling back transaction: %v", err)
		}
	}()
	
	// Update workspace provider
	query := `
		UPDATE workspace_providers 
		SET enabled = ?, priority = ?, updated_at = NOW()
		WHERE id = ?
	`
	
	result, err := tx.Exec(query, req.Enabled, req.Priority, providerID)
	if err != nil {
		log.Printf("Error updating provider: %v", err)
		http.Error(w, "Failed to update provider", http.StatusInternalServerError)
		return
	}
	
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, "Provider not found", http.StatusNotFound)
		return
	}
	
	// Update provider-specific configuration if provided
	if req.Config != nil {
		if err := api.updateProviderConfig(tx, providerID, providerType, req.Config); err != nil {
			log.Printf("Error updating provider config: %v", err)
			http.Error(w, "Failed to update provider configuration", http.StatusInternalServerError)
			return
		}
	}
	
	if err := tx.Commit(); err != nil {
		log.Printf("Error committing transaction: %v", err)
		http.Error(w, "Failed to update provider", http.StatusInternalServerError)
		return
	}
	
	// Return updated provider
	updatedProvider, err := api.getProviderByID(providerID)
	if err != nil {
		log.Printf("Error fetching updated provider: %v", err)
		http.Error(w, "Provider updated but failed to fetch result", http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updatedProvider)
}

func (api *ProviderManagementAPI) DeleteProvider(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]
	
	providerID, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid provider ID", http.StatusBadRequest)
		return
	}
	
	// Start transaction for cascading delete
	tx, err := api.db.Begin()
	if err != nil {
		log.Printf("Error starting transaction: %v", err)
		http.Error(w, "Failed to delete provider", http.StatusInternalServerError)
		return
	}
	defer func() {
		if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
			log.Printf("Error rolling back transaction: %v", err)
		}
	}()
	
	// Delete provider-specific configurations (cascading delete should handle this)
	// Delete header rewrite rules
	_, err = tx.Exec("DELETE FROM provider_header_rewrite_rules WHERE provider_id = ?", providerID)
	if err != nil {
		log.Printf("Error deleting header rules: %v", err)
		http.Error(w, "Failed to delete provider", http.StatusInternalServerError)
		return
	}
	
	// Delete provider configuration records
	_, err = tx.Exec("DELETE FROM gmail_provider_configs WHERE provider_id = ?", providerID)
	if err != nil {
		log.Printf("Error deleting Gmail config: %v", err)
	}
	
	_, err = tx.Exec("DELETE FROM mailgun_provider_configs WHERE provider_id = ?", providerID)
	if err != nil {
		log.Printf("Error deleting Mailgun config: %v", err)
	}
	
	_, err = tx.Exec("DELETE FROM mandrill_provider_configs WHERE provider_id = ?", providerID)
	if err != nil {
		log.Printf("Error deleting Mandrill config: %v", err)
	}
	
	// Delete main provider record
	result, err := tx.Exec("DELETE FROM workspace_providers WHERE id = ?", providerID)
	if err != nil {
		log.Printf("Error deleting provider: %v", err)
		http.Error(w, "Failed to delete provider", http.StatusInternalServerError)
		return
	}
	
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, "Provider not found", http.StatusNotFound)
		return
	}
	
	if err := tx.Commit(); err != nil {
		log.Printf("Error committing transaction: %v", err)
		http.Error(w, "Failed to delete provider", http.StatusInternalServerError)
		return
	}
	
	w.WriteHeader(http.StatusNoContent)
}

// Rate Limits Operations
func (api *ProviderManagementAPI) GetRateLimits(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	workspaceID := vars["id"]
	
	if workspaceID == "" {
		http.Error(w, "Workspace ID is required", http.StatusBadRequest)
		return
	}
	
	query := `
		SELECT workspace_id, workspace_daily, per_user_daily, burst_limit, created_at, updated_at
		FROM workspace_rate_limits
		WHERE workspace_id = ?
	`
	
	var rateLimit WorkspaceRateLimit
	var burstLimit sql.NullInt64
	err := api.db.QueryRow(query, workspaceID).Scan(
		&rateLimit.WorkspaceID, &rateLimit.Daily, &rateLimit.PerUserDaily,
		&burstLimit, &rateLimit.CreatedAt, &rateLimit.UpdatedAt,
	)
	// Set hourly to 0 since we don't have those columns
	rateLimit.Hourly = 0
	rateLimit.PerUserHourly = 0
	
	if err == sql.ErrNoRows {
		// Return default values if no rate limits configured
		rateLimit = WorkspaceRateLimit{
			WorkspaceID:   workspaceID,
			Daily:         1000,
			Hourly:        100,
			PerUserDaily:  100,
			PerUserHourly: 10,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
	} else if err != nil {
		log.Printf("Error fetching rate limits: %v", err)
		http.Error(w, "Failed to fetch rate limits", http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rateLimit)
}

func (api *ProviderManagementAPI) UpdateRateLimits(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	workspaceID := vars["id"]
	
	if workspaceID == "" {
		http.Error(w, "Workspace ID is required", http.StatusBadRequest)
		return
	}
	
	var req UpdateRateLimitsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	
	// Defensive programming: validate rate limits
	if err := validateRateLimit(req.Daily, "daily"); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	
	if err := validateRateLimit(req.Hourly, "hourly"); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	
	if err := validateRateLimit(req.PerUserDaily, "per-user daily"); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	
	if err := validateRateLimit(req.PerUserHourly, "per-user hourly"); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	
	query := `
		INSERT INTO workspace_rate_limits (workspace_id, daily, hourly, per_user_daily, per_user_hourly)
		VALUES (?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
		daily = VALUES(daily),
		hourly = VALUES(hourly),
		per_user_daily = VALUES(per_user_daily),
		per_user_hourly = VALUES(per_user_hourly),
		updated_at = NOW()
	`
	
	_, err := api.db.Exec(query, workspaceID, req.Daily, req.Hourly, req.PerUserDaily, req.PerUserHourly)
	if err != nil {
		log.Printf("Error updating rate limits: %v", err)
		http.Error(w, "Failed to update rate limits", http.StatusInternalServerError)
		return
	}
	
	// Return updated rate limits
	rateLimit := WorkspaceRateLimit{
		WorkspaceID:   workspaceID,
		Daily:         req.Daily,
		Hourly:        req.Hourly,
		PerUserDaily:  req.PerUserDaily,
		PerUserHourly: req.PerUserHourly,
		UpdatedAt:     time.Now(),
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rateLimit)
}

// User Rate Limits Operations
func (api *ProviderManagementAPI) ListUserRateLimits(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	workspaceID := vars["id"]
	
	if workspaceID == "" {
		http.Error(w, "Workspace ID is required", http.StatusBadRequest)
		return
	}
	
	query := `
		SELECT id, workspace_id, email_address, daily_limit, created_at, updated_at
		FROM workspace_user_rate_limits
		WHERE workspace_id = ?
		ORDER BY email_address ASC
	`
	
	rows, err := api.db.Query(query, workspaceID)
	if err != nil {
		log.Printf("Error querying user rate limits: %v", err)
		http.Error(w, "Failed to fetch user rate limits", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	
	var userRateLimits []WorkspaceUserRateLimit
	for rows.Next() {
		var userRateLimit WorkspaceUserRateLimit
		err := rows.Scan(
			&userRateLimit.ID, &userRateLimit.WorkspaceID, &userRateLimit.UserEmail,
			&userRateLimit.Daily, &userRateLimit.CreatedAt, &userRateLimit.UpdatedAt,
		)
		userRateLimit.Hourly = 0 // We don't have hourly in the table
		if err != nil {
			log.Printf("Error scanning user rate limit row: %v", err)
			http.Error(w, "Failed to process user rate limit data", http.StatusInternalServerError)
			return
		}
		userRateLimits = append(userRateLimits, userRateLimit)
	}
	
	if err = rows.Err(); err != nil {
		log.Printf("Error iterating user rate limit rows: %v", err)
		http.Error(w, "Failed to fetch user rate limits", http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(userRateLimits)
}

func (api *ProviderManagementAPI) CreateUserRateLimit(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	workspaceID := vars["id"]
	
	var req CreateUserRateLimitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	
	// Defensive programming: validate inputs
	if err := validateEmailAddress(req.UserEmail); err != nil {
		http.Error(w, fmt.Sprintf("Invalid user email: %v", err), http.StatusBadRequest)
		return
	}
	
	if err := validateRateLimit(req.Daily, "daily"); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	
	if err := validateRateLimit(req.Hourly, "hourly"); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	
	req.WorkspaceID = workspaceID
	
	query := `
		INSERT INTO workspace_user_rate_limits (workspace_id, user_email, daily, hourly)
		VALUES (?, ?, ?, ?)
	`
	
	result, err := api.db.Exec(query, req.WorkspaceID, req.UserEmail, req.Daily, req.Hourly)
	if err != nil {
		log.Printf("Error creating user rate limit: %v", err)
		http.Error(w, "Failed to create user rate limit", http.StatusInternalServerError)
		return
	}
	
	userRateLimitID, err := result.LastInsertId()
	if err != nil {
		log.Printf("Error getting user rate limit ID: %v", err)
		http.Error(w, "Failed to create user rate limit", http.StatusInternalServerError)
		return
	}
	
	userRateLimit := WorkspaceUserRateLimit{
		ID:          int(userRateLimitID),
		WorkspaceID: req.WorkspaceID,
		UserEmail:   req.UserEmail,
		Daily:       req.Daily,
		Hourly:      req.Hourly,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(userRateLimit)
}

func (api *ProviderManagementAPI) DeleteUserRateLimit(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]
	
	userRateLimitID, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid user rate limit ID", http.StatusBadRequest)
		return
	}
	
	query := `DELETE FROM workspace_user_rate_limits WHERE id = ?`
	result, err := api.db.Exec(query, userRateLimitID)
	if err != nil {
		log.Printf("Error deleting user rate limit: %v", err)
		http.Error(w, "Failed to delete user rate limit", http.StatusInternalServerError)
		return
	}
	
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, "User rate limit not found", http.StatusNotFound)
		return
	}
	
	w.WriteHeader(http.StatusNoContent)
}

// Header Rules Operations
func (api *ProviderManagementAPI) ListHeaderRules(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]
	
	providerID, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid provider ID", http.StatusBadRequest)
		return
	}
	
	query := `
		SELECT id, provider_id, header_name, action, value, condition, enabled, created_at, updated_at
		FROM provider_header_rewrite_rules
		WHERE provider_id = ?
		ORDER BY header_name ASC, created_at DESC
	`
	
	rows, err := api.db.Query(query, providerID)
	if err != nil {
		log.Printf("Error querying header rules: %v", err)
		http.Error(w, "Failed to fetch header rules", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	
	var headerRules []ProviderHeaderRewriteRule
	for rows.Next() {
		var rule ProviderHeaderRewriteRule
		err := rows.Scan(
			&rule.ID, &rule.ProviderID, &rule.HeaderName, &rule.Action,
			&rule.Value, &rule.Condition, &rule.Enabled,
			&rule.CreatedAt, &rule.UpdatedAt,
		)
		if err != nil {
			log.Printf("Error scanning header rule row: %v", err)
			http.Error(w, "Failed to process header rule data", http.StatusInternalServerError)
			return
		}
		headerRules = append(headerRules, rule)
	}
	
	if err = rows.Err(); err != nil {
		log.Printf("Error iterating header rule rows: %v", err)
		http.Error(w, "Failed to fetch header rules", http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(headerRules)
}

func (api *ProviderManagementAPI) CreateHeaderRule(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]
	
	providerID, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid provider ID", http.StatusBadRequest)
		return
	}
	
	var req CreateHeaderRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	
	// Validate required fields
	if req.HeaderName == "" || req.Action == "" {
		http.Error(w, "Header name and action are required", http.StatusBadRequest)
		return
	}
	
	// Validate action type
	validActions := map[string]bool{"add": true, "replace": true, "remove": true}
	if !validActions[req.Action] {
		http.Error(w, "Invalid action type", http.StatusBadRequest)
		return
	}
	
	// Validate that value is provided for add/replace actions
	if (req.Action == "add" || req.Action == "replace") && (req.Value == nil || *req.Value == "") {
		http.Error(w, "Value is required for add/replace actions", http.StatusBadRequest)
		return
	}
	
	req.ProviderID = providerID
	
	query := `
		INSERT INTO provider_header_rewrite_rules (provider_id, header_name, action, value, condition, enabled)
		VALUES (?, ?, ?, ?, ?, ?)
	`
	
	result, err := api.db.Exec(query, req.ProviderID, req.HeaderName, req.Action, req.Value, req.Condition, req.Enabled)
	if err != nil {
		log.Printf("Error creating header rule: %v", err)
		http.Error(w, "Failed to create header rule", http.StatusInternalServerError)
		return
	}
	
	ruleID, err := result.LastInsertId()
	if err != nil {
		log.Printf("Error getting header rule ID: %v", err)
		http.Error(w, "Failed to create header rule", http.StatusInternalServerError)
		return
	}
	
	headerRule := ProviderHeaderRewriteRule{
		ID:         int(ruleID),
		ProviderID: req.ProviderID,
		HeaderName: req.HeaderName,
		Action:     req.Action,
		Value:      req.Value,
		Condition:  req.Condition,
		Enabled:    req.Enabled,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(headerRule)
}

func (api *ProviderManagementAPI) DeleteHeaderRule(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]
	
	ruleID, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid header rule ID", http.StatusBadRequest)
		return
	}
	
	query := `DELETE FROM provider_header_rewrite_rules WHERE id = ?`
	result, err := api.db.Exec(query, ruleID)
	if err != nil {
		log.Printf("Error deleting header rule: %v", err)
		http.Error(w, "Failed to delete header rule", http.StatusInternalServerError)
		return
	}
	
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, "Header rule not found", http.StatusNotFound)
		return
	}
	
	w.WriteHeader(http.StatusNoContent)
}

// Helper methods
func (api *ProviderManagementAPI) loadProviderConfig(providerID int, providerType string) (interface{}, error) {
	switch providerType {
	case "gmail":
		return api.loadGmailConfig(providerID)
	case "mailgun":
		return api.loadMailgunConfig(providerID)
	case "mandrill":
		return api.loadMandrillConfig(providerID)
	default:
		return nil, fmt.Errorf("unknown provider type: %s", providerType)
	}
}

func (api *ProviderManagementAPI) loadGmailConfig(providerID int) (*GmailProviderConfig, error) {
	query := `
		SELECT provider_id, service_account_file, service_account_env, default_sender, impersonate_user, scopes, created_at, updated_at
		FROM gmail_provider_configs
		WHERE provider_id = ?
	`
	
	var config GmailProviderConfig
	var scopesJSON sql.NullString
	var serviceAccountFile sql.NullString
	var serviceAccountEnv sql.NullString
	var impersonateUser sql.NullString
	
	err := api.db.QueryRow(query, providerID).Scan(
		&config.ProviderID, &serviceAccountFile, &serviceAccountEnv, &config.DefaultSender,
		&impersonateUser, &scopesJSON, &config.CreatedAt, &config.UpdatedAt,
	)
	
	// Use impersonate_user as delegated_user for compatibility
	if impersonateUser.Valid {
		config.DelegatedUser = &impersonateUser.String
	}
	
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	
	// Handle nullable fields
	if serviceAccountFile.Valid {
		config.ServiceAccountFile = serviceAccountFile.String
	}
	if serviceAccountEnv.Valid {
		config.ServiceAccountEnv = serviceAccountEnv.String
	}
	
	if scopesJSON.Valid {
		json.Unmarshal([]byte(scopesJSON.String), &config.Scopes)
	}
	
	return &config, nil
}

func (api *ProviderManagementAPI) loadMailgunConfig(providerID int) (*MailgunProviderConfig, error) {
	query := `
		SELECT provider_id, api_key_env, domain, base_url, region, track_opens, track_clicks, track_unsubscribes, webhook_signing_key, created_at, updated_at
		FROM mailgun_provider_configs
		WHERE provider_id = ?
	`
	
	var config MailgunProviderConfig
	var apiKeyEnv sql.NullString
	var region sql.NullString
	var trackUnsubscribes sql.NullBool
	var webhookSigningKey sql.NullString
	
	err := api.db.QueryRow(query, providerID).Scan(
		&config.ProviderID, &apiKeyEnv, &config.Domain, &config.BaseURL, &region,
		&config.TrackOpens, &config.TrackClicks, &trackUnsubscribes, &webhookSigningKey, &config.CreatedAt, &config.UpdatedAt,
	)
	
	// Note: api_key_env contains the environment variable name, not the actual key
	// The actual key should be loaded from environment when needed
	if apiKeyEnv.Valid {
		config.APIKeyEnv = apiKeyEnv.String
		// Don't expose the actual key, just indicate it's configured
		config.APIKey = "[configured]"
	}
	
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	
	return &config, nil
}

func (api *ProviderManagementAPI) loadMandrillConfig(providerID int) (*MandrillProviderConfig, error) {
	query := `
		SELECT provider_id, api_key_env, subaccount, default_from_name, default_from_email, 
		       track_opens, track_clicks, default_tags, created_at, updated_at
		FROM mandrill_provider_configs
		WHERE provider_id = ?
	`
	
	var config MandrillProviderConfig
	var apiKeyEnv sql.NullString
	var subaccount sql.NullString
	var defaultFromName sql.NullString
	var defaultFromEmail string
	var trackOpens sql.NullBool
	var trackClicks sql.NullBool
	var defaultTags sql.NullString
	
	err := api.db.QueryRow(query, providerID).Scan(
		&config.ProviderID, &apiKeyEnv, &subaccount, &defaultFromName, &defaultFromEmail,
		&trackOpens, &trackClicks, &defaultTags, &config.CreatedAt, &config.UpdatedAt,
	)
	
	// Map fields to config struct
	if apiKeyEnv.Valid {
		config.APIKeyEnv = apiKeyEnv.String
		// Don't expose the actual key
		config.APIKey = ""
	}
	config.BaseURL = "https://mandrillapp.com/api/1.0" // Default Mandrill URL
	
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	
	return &config, nil
}

func (api *ProviderManagementAPI) createProviderConfig(tx *sql.Tx, providerID int, providerType string, config interface{}) error {
	switch providerType {
	case "gmail":
		return api.createGmailConfig(tx, providerID, config)
	case "mailgun":
		return api.createMailgunConfig(tx, providerID, config)
	case "mandrill":
		return api.createMandrillConfig(tx, providerID, config)
	default:
		return fmt.Errorf("unknown provider type: %s", providerType)
	}
}

func (api *ProviderManagementAPI) createGmailConfig(tx *sql.Tx, providerID int, config interface{}) error {
	configMap, ok := config.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid Gmail configuration format")
	}
	
	serviceAccountFile, _ := configMap["service_account_file"].(string)
	defaultSender, _ := configMap["default_sender"].(string)
	delegatedUser, _ := configMap["delegated_user"].(string)
	scopes, _ := configMap["scopes"].([]interface{})
	
	scopesJSON := "[]"
	if scopes != nil {
		if scopesBytes, err := json.Marshal(scopes); err == nil {
			scopesJSON = string(scopesBytes)
		}
	}
	
	query := `
		INSERT INTO gmail_provider_configs (provider_id, service_account_file, default_sender, delegated_user, scopes)
		VALUES (?, ?, ?, ?, ?)
	`
	
	var delegatedUserPtr *string
	if delegatedUser != "" {
		delegatedUserPtr = &delegatedUser
	}
	
	_, err := tx.Exec(query, providerID, serviceAccountFile, defaultSender, delegatedUserPtr, scopesJSON)
	return err
}

func (api *ProviderManagementAPI) createMailgunConfig(tx *sql.Tx, providerID int, config interface{}) error {
	configMap, ok := config.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid Mailgun configuration format")
	}
	
	apiKey, _ := configMap["api_key"].(string)
	domain, _ := configMap["domain"].(string)
	baseURL, _ := configMap["base_url"].(string)
	trackOpens, _ := configMap["track_opens"].(bool)
	trackClicks, _ := configMap["track_clicks"].(bool)
	
	if baseURL == "" {
		baseURL = "https://api.mailgun.net/v3"
	}
	
	query := `
		INSERT INTO mailgun_provider_configs (provider_id, api_key, domain, base_url, track_opens, track_clicks)
		VALUES (?, ?, ?, ?, ?, ?)
	`
	
	_, err := tx.Exec(query, providerID, apiKey, domain, baseURL, trackOpens, trackClicks)
	return err
}

func (api *ProviderManagementAPI) createMandrillConfig(tx *sql.Tx, providerID int, config interface{}) error {
	configMap, ok := config.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid Mandrill configuration format")
	}
	
	apiKey, _ := configMap["api_key"].(string)
	baseURL, _ := configMap["base_url"].(string)
	
	if baseURL == "" {
		baseURL = "https://mandrillapp.com/api/1.0"
	}
	
	query := `
		INSERT INTO mandrill_provider_configs (provider_id, api_key, base_url)
		VALUES (?, ?, ?)
	`
	
	_, err := tx.Exec(query, providerID, apiKey, baseURL)
	return err
}

func (api *ProviderManagementAPI) updateProviderConfig(tx *sql.Tx, providerID int, providerType string, config interface{}) error {
	switch providerType {
	case "gmail":
		return api.updateGmailConfig(tx, providerID, config)
	case "mailgun":
		return api.updateMailgunConfig(tx, providerID, config)
	case "mandrill":
		return api.updateMandrillConfig(tx, providerID, config)
	default:
		return fmt.Errorf("unknown provider type: %s", providerType)
	}
}

func (api *ProviderManagementAPI) updateGmailConfig(tx *sql.Tx, providerID int, config interface{}) error {
	configMap, ok := config.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid Gmail configuration format")
	}
	
	serviceAccountFile, _ := configMap["service_account_file"].(string)
	defaultSender, _ := configMap["default_sender"].(string)
	delegatedUser, _ := configMap["delegated_user"].(string)
	scopes, _ := configMap["scopes"].([]interface{})
	
	scopesJSON := "[]"
	if scopes != nil {
		if scopesBytes, err := json.Marshal(scopes); err == nil {
			scopesJSON = string(scopesBytes)
		}
	}
	
	query := `
		UPDATE gmail_provider_configs 
		SET service_account_file = ?, default_sender = ?, delegated_user = ?, scopes = ?, updated_at = NOW()
		WHERE provider_id = ?
	`
	
	var delegatedUserPtr *string
	if delegatedUser != "" {
		delegatedUserPtr = &delegatedUser
	}
	
	_, err := tx.Exec(query, serviceAccountFile, defaultSender, delegatedUserPtr, scopesJSON, providerID)
	return err
}

func (api *ProviderManagementAPI) updateMailgunConfig(tx *sql.Tx, providerID int, config interface{}) error {
	configMap, ok := config.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid Mailgun configuration format")
	}
	
	apiKey, _ := configMap["api_key"].(string)
	domain, _ := configMap["domain"].(string)
	baseURL, _ := configMap["base_url"].(string)
	trackOpens, _ := configMap["track_opens"].(bool)
	trackClicks, _ := configMap["track_clicks"].(bool)
	
	query := `
		UPDATE mailgun_provider_configs 
		SET api_key = ?, domain = ?, base_url = ?, track_opens = ?, track_clicks = ?, updated_at = NOW()
		WHERE provider_id = ?
	`
	
	_, err := tx.Exec(query, apiKey, domain, baseURL, trackOpens, trackClicks, providerID)
	return err
}

func (api *ProviderManagementAPI) updateMandrillConfig(tx *sql.Tx, providerID int, config interface{}) error {
	configMap, ok := config.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid Mandrill configuration format")
	}
	
	apiKey, _ := configMap["api_key"].(string)
	baseURL, _ := configMap["base_url"].(string)
	
	query := `
		UPDATE mandrill_provider_configs 
		SET api_key = ?, base_url = ?, updated_at = NOW()
		WHERE provider_id = ?
	`
	
	_, err := tx.Exec(query, apiKey, baseURL, providerID)
	return err
}

func (api *ProviderManagementAPI) getProviderByID(providerID int) (*WorkspaceProvider, error) {
	query := `
		SELECT id, workspace_id, provider_type, enabled, priority, created_at, updated_at
		FROM workspace_providers
		WHERE id = ?
	`
	
	var provider WorkspaceProvider
	err := api.db.QueryRow(query, providerID).Scan(
		&provider.ID, &provider.WorkspaceID, &provider.Type,
		&provider.Enabled, &provider.Priority, &provider.CreatedAt, &provider.UpdatedAt,
	)
	
	if err != nil {
		return nil, err
	}
	
	// Load provider-specific configuration
	if config, err := api.loadProviderConfig(provider.ID, provider.Type); err == nil {
		provider.Config = config
	}
	
	return &provider, nil
}

// Input validation helper functions for defensive programming

// validateWorkspaceID validates a workspace ID
func validateWorkspaceID(workspaceID string) error {
	if workspaceID == "" {
		return fmt.Errorf("workspace ID is required")
	}
	if len(workspaceID) > 255 {
		return fmt.Errorf("workspace ID too long (max 255 characters)")
	}
	// Additional validation: check for valid characters (alphanumeric, hyphens, underscores)
	for _, char := range workspaceID {
		if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || 
			 (char >= '0' && char <= '9') || char == '-' || char == '_' || char == '.') {
			return fmt.Errorf("workspace ID contains invalid characters (only alphanumeric, hyphens, underscores, and dots allowed)")
		}
	}
	return nil
}

// validateAndParseProviderID validates and parses a provider ID
func validateAndParseProviderID(idStr string) (int, error) {
	if idStr == "" {
		return 0, fmt.Errorf("provider ID is required")
	}
	
	providerID, err := strconv.Atoi(idStr)
	if err != nil {
		return 0, fmt.Errorf("provider ID must be a valid integer")
	}
	
	if providerID <= 0 {
		return 0, fmt.Errorf("provider ID must be positive")
	}
	
	return providerID, nil
}

// validateProviderType validates a provider type
func validateProviderType(providerType string) error {
	if providerType == "" {
		return fmt.Errorf("provider type is required")
	}
	
	validTypes := map[string]bool{
		"gmail":    true,
		"mailgun":  true,
		"mandrill": true,
	}
	
	if !validTypes[providerType] {
		return fmt.Errorf("invalid provider type (must be one of: gmail, mailgun, mandrill)")
	}
	
	return nil
}

// validatePriority validates a priority value
func validatePriority(priority int) error {
	if priority < 0 {
		return fmt.Errorf("priority must be non-negative")
	}
	
	if priority > 1000 {
		return fmt.Errorf("priority cannot exceed 1000")
	}
	
	return nil
}

// validateEmailAddress validates an email address format
func validateEmailAddress(email string) error {
	if email == "" {
		return fmt.Errorf("email address is required")
	}
	
	if len(email) > 320 { // RFC 5321 limit
		return fmt.Errorf("email address too long (max 320 characters)")
	}
	
	// Basic email format validation
	atIndex := strings.LastIndex(email, "@")
	if atIndex == -1 || atIndex == 0 || atIndex == len(email)-1 {
		return fmt.Errorf("invalid email format")
	}
	
	localPart := email[:atIndex]
	domain := email[atIndex+1:]
	
	if len(localPart) == 0 || len(localPart) > 64 {
		return fmt.Errorf("email local part invalid length")
	}
	
	if len(domain) == 0 || len(domain) > 253 {
		return fmt.Errorf("email domain invalid length")
	}
	
	// Check for consecutive dots
	if strings.Contains(email, "..") {
		return fmt.Errorf("email contains consecutive dots")
	}
	
	return nil
}

// validateRateLimit validates rate limit values
func validateRateLimit(limit int, limitType string) error {
	if limit <= 0 {
		return fmt.Errorf("%s rate limit must be positive", limitType)
	}
	
	// Set reasonable upper bounds to prevent abuse
	maxLimits := map[string]int{
		"daily":  100000,
		"hourly": 10000,
	}
	
	if maxLimit, exists := maxLimits[limitType]; exists && limit > maxLimit {
		return fmt.Errorf("%s rate limit cannot exceed %d", limitType, maxLimit)
	}
	
	return nil
}