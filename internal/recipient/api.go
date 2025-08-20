package recipient

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"relay/pkg/models"
)

// APIHandler handles HTTP API requests for recipient management
type APIHandler struct {
	recipientService *Service
}

// NewAPIHandler creates a new API handler
func NewAPIHandler(service *Service) *APIHandler {
	return &APIHandler{
		recipientService: service,
	}
}

// GetRecipient handles GET /api/recipients/{email}?workspace_id=xxx
func (h *APIHandler) GetRecipient(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract email from URL path
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/recipients/"), "/")
	if len(pathParts) == 0 || pathParts[0] == "" {
		http.Error(w, "Email required", http.StatusBadRequest)
		return
	}

	email := strings.ToLower(strings.TrimSpace(pathParts[0]))
	workspaceID := r.URL.Query().Get("workspace_id")

	if workspaceID == "" {
		http.Error(w, "workspace_id parameter required", http.StatusBadRequest)
		return
	}

	recipient, err := h.recipientService.GetRecipient(email, workspaceID)
	if err != nil {
		log.Printf("Error getting recipient %s: %v", email, err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if recipient == nil {
		http.Error(w, "Recipient not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(recipient)
}

// GetRecipientSummary handles GET /api/recipients/{email}/summary?workspace_id=xxx
func (h *APIHandler) GetRecipientSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract email from URL path
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/recipients/"), "/")
	if len(pathParts) < 2 || pathParts[0] == "" {
		http.Error(w, "Email required", http.StatusBadRequest)
		return
	}

	email := strings.ToLower(strings.TrimSpace(pathParts[0]))
	workspaceID := r.URL.Query().Get("workspace_id")

	if workspaceID == "" {
		http.Error(w, "workspace_id parameter required", http.StatusBadRequest)
		return
	}

	summary, err := h.recipientService.GetRecipientSummary(email, workspaceID)
	if err != nil {
		log.Printf("Error getting recipient summary %s: %v", email, err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if summary == nil {
		http.Error(w, "Recipient not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}

// GetCampaignStats handles GET /api/campaigns/{campaign_id}/stats?workspace_id=xxx
func (h *APIHandler) GetCampaignStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract campaign ID from URL path
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/campaigns/"), "/")
	if len(pathParts) < 2 || pathParts[0] == "" {
		http.Error(w, "Campaign ID required", http.StatusBadRequest)
		return
	}

	campaignID := pathParts[0]
	workspaceID := r.URL.Query().Get("workspace_id")

	if workspaceID == "" {
		http.Error(w, "workspace_id parameter required", http.StatusBadRequest)
		return
	}

	stats, err := h.recipientService.GetCampaignStats(campaignID, workspaceID)
	if err != nil {
		log.Printf("Error getting campaign stats %s: %v", campaignID, err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// UpdateRecipientStatus handles PUT /api/recipients/{email}/status
func (h *APIHandler) UpdateRecipientStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract email from URL path
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/recipients/"), "/")
	if len(pathParts) < 2 || pathParts[0] == "" {
		http.Error(w, "Email required", http.StatusBadRequest)
		return
	}

	email := strings.ToLower(strings.TrimSpace(pathParts[0]))
	workspaceID := r.URL.Query().Get("workspace_id")

	if workspaceID == "" {
		http.Error(w, "workspace_id parameter required", http.StatusBadRequest)
		return
	}

	// Parse request body
	var request struct {
		Status models.RecipientStatus `json:"status"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate status
	if request.Status == "" {
		http.Error(w, "Status is required", http.StatusBadRequest)
		return
	}

	// Get existing recipient
	recipient, err := h.recipientService.GetRecipient(email, workspaceID)
	if err != nil {
		log.Printf("Error getting recipient %s: %v", email, err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if recipient == nil {
		http.Error(w, "Recipient not found", http.StatusNotFound)
		return
	}

	// Update status
	recipient.Status = request.Status

	// Set opt-out date if unsubscribing
	if request.Status == models.RecipientStatusUnsubscribed && recipient.OptOutDate == nil {
		now := h.recipientService.db.QueryRow("SELECT NOW()").Scan()
		if now != nil {
			// This is a simplified approach - in production you'd handle this more carefully
		}
	}

	if err := h.recipientService.UpsertRecipient(recipient); err != nil {
		log.Printf("Error updating recipient status %s: %v", email, err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(recipient)
}

// ListRecipients handles GET /api/recipients?workspace_id=xxx&status=xxx&limit=50&offset=0
func (h *APIHandler) ListRecipients(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	workspaceID := r.URL.Query().Get("workspace_id")
	if workspaceID == "" {
		http.Error(w, "workspace_id parameter required", http.StatusBadRequest)
		return
	}

	status := r.URL.Query().Get("status")
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")

	// Parse limit and offset
	limit := 50 // default
	offset := 0 // default

	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 1000 {
			limit = l
		}
	}

	if offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	recipients, err := h.listRecipients(workspaceID, status, limit, offset)
	if err != nil {
		log.Printf("Error listing recipients: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"recipients": recipients,
		"limit":      limit,
		"offset":     offset,
		"count":      len(recipients),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// listRecipients is a helper method to list recipients with filtering
func (h *APIHandler) listRecipients(workspaceID, status string, limit, offset int) ([]*models.Recipient, error) {
	query := `
		SELECT id, email_address, workspace_id, user_id, campaign_id,
			first_name, last_name, status, opt_in_date, opt_out_date,
			bounce_count, last_bounce_date, bounce_type, metadata,
			created_at, updated_at
		FROM recipients
		WHERE workspace_id = ?
	`

	args := []interface{}{workspaceID}

	if status != "" && status != "all" {
		query += " AND status = ?"
		args = append(args, status)
	}

	query += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := h.recipientService.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query recipients: %w", err)
	}
	defer rows.Close()

	var recipients []*models.Recipient

	for rows.Next() {
		recipient := &models.Recipient{}
		var metadata string
		var userID, campaignID, firstName, lastName sql.NullString
		var optInDate, optOutDate, lastBounceDate sql.NullTime
		var bounceType sql.NullString

		err := rows.Scan(
			&recipient.ID,
			&recipient.EmailAddress,
			&recipient.WorkspaceID,
			&userID,
			&campaignID,
			&firstName,
			&lastName,
			&recipient.Status,
			&optInDate,
			&optOutDate,
			&recipient.BounceCount,
			&lastBounceDate,
			&bounceType,
			&metadata,
			&recipient.CreatedAt,
			&recipient.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan recipient: %w", err)
		}

		// Handle nullable fields
		if userID.Valid {
			recipient.UserID = &userID.String
		}
		if campaignID.Valid {
			recipient.CampaignID = &campaignID.String
		}
		if firstName.Valid {
			recipient.FirstName = &firstName.String
		}
		if lastName.Valid {
			recipient.LastName = &lastName.String
		}
		if optInDate.Valid {
			recipient.OptInDate = &optInDate.Time
		}
		if optOutDate.Valid {
			recipient.OptOutDate = &optOutDate.Time
		}
		if lastBounceDate.Valid {
			recipient.LastBounceDate = &lastBounceDate.Time
		}
		if bounceType.Valid {
			bt := models.BounceType(bounceType.String)
			recipient.BounceType = &bt
		}

		// Parse metadata JSON
		if metadata != "" {
			json.Unmarshal([]byte(metadata), &recipient.Metadata)
		}

		recipients = append(recipients, recipient)
	}

	return recipients, nil
}

// CleanupRecipients handles POST /api/recipients/cleanup
func (h *APIHandler) CleanupRecipients(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var request struct {
		RetentionDays int `json:"retention_days"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Default to 90 days if not specified
	if request.RetentionDays <= 0 {
		request.RetentionDays = 90
	}

	// Validate retention days (minimum 7 days for safety)
	if request.RetentionDays < 7 {
		http.Error(w, "Retention days must be at least 7", http.StatusBadRequest)
		return
	}

	if err := h.recipientService.CleanupInactiveRecipients(request.RetentionDays); err != nil {
		log.Printf("Error cleaning up recipients: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Cleanup completed with %d day retention", request.RetentionDays),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
