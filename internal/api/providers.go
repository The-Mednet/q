package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

type ProvidersAPI struct {
	db *sql.DB
}

func NewProvidersAPI(db *sql.DB) *ProvidersAPI {
	return &ProvidersAPI{db: db}
}

type WorkspaceResponse struct {
	ID          string           `json:"id"`
	DisplayName string           `json:"display_name"`
	Domain      string           `json:"domain"`
	RateLimits  RateLimitsConfig  `json:"rate_limits"`
	Gmail       *GmailConfig      `json:"gmail,omitempty"`
	Mailgun     *MailgunConfig    `json:"mailgun,omitempty"`
	Mandrill    *MandrillConfig   `json:"mandrill,omitempty"`
	Enabled     bool              `json:"enabled"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

type RateLimitsConfig struct {
	WorkspaceDaily   int            `json:"workspace_daily"`
	PerUserDaily     int            `json:"per_user_daily"`
	CustomUserLimits map[string]int `json:"custom_user_limits,omitempty"`
}

type GmailConfig struct {
	ServiceAccountFile string `json:"service_account_file"`
	HasCredentials     bool   `json:"has_credentials"`
	Enabled            bool   `json:"enabled"`
	DefaultSender      string `json:"default_sender"`
}

type MailgunConfig struct {
	APIKey   string          `json:"api_key"`
	Domain   string          `json:"domain"`
	BaseURL  string          `json:"base_url"`
	Enabled  bool            `json:"enabled"`
	Tracking *TrackingConfig `json:"tracking,omitempty"`
}

type TrackingConfig struct {
	Opens  bool `json:"opens"`
	Clicks bool `json:"clicks"`
}

type MandrillConfig struct {
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"`
	Enabled bool   `json:"enabled"`
}

func (api *ProvidersAPI) RegisterRoutes(router *mux.Router) {
	router.HandleFunc("/api/workspaces", api.ListWorkspaces).Methods("GET")
	router.HandleFunc("/api/workspaces", api.CreateWorkspace).Methods("POST")
	router.HandleFunc("/api/workspaces/{id}", api.GetWorkspace).Methods("GET")
	router.HandleFunc("/api/workspaces/{id}", api.UpdateWorkspace).Methods("PUT")
	router.HandleFunc("/api/workspaces/{id}", api.DeleteWorkspace).Methods("DELETE")
	router.HandleFunc("/api/workspaces/{id}/credentials", api.UploadCredentials).Methods("POST")
}

func (api *ProvidersAPI) ListWorkspaces(w http.ResponseWriter, r *http.Request) {
	log.Println("DEBUG: ListWorkspaces called")
	query := `
		SELECT id, display_name, domain, rate_limit_workspace_daily, 
		       rate_limit_per_user_daily, rate_limit_custom_users,
		       provider_type, provider_config, enabled, created_at, updated_at,
		       CASE WHEN service_account_json IS NOT NULL AND service_account_json != '' THEN 1 ELSE 0 END as has_credentials
		FROM workspaces
		ORDER BY created_at DESC
	`

	rows, err := api.db.Query(query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var workspaces []WorkspaceResponse
	for rows.Next() {
		var ws WorkspaceResponse
		var providerType string
		var providerConfig json.RawMessage
		var customLimits sql.NullString
		var hasCredentials int

		err := rows.Scan(
			&ws.ID, &ws.DisplayName, &ws.Domain,
			&ws.RateLimits.WorkspaceDaily, &ws.RateLimits.PerUserDaily,
			&customLimits, &providerType, &providerConfig,
			&ws.Enabled, &ws.CreatedAt, &ws.UpdatedAt, &hasCredentials,
		)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if customLimits.Valid {
			json.Unmarshal([]byte(customLimits.String), &ws.RateLimits.CustomUserLimits)
		}

		switch providerType {
		case "gmail":
			var gmail GmailConfig
			json.Unmarshal(providerConfig, &gmail)
			// Always set service_account_file to empty string if not set
			if gmail.ServiceAccountFile == "" {
				gmail.ServiceAccountFile = ""
			}
			gmail.HasCredentials = hasCredentials == 1
			// Debug logging
			log.Printf("DEBUG: Setting HasCredentials for %s to %v (hasCredentials=%d)", ws.ID, gmail.HasCredentials, hasCredentials)
			ws.Gmail = &gmail
		case "mailgun":
			var mailgun MailgunConfig
			json.Unmarshal(providerConfig, &mailgun)
			ws.Mailgun = &mailgun
		case "mandrill":
			var mandrill MandrillConfig
			json.Unmarshal(providerConfig, &mandrill)
			ws.Mandrill = &mandrill
		}

		workspaces = append(workspaces, ws)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(workspaces)
}

func (api *ProvidersAPI) CreateWorkspace(w http.ResponseWriter, r *http.Request) {
	var req WorkspaceResponse
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.ID == "" {
		req.ID = uuid.NewString()
	}

	var providerType string
	var providerConfig []byte
	var err error

	if req.Gmail != nil {
		providerType = "gmail"
		providerConfig, err = json.Marshal(req.Gmail)
	} else if req.Mailgun != nil {
		providerType = "mailgun"
		providerConfig, err = json.Marshal(req.Mailgun)
	} else {
		http.Error(w, "Provider configuration required", http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	customLimits, _ := json.Marshal(req.RateLimits.CustomUserLimits)

	query := `
		INSERT INTO workspaces (
			id, display_name, domain, rate_limit_workspace_daily,
			rate_limit_per_user_daily, rate_limit_custom_users,
			provider_type, provider_config, enabled
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = api.db.Exec(query,
		req.ID, req.DisplayName, req.Domain,
		req.RateLimits.WorkspaceDaily, req.RateLimits.PerUserDaily,
		string(customLimits), providerType, string(providerConfig), req.Enabled,
	)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	req.CreatedAt = time.Now()
	req.UpdatedAt = time.Now()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(req)
}

func (api *ProvidersAPI) GetWorkspace(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	query := `
		SELECT id, display_name, domain, rate_limit_workspace_daily, 
		       rate_limit_per_user_daily, rate_limit_custom_users,
		       provider_type, provider_config, enabled, created_at, updated_at,
		       CASE WHEN service_account_json IS NOT NULL AND service_account_json != '' THEN 1 ELSE 0 END as has_credentials
		FROM workspaces
		WHERE id = ?
	`

	var ws WorkspaceResponse
	var providerType string
	var providerConfig json.RawMessage
	var customLimits sql.NullString
	var hasCredentials int

	err := api.db.QueryRow(query, id).Scan(
		&ws.ID, &ws.DisplayName, &ws.Domain,
		&ws.RateLimits.WorkspaceDaily, &ws.RateLimits.PerUserDaily,
		&customLimits, &providerType, &providerConfig,
		&ws.Enabled, &ws.CreatedAt, &ws.UpdatedAt, &hasCredentials,
	)

	if err == sql.ErrNoRows {
		http.Error(w, "Workspace not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if customLimits.Valid {
		json.Unmarshal([]byte(customLimits.String), &ws.RateLimits.CustomUserLimits)
	}

	switch providerType {
	case "gmail":
		var gmail GmailConfig
		json.Unmarshal(providerConfig, &gmail)
		gmail.HasCredentials = hasCredentials == 1
		ws.Gmail = &gmail
	case "mailgun":
		var mailgun MailgunConfig
		json.Unmarshal(providerConfig, &mailgun)
		ws.Mailgun = &mailgun
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ws)
}

func (api *ProvidersAPI) UpdateWorkspace(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var req WorkspaceResponse
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var providerType string
	var providerConfig []byte
	var err error

	if req.Gmail != nil {
		providerType = "gmail"
		providerConfig, err = json.Marshal(req.Gmail)
	} else if req.Mailgun != nil {
		providerType = "mailgun"
		providerConfig, err = json.Marshal(req.Mailgun)
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	customLimits, _ := json.Marshal(req.RateLimits.CustomUserLimits)

	query := `
		UPDATE workspaces SET
			display_name = ?, domain = ?, rate_limit_workspace_daily = ?,
			rate_limit_per_user_daily = ?, rate_limit_custom_users = ?,
			provider_type = ?, provider_config = ?, enabled = ?,
			updated_at = NOW()
		WHERE id = ?
	`

	result, err := api.db.Exec(query,
		req.DisplayName, req.Domain,
		req.RateLimits.WorkspaceDaily, req.RateLimits.PerUserDaily,
		string(customLimits), providerType, string(providerConfig), req.Enabled,
		id,
	)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, "Workspace not found", http.StatusNotFound)
		return
	}

	req.ID = id
	req.UpdatedAt = time.Now()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(req)
}

func (api *ProvidersAPI) DeleteWorkspace(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	query := `DELETE FROM workspaces WHERE id = ?`
	result, err := api.db.Exec(query, id)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, "Workspace not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (api *ProvidersAPI) UploadCredentials(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	// Parse multipart form with max 10MB for file
	err := r.ParseMultipartForm(10 << 20)
	if err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	// Get the uploaded file
	file, header, err := r.FormFile("credentials")
	if err != nil {
		http.Error(w, "Failed to get credentials file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Read the file content
	fileBytes, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "Failed to read file", http.StatusInternalServerError)
		return
	}

	// Validate it's valid JSON
	var credentialsJSON map[string]interface{}
	if err := json.Unmarshal(fileBytes, &credentialsJSON); err != nil {
		http.Error(w, "Invalid JSON in credentials file", http.StatusBadRequest)
		return
	}

	// Validate required fields for Gmail service account
	requiredFields := []string{"type", "project_id", "private_key_id", "private_key", "client_email"}
	for _, field := range requiredFields {
		if _, ok := credentialsJSON[field]; !ok {
			http.Error(w, fmt.Sprintf("Missing required field: %s", field), http.StatusBadRequest)
			return
		}
	}

	// Check if this is a service account
	if credentialsJSON["type"] != "service_account" {
		http.Error(w, "Credentials must be for a service account", http.StatusBadRequest)
		return
	}

	// Update the workspace with the new credentials
	query := `
		UPDATE workspaces 
		SET service_account_json = ?,
		    credentials_updated_at = NOW(),
		    provider_config = JSON_SET(COALESCE(provider_config, '{}'), 
		                              '$.service_account_file', NULL,
		                              '$.has_credentials', true)
		WHERE id = ? AND provider_type = 'gmail'
	`

	result, err := api.db.Exec(query, string(fileBytes), id)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to update credentials: %v", err), http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, "Workspace not found or not a Gmail provider", http.StatusNotFound)
		return
	}

	// Log the credential upload
	auditQuery := `
		INSERT INTO credential_audit_log (workspace_id, action, performed_by, details)
		VALUES (?, 'uploaded', ?, ?)
	`
	auditDetails := fmt.Sprintf(`{"filename": "%s", "size": %d}`, header.Filename, header.Size)
	api.db.Exec(auditQuery, id, "dashboard_user", auditDetails)

	// Return success response
	response := map[string]interface{}{
		"success": true,
		"message": "Credentials uploaded successfully",
		"filename": header.Filename,
		"workspace_id": id,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

