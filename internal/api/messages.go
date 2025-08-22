package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

type MessagesAPI struct {
	db *sql.DB
}

func NewMessagesAPI(db *sql.DB) *MessagesAPI {
	return &MessagesAPI{db: db}
}

type Message struct {
	ID         string            `json:"id"`
	FromEmail  string            `json:"from_email"`
	FromName   string            `json:"from_name,omitempty"`
	ToEmails   []string          `json:"to_emails"`
	CcEmails   []string          `json:"cc_emails,omitempty"`
	BccEmails  []string          `json:"bcc_emails,omitempty"`
	Subject    string            `json:"subject"`
	HTMLBody   string            `json:"html_body,omitempty"`
	TextBody   string            `json:"text_body,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
	Status     string            `json:"status"`
	Provider   string            `json:"provider,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
	SentAt     *time.Time        `json:"sent_at,omitempty"`
	Error      string            `json:"error,omitempty"`
	RetryCount int               `json:"retry_count"`
}

type MessagesResponse struct {
	Messages []Message `json:"messages"`
	Total    int64     `json:"total"`
	Offset   int       `json:"offset"`
	Limit    int       `json:"limit"`
}

func (api *MessagesAPI) RegisterRoutes(router *mux.Router) {
	router.HandleFunc("/api/messages", api.ListMessages).Methods("GET")
	router.HandleFunc("/api/messages/{id}", api.GetMessage).Methods("GET")
	router.HandleFunc("/api/messages/{id}/resend", api.ResendMessage).Methods("POST")
}

func (api *MessagesAPI) ListMessages(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	offsetStr := r.URL.Query().Get("offset")
	limitStr := r.URL.Query().Get("limit")
	search := r.URL.Query().Get("search")
	status := r.URL.Query().Get("status")

	offset := 0
	if offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	limit := 25
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	// Build query
	where := []string{"1=1"}
	args := []interface{}{}

	if search != "" {
		where = append(where, "(from_email LIKE ? OR to_emails LIKE ? OR subject LIKE ?)")
		searchPattern := "%" + search + "%"
		args = append(args, searchPattern, searchPattern, searchPattern)
	}

	if status != "" {
		where = append(where, "status = ?")
		args = append(args, status)
	}

	// Get total count
	countQuery := "SELECT COUNT(*) FROM messages WHERE " + strings.Join(where, " AND ")
	var total int64
	api.db.QueryRow(countQuery, args...).Scan(&total)

	// Get messages
	args = append(args, limit, offset)
	query := `
		SELECT 
			id, from_email, to_emails, cc_emails, bcc_emails,
			subject, html_body, text_body, headers, status,
			provider_id, queued_at, sent_at, error, retry_count
		FROM messages
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY queued_at DESC
		LIMIT ? OFFSET ?
	`

	rows, err := api.db.Query(query, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	messages := []Message{}
	for rows.Next() {
		var msg Message
		var toEmails, ccEmails, bccEmails sql.NullString
		var htmlBody, textBody, headers, provider, errorMsg sql.NullString
		var sentAt sql.NullTime

		err := rows.Scan(
			&msg.ID, &msg.FromEmail, &toEmails, &ccEmails, &bccEmails,
			&msg.Subject, &htmlBody, &textBody, &headers, &msg.Status,
			&provider, &msg.CreatedAt, &sentAt, &errorMsg, &msg.RetryCount,
		)
		if err != nil {
			continue
		}

		// Parse JSON fields
		if toEmails.Valid {
			json.Unmarshal([]byte(toEmails.String), &msg.ToEmails)
		}
		if ccEmails.Valid {
			json.Unmarshal([]byte(ccEmails.String), &msg.CcEmails)
		}
		if bccEmails.Valid {
			json.Unmarshal([]byte(bccEmails.String), &msg.BccEmails)
		}
		if headers.Valid {
			json.Unmarshal([]byte(headers.String), &msg.Headers)
		}

		if htmlBody.Valid {
			msg.HTMLBody = htmlBody.String
		}
		if textBody.Valid {
			msg.TextBody = textBody.String
		}
		if provider.Valid {
			msg.Provider = provider.String
		}
		if sentAt.Valid {
			msg.SentAt = &sentAt.Time
		}
		if errorMsg.Valid {
			msg.Error = errorMsg.String
		}

		// Extract from name from headers if available
		if fromHeader, ok := msg.Headers["From"]; ok {
			if idx := strings.Index(fromHeader, "<"); idx > 0 {
				msg.FromName = strings.TrimSpace(fromHeader[:idx])
			}
		}

		messages = append(messages, msg)
	}

	response := MessagesResponse{
		Messages: messages,
		Total:    total,
		Offset:   offset,
		Limit:    limit,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (api *MessagesAPI) GetMessage(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	query := `
		SELECT 
			id, from_email, to_emails, cc_emails, bcc_emails,
			subject, html_body, text_body, headers, status,
			provider_id, queued_at, sent_at, error, retry_count
		FROM messages
		WHERE id = ?
	`

	var msg Message
	var toEmails, ccEmails, bccEmails sql.NullString
	var htmlBody, textBody, headers, provider, errorMsg sql.NullString
	var sentAt sql.NullTime

	err := api.db.QueryRow(query, id).Scan(
		&msg.ID, &msg.FromEmail, &toEmails, &ccEmails, &bccEmails,
		&msg.Subject, &htmlBody, &textBody, &headers, &msg.Status,
		&provider, &msg.CreatedAt, &sentAt, &errorMsg, &msg.RetryCount,
	)

	if err == sql.ErrNoRows {
		http.Error(w, "Message not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Parse JSON fields
	if toEmails.Valid {
		json.Unmarshal([]byte(toEmails.String), &msg.ToEmails)
	}
	if ccEmails.Valid {
		json.Unmarshal([]byte(ccEmails.String), &msg.CcEmails)
	}
	if bccEmails.Valid {
		json.Unmarshal([]byte(bccEmails.String), &msg.BccEmails)
	}
	if headers.Valid {
		json.Unmarshal([]byte(headers.String), &msg.Headers)
	}

	if htmlBody.Valid {
		msg.HTMLBody = htmlBody.String
	}
	if textBody.Valid {
		msg.TextBody = textBody.String
	}
	if provider.Valid {
		msg.Provider = provider.String
	}
	if sentAt.Valid {
		msg.SentAt = &sentAt.Time
	}
	if errorMsg.Valid {
		msg.Error = errorMsg.String
	}

	// Extract from name from headers if available
	if fromHeader, ok := msg.Headers["From"]; ok {
		if idx := strings.Index(fromHeader, "<"); idx > 0 {
			msg.FromName = strings.TrimSpace(fromHeader[:idx])
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(msg)
}

func (api *MessagesAPI) ResendMessage(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	// Reset message status to queued for retry
	query := `
		UPDATE messages 
		SET status = 'queued', 
		    error = NULL,
		    retry_count = retry_count + 1,
		    processed_at = NULL,
		    sent_at = NULL
		WHERE id = ? AND status IN ('failed', 'auth_error')
	`

	result, err := api.db.Exec(query, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, "Message not found or not in failed state", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "Message queued for resend",
		"id":     id,
	})
}

