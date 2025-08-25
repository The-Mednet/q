package queue

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"relay/internal/config"
	"relay/pkg/models"

	_ "github.com/go-sql-driver/mysql"
)

type MySQLQueue struct {
	db *sql.DB
}

func NewMySQLQueue(cfg *config.MySQLConfig) (*MySQLQueue, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&multiStatements=true",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Database)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	return &MySQLQueue{db: db}, nil
}

func (q *MySQLQueue) Enqueue(message *models.Message) error {
	toEmails, _ := json.Marshal(message.To)
	ccEmails, _ := json.Marshal(message.CC)
	bccEmails, _ := json.Marshal(message.BCC)
	headers, _ := json.Marshal(message.Headers)
	attachments, _ := json.Marshal(message.Attachments)
	metadata, _ := json.Marshal(message.Metadata)

	query := `
		INSERT INTO messages (
			id, from_email, to_emails, cc_emails, bcc_emails, 
			subject, html_body, text_body, headers, attachments, 
			metadata, campaign_id, notification_id, user_id, provider_id, status, queued_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := q.db.Exec(query,
		message.ID,
		message.From,
		string(toEmails),
		string(ccEmails),
		string(bccEmails),
		message.Subject,
		message.HTML,
		message.Text,
		string(headers),
		string(attachments),
		string(metadata),
		message.CampaignID,
		message.NotificationID,
		message.UserID,
		message.ProviderID,
		message.Status,
		message.QueuedAt,
	)

	return err
}

func (q *MySQLQueue) Dequeue(batchSize int) ([]*models.Message, error) {
	tx, err := q.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	query := `
		SELECT id, from_email, to_emails, cc_emails, bcc_emails,
			subject, html_body, text_body, headers, attachments,
			metadata, campaign_id, notification_id, user_id, provider_id, status, queued_at, processed_at, error, retry_count
		FROM messages
		WHERE status = 'queued' OR (status = 'failed' AND retry_count < 3)
		ORDER BY queued_at ASC
		LIMIT ?
		FOR UPDATE
	`

	rows, err := tx.Query(query, batchSize)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []*models.Message
	var ids []string

	for rows.Next() {
		msg := &models.Message{}
		var toEmails, ccEmails, bccEmails, headers, attachments, metadata string
		var processedAt sql.NullTime
		var errorMsg sql.NullString
		var retryCount int

		err := rows.Scan(
			&msg.ID,
			&msg.From,
			&toEmails,
			&ccEmails,
			&bccEmails,
			&msg.Subject,
			&msg.HTML,
			&msg.Text,
			&headers,
			&attachments,
			&metadata,
			&msg.CampaignID,
			&msg.NotificationID,
			&msg.UserID,
			&msg.ProviderID,
			&msg.Status,
			&msg.QueuedAt,
			&processedAt,
			&errorMsg,
			&retryCount,
		)
		if err != nil {
			return nil, err
		}

		json.Unmarshal([]byte(toEmails), &msg.To)
		json.Unmarshal([]byte(ccEmails), &msg.CC)
		json.Unmarshal([]byte(bccEmails), &msg.BCC)
		json.Unmarshal([]byte(headers), &msg.Headers)
		json.Unmarshal([]byte(attachments), &msg.Attachments)
		json.Unmarshal([]byte(metadata), &msg.Metadata)

		if processedAt.Valid {
			msg.ProcessedAt = &processedAt.Time
		}
		if errorMsg.Valid {
			msg.Error = errorMsg.String
		}

		messages = append(messages, msg)
		ids = append(ids, msg.ID)
	}

	if len(ids) > 0 {
		// Validate IDs to ensure they are valid UUIDs (defense in depth)
		for _, id := range ids {
			if len(id) != 36 || !isValidUUID(id) {
				return nil, fmt.Errorf("invalid message ID format: %s", id)
			}
		}
		
		// Build parameterized query safely
		placeholders := make([]string, len(ids))
		args := make([]interface{}, len(ids))
		for i, id := range ids {
			placeholders[i] = "?"
			args[i] = id
		}
		
		updateQuery := fmt.Sprintf(
			"UPDATE messages SET status = 'processing' WHERE id IN (%s)",
			strings.Join(placeholders, ","),
		)

		_, err = tx.Exec(updateQuery, args...)
		if err != nil {
			return nil, fmt.Errorf("failed to update message status: %w", err)
		}
	}

	return messages, tx.Commit()
}

func (q *MySQLQueue) UpdateStatus(id string, status models.MessageStatus, err error) error {
	query := `
		UPDATE messages 
		SET status = ?, processed_at = ?, error = ?, retry_count = retry_count + 1
		WHERE id = ?
	`

	var errorMsg sql.NullString
	if err != nil {
		errorMsg.Valid = true
		errorMsg.String = err.Error()
	}

	_, dbErr := q.db.Exec(query, status, time.Now(), errorMsg, id)
	return dbErr
}

func (q *MySQLQueue) UpdateStatusWithProvider(id string, status models.MessageStatus, providerID string, err error) error {
	log.Printf("DEBUG: UpdateStatusWithProvider called - id=%s, status=%s, provider=%s, hasError=%v", id, status, providerID, err != nil)
	
	// For sent messages, also update sent_at
	query := `
		UPDATE messages 
		SET status = ?, processed_at = ?, sent_at = ?, provider_id = ?, error = ?, retry_count = retry_count + 1
		WHERE id = ?
	`

	var errorMsg sql.NullString
	if err != nil {
		errorMsg.Valid = true
		errorMsg.String = err.Error()
	}

	var provider sql.NullString
	if providerID != "" {
		provider.Valid = true
		provider.String = providerID
		log.Printf("DEBUG: Setting provider_id to '%s'", providerID)
	} else {
		log.Printf("DEBUG: Provider ID is empty, setting NULL")
	}

	now := time.Now()
	var sentAt sql.NullTime
	if status == models.StatusSent {
		sentAt.Valid = true
		sentAt.Time = now
	}

	log.Printf("DEBUG: Executing SQL with params - status=%s, processed_at=%v, sent_at=%v, provider=%v, error=%v, id=%s", 
		status, now, sentAt, provider, errorMsg, id)

	result, dbErr := q.db.Exec(query, status, now, sentAt, provider, errorMsg, id)
	if dbErr != nil {
		log.Printf("ERROR: UpdateStatusWithProvider failed - %v", dbErr)
		return dbErr
	}
	
	rows, _ := result.RowsAffected()
	log.Printf("DEBUG: UpdateStatusWithProvider updated %d rows for message %s", rows, id)
	
	// Verify the update
	var checkProvider sql.NullString
	checkErr := q.db.QueryRow("SELECT provider_id FROM messages WHERE id = ?", id).Scan(&checkProvider)
	if checkErr == nil {
		log.Printf("DEBUG: After update, provider_id in DB is: %v (valid=%v)", checkProvider.String, checkProvider.Valid)
	}
	
	return nil
}

func (q *MySQLQueue) Get(id string) (*models.Message, error) {
	query := `
		SELECT id, from_email, to_emails, cc_emails, bcc_emails,
			subject, html_body, text_body, headers, attachments,
			metadata, campaign_id, notification_id, user_id, provider_id, status, queued_at, processed_at, error
		FROM messages
		WHERE id = ?
	`

	msg := &models.Message{}
	var toEmails, ccEmails, bccEmails, headers, attachments, metadata string
	var processedAt sql.NullTime
	var errorMsg sql.NullString

	err := q.db.QueryRow(query, id).Scan(
		&msg.ID,
		&msg.From,
		&toEmails,
		&ccEmails,
		&bccEmails,
		&msg.Subject,
		&msg.HTML,
		&msg.Text,
		&headers,
		&attachments,
		&metadata,
		&msg.CampaignID,
		&msg.NotificationID,
		&msg.UserID,
		&msg.ProviderID,
		&msg.Status,
		&msg.QueuedAt,
		&processedAt,
		&errorMsg,
	)

	if err != nil {
		return nil, err
	}

	json.Unmarshal([]byte(toEmails), &msg.To)
	json.Unmarshal([]byte(ccEmails), &msg.CC)
	json.Unmarshal([]byte(bccEmails), &msg.BCC)
	json.Unmarshal([]byte(headers), &msg.Headers)
	json.Unmarshal([]byte(attachments), &msg.Attachments)
	json.Unmarshal([]byte(metadata), &msg.Metadata)

	if processedAt.Valid {
		msg.ProcessedAt = &processedAt.Time
	}
	if errorMsg.Valid {
		msg.Error = errorMsg.String
	}

	return msg, nil
}

func (q *MySQLQueue) Remove(id string) error {
	_, err := q.db.Exec("DELETE FROM messages WHERE id = ?", id)
	return err
}

func (q *MySQLQueue) Close() error {
	return q.db.Close()
}

func (q *MySQLQueue) GetStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	var counts []struct {
		Status string
		Count  int
	}

	query := `
		SELECT status, COUNT(*) as count
		FROM messages
		GROUP BY status
	`

	rows, err := q.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var c struct {
			Status string
			Count  int
		}
		if err := rows.Scan(&c.Status, &c.Count); err != nil {
			return nil, err
		}
		counts = append(counts, c)
	}

	stats["statusCounts"] = counts

	var total int
	q.db.QueryRow("SELECT COUNT(*) FROM messages").Scan(&total)
	stats["total"] = total

	return stats, nil
}

func (q *MySQLQueue) GetMessages(limit, offset int, status string) ([]*models.Message, error) {
	query := `
		SELECT id, from_email, to_emails, subject, status, queued_at, processed_at, error
		FROM messages
	`

	args := []interface{}{}

	if status != "" && status != "all" {
		query += " WHERE status = ?"
		args = append(args, status)
	}

	query += " ORDER BY queued_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := q.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []*models.Message

	for rows.Next() {
		msg := &models.Message{}
		var toEmails string
		var processedAt sql.NullTime
		var errorMsg sql.NullString

		err := rows.Scan(
			&msg.ID,
			&msg.From,
			&toEmails,
			&msg.Subject,
			&msg.Status,
			&msg.QueuedAt,
			&processedAt,
			&errorMsg,
		)
		if err != nil {
			return nil, err
		}

		json.Unmarshal([]byte(toEmails), &msg.To)

		if processedAt.Valid {
			msg.ProcessedAt = &processedAt.Time
		}
		if errorMsg.Valid {
			msg.Error = errorMsg.String
		}

		messages = append(messages, msg)
	}

	return messages, nil
}

// GetSentCountsByProviderAndSender returns sent message counts for the last 24 hours
func (q *MySQLQueue) GetSentCountsByWorkspaceAndSender() (map[string]map[string]int, error) {
	query := `
		SELECT provider_id, from_email, COUNT(*) as count
		FROM messages
		WHERE status = 'sent' 
		  AND processed_at >= DATE_SUB(NOW(), INTERVAL 24 HOUR)
		GROUP BY provider_id, from_email
	`

	rows, err := q.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Structure: provider_id -> sender_email -> count
	counts := make(map[string]map[string]int)

	for rows.Next() {
		var providerID, fromEmail string
		var count int

		if err := rows.Scan(&providerID, &fromEmail, &count); err != nil {
			return nil, err
		}

		log.Printf("DEBUG: Found DB record: provider='%s', from='%s', count=%d", providerID, fromEmail, count)

		if counts[providerID] == nil {
			counts[providerID] = make(map[string]int)
		}
		counts[providerID][fromEmail] = count
	}

	return counts, nil
}

// isValidUUID validates that a string is a valid UUID format
func isValidUUID(s string) bool {
	// UUID format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
	uuidRegex := regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	return uuidRegex.MatchString(s)
}
