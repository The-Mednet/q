package recipient

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"relay/pkg/models"

	_ "github.com/go-sql-driver/mysql"
)

// Service handles all recipient-related database operations
type Service struct {
	db *sql.DB
}

// NewService creates a new recipient service
func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

// UpsertRecipient creates or updates a recipient record
func (s *Service) UpsertRecipient(recipient *models.Recipient) error {
	metadata, err := json.Marshal(recipient.Metadata)
	if err != nil {
		metadata = []byte("{}")
	}

	query := `
		INSERT INTO recipients (
			email_address, workspace_id, user_id, campaign_id, 
			first_name, last_name, status, opt_in_date, opt_out_date,
			bounce_count, last_bounce_date, bounce_type, metadata
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			user_id = COALESCE(VALUES(user_id), user_id),
			campaign_id = COALESCE(VALUES(campaign_id), campaign_id),
			first_name = COALESCE(VALUES(first_name), first_name),
			last_name = COALESCE(VALUES(last_name), last_name),
			status = VALUES(status),
			opt_in_date = COALESCE(VALUES(opt_in_date), opt_in_date),
			opt_out_date = VALUES(opt_out_date),
			bounce_count = VALUES(bounce_count),
			last_bounce_date = VALUES(last_bounce_date),
			bounce_type = VALUES(bounce_type),
			metadata = VALUES(metadata),
			updated_at = CURRENT_TIMESTAMP
	`

	result, err := s.db.Exec(query,
		recipient.EmailAddress,
		recipient.WorkspaceID,
		recipient.UserID,
		recipient.CampaignID,
		recipient.FirstName,
		recipient.LastName,
		recipient.Status,
		recipient.OptInDate,
		recipient.OptOutDate,
		recipient.BounceCount,
		recipient.LastBounceDate,
		recipient.BounceType,
		string(metadata),
	)
	if err != nil {
		return fmt.Errorf("failed to upsert recipient: %w", err)
	}

	// Update the ID if this was an insert
	if recipient.ID == 0 {
		id, err := result.LastInsertId()
		if err == nil {
			recipient.ID = id
		}
	}

	return nil
}

// GetRecipient retrieves a recipient by email and workspace
func (s *Service) GetRecipient(email, workspaceID string) (*models.Recipient, error) {
	query := `
		SELECT id, email_address, workspace_id, user_id, campaign_id,
			first_name, last_name, status, opt_in_date, opt_out_date,
			bounce_count, last_bounce_date, bounce_type, metadata,
			created_at, updated_at
		FROM recipients
		WHERE email_address = ? AND workspace_id = ?
	`

	recipient := &models.Recipient{}
	var metadata string
	var userID, campaignID, firstName, lastName sql.NullString
	var optInDate, optOutDate, lastBounceDate sql.NullTime
	var bounceType sql.NullString

	err := s.db.QueryRow(query, email, workspaceID).Scan(
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
		if err == sql.ErrNoRows {
			return nil, nil // Not found
		}
		return nil, fmt.Errorf("failed to get recipient: %w", err)
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

	return recipient, nil
}

// ProcessMessageRecipients extracts and stores recipient information from a message
func (s *Service) ProcessMessageRecipients(message *models.Message) error {
	// Start a transaction for atomicity
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Process TO recipients
	if err := s.processRecipientsInTransaction(tx, message, message.To, models.RecipientTypeTo); err != nil {
		return fmt.Errorf("failed to process TO recipients: %w", err)
	}

	// Process CC recipients
	if err := s.processRecipientsInTransaction(tx, message, message.CC, models.RecipientTypeCc); err != nil {
		return fmt.Errorf("failed to process CC recipients: %w", err)
	}

	// Process BCC recipients
	if err := s.processRecipientsInTransaction(tx, message, message.BCC, models.RecipientTypeBcc); err != nil {
		return fmt.Errorf("failed to process BCC recipients: %w", err)
	}

	return tx.Commit()
}

// processRecipientsInTransaction processes a list of email addresses within a transaction
func (s *Service) processRecipientsInTransaction(tx *sql.Tx, message *models.Message, emails []string, recipientType models.RecipientType) error {
	for _, email := range emails {
		if email == "" {
			continue
		}

		// Clean and validate email
		email = strings.TrimSpace(strings.ToLower(email))

		// Create or update recipient record
		recipientID, err := s.upsertRecipientInTransaction(tx, email, message)
		if err != nil {
			log.Printf("Warning: Failed to upsert recipient %s: %v", email, err)
			continue // Continue processing other recipients
		}

		// Create message recipient record
		if err := s.createMessageRecipientInTransaction(tx, message.ID, recipientID, recipientType); err != nil {
			log.Printf("Warning: Failed to create message recipient record for %s: %v", email, err)
			continue
		}
	}

	return nil
}

// upsertRecipientInTransaction handles recipient upsert within a transaction
func (s *Service) upsertRecipientInTransaction(tx *sql.Tx, email string, message *models.Message) (int64, error) {
	// Check if recipient exists
	var recipientID int64
	query := `SELECT id FROM recipients WHERE email_address = ? AND workspace_id = ?`
	err := tx.QueryRow(query, email, message.WorkspaceID).Scan(&recipientID)

	if err == sql.ErrNoRows {
		// Create new recipient
		insertQuery := `
			INSERT INTO recipients (email_address, workspace_id, user_id, campaign_id, status)
			VALUES (?, ?, ?, ?, ?)
		`
		result, err := tx.Exec(insertQuery,
			email,
			message.WorkspaceID,
			message.UserID,
			message.CampaignID,
			models.RecipientStatusActive,
		)
		if err != nil {
			return 0, err
		}

		recipientID, err = result.LastInsertId()
		if err != nil {
			return 0, err
		}
	} else if err != nil {
		return 0, err
	}

	return recipientID, nil
}

// createMessageRecipientInTransaction creates a message recipient record within a transaction
func (s *Service) createMessageRecipientInTransaction(tx *sql.Tx, messageID string, recipientID int64, recipientType models.RecipientType) error {
	query := `
		INSERT IGNORE INTO message_recipients (message_id, recipient_id, recipient_type, delivery_status)
		VALUES (?, ?, ?, ?)
	`

	_, err := tx.Exec(query, messageID, recipientID, recipientType, models.DeliveryStatusPending)
	return err
}

// UpdateDeliveryStatus updates the delivery status for a message recipient
func (s *Service) UpdateDeliveryStatus(messageID string, email string, status models.DeliveryStatus, bounceReason *string) error {
	query := `
		UPDATE message_recipients mr
		JOIN recipients r ON mr.recipient_id = r.id
		JOIN messages m ON mr.message_id = m.id
		SET mr.delivery_status = ?,
			mr.sent_at = CASE WHEN ? = 'SENT' THEN CURRENT_TIMESTAMP ELSE mr.sent_at END,
			mr.bounce_reason = ?,
			mr.updated_at = CURRENT_TIMESTAMP
		WHERE mr.message_id = ? AND r.email_address = ? AND m.workspace_id = r.workspace_id
	`

	_, err := s.db.Exec(query, status, string(status), bounceReason, messageID, email)
	if err != nil {
		return fmt.Errorf("failed to update delivery status: %w", err)
	}

	// If this is a bounce, update the recipient's bounce tracking
	if status == models.DeliveryStatusBounced {
		s.updateRecipientBounceStatus(email, messageID, bounceReason)
	}

	return nil
}

// updateRecipientBounceStatus updates bounce tracking for a recipient
func (s *Service) updateRecipientBounceStatus(email, messageID string, bounceReason *string) {
	// Determine bounce type based on reason
	bounceType := models.BounceTypeSoft // Default to soft bounce
	if bounceReason != nil {
		reason := strings.ToLower(*bounceReason)
		if strings.Contains(reason, "permanent") || strings.Contains(reason, "invalid") ||
			strings.Contains(reason, "not exist") || strings.Contains(reason, "unknown user") {
			bounceType = models.BounceTypeHard
		}
	}

	query := `
		UPDATE recipients r
		JOIN messages m ON r.workspace_id = m.workspace_id
		SET r.bounce_count = r.bounce_count + 1,
			r.last_bounce_date = CURRENT_TIMESTAMP,
			r.bounce_type = ?,
			r.status = CASE 
				WHEN ? = 'HARD' OR r.bounce_count >= 5 THEN 'BOUNCED'
				ELSE r.status
			END,
			r.updated_at = CURRENT_TIMESTAMP
		WHERE r.email_address = ? AND m.id = ?
	`

	_, err := s.db.Exec(query, bounceType, bounceType, email, messageID)
	if err != nil {
		log.Printf("Warning: Failed to update recipient bounce status for %s: %v", email, err)
	}
}

// RecordEngagementEvent records an engagement event (open, click, etc.)
func (s *Service) RecordEngagementEvent(messageID, email string, eventType models.EventType, eventData map[string]interface{}, ipAddress, userAgent *string) error {
	// First, get the message recipient ID
	var messageRecipientID int64
	query := `
		SELECT mr.id
		FROM message_recipients mr
		JOIN recipients r ON mr.recipient_id = r.id
		WHERE mr.message_id = ? AND r.email_address = ?
	`

	err := s.db.QueryRow(query, messageID, email).Scan(&messageRecipientID)
	if err != nil {
		return fmt.Errorf("failed to find message recipient: %w", err)
	}

	// Serialize event data
	eventDataJSON, _ := json.Marshal(eventData)

	// Start transaction
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Insert the event
	insertEventQuery := `
		INSERT INTO recipient_events (message_recipient_id, event_type, event_data, ip_address, user_agent)
		VALUES (?, ?, ?, ?, ?)
	`

	_, err = tx.Exec(insertEventQuery, messageRecipientID, eventType, string(eventDataJSON), ipAddress, userAgent)
	if err != nil {
		return fmt.Errorf("failed to insert recipient event: %w", err)
	}

	// Update message recipient engagement counters
	var updateQuery string
	switch eventType {
	case models.EventTypeOpen:
		updateQuery = `
			UPDATE message_recipients
			SET opens = opens + 1, last_open_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`
	case models.EventTypeClick:
		updateQuery = `
			UPDATE message_recipients
			SET clicks = clicks + 1, last_click_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`
	case models.EventTypeUnsubscribe:
		// Update recipient status
		updateRecipientQuery := `
			UPDATE recipients r
			JOIN message_recipients mr ON r.id = mr.recipient_id
			SET r.status = 'UNSUBSCRIBED', r.opt_out_date = CURRENT_TIMESTAMP, r.updated_at = CURRENT_TIMESTAMP
			WHERE mr.id = ?
		`
		_, err = tx.Exec(updateRecipientQuery, messageRecipientID)
		if err != nil {
			return fmt.Errorf("failed to update recipient unsubscribe status: %w", err)
		}
	}

	if updateQuery != "" {
		_, err = tx.Exec(updateQuery, messageRecipientID)
		if err != nil {
			return fmt.Errorf("failed to update message recipient engagement: %w", err)
		}
	}

	return tx.Commit()
}

// GetRecipientSummary gets aggregated stats for a recipient
func (s *Service) GetRecipientSummary(email, workspaceID string) (*models.RecipientSummary, error) {
	recipient, err := s.GetRecipient(email, workspaceID)
	if err != nil || recipient == nil {
		return nil, err
	}

	query := `
		SELECT 
			COUNT(mr.id) as total_messages,
			SUM(mr.opens) as total_opens,
			SUM(mr.clicks) as total_clicks,
			MAX(GREATEST(mr.last_open_at, mr.last_click_at)) as last_activity
		FROM message_recipients mr
		WHERE mr.recipient_id = ?
	`

	var totalMessages, totalOpens, totalClicks int
	var lastActivity sql.NullTime

	err = s.db.QueryRow(query, recipient.ID).Scan(&totalMessages, &totalOpens, &totalClicks, &lastActivity)
	if err != nil {
		return nil, fmt.Errorf("failed to get recipient summary: %w", err)
	}

	summary := &models.RecipientSummary{
		Recipient:      recipient,
		TotalMessages:  totalMessages,
		TotalOpens:     totalOpens,
		TotalClicks:    totalClicks,
		EngagementRate: 0,
	}

	if lastActivity.Valid {
		summary.LastActivity = &lastActivity.Time
	}

	// Calculate engagement rate (opens + clicks / messages)
	if totalMessages > 0 {
		summary.EngagementRate = float64(totalOpens+totalClicks) / float64(totalMessages)
	}

	return summary, nil
}

// GetCampaignStats gets aggregated stats for a campaign
func (s *Service) GetCampaignStats(campaignID, workspaceID string) (*models.CampaignRecipientStats, error) {
	query := `
		SELECT 
			COUNT(DISTINCT mr.recipient_id) as total_recipients,
			COUNT(CASE WHEN mr.delivery_status = 'SENT' THEN 1 END) as sent,
			COUNT(CASE WHEN mr.delivery_status = 'BOUNCED' THEN 1 END) as bounced,
			COUNT(CASE WHEN mr.opens > 0 THEN 1 END) as opened,
			COUNT(CASE WHEN mr.clicks > 0 THEN 1 END) as clicked,
			COUNT(CASE WHEN r.status = 'UNSUBSCRIBED' AND r.opt_out_date >= mr.created_at THEN 1 END) as unsubscribed
		FROM message_recipients mr
		JOIN recipients r ON mr.recipient_id = r.id
		JOIN messages m ON mr.message_id = m.id
		WHERE m.campaign_id = ? AND r.workspace_id = ?
	`

	var stats models.CampaignRecipientStats
	stats.CampaignID = campaignID

	err := s.db.QueryRow(query, campaignID, workspaceID).Scan(
		&stats.TotalRecipients,
		&stats.Sent,
		&stats.Bounced,
		&stats.Opened,
		&stats.Clicked,
		&stats.Unsubscribed,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get campaign stats: %w", err)
	}

	// Calculate rates
	if stats.Sent > 0 {
		stats.OpenRate = float64(stats.Opened) / float64(stats.Sent)
		stats.ClickRate = float64(stats.Clicked) / float64(stats.Sent)
	}

	if stats.TotalRecipients > 0 {
		stats.BounceRate = float64(stats.Bounced) / float64(stats.TotalRecipients)
	}

	return &stats, nil
}

// CleanupInactiveRecipients removes inactive recipients based on retention policy
func (s *Service) CleanupInactiveRecipients(retentionDays int) error {
	query := `
		DELETE r FROM recipients r
		LEFT JOIN message_recipients mr ON r.id = mr.recipient_id
		WHERE r.status = 'INACTIVE'
		  AND r.updated_at < DATE_SUB(NOW(), INTERVAL ? DAY)
		  AND mr.id IS NULL  -- No associated messages
	`

	result, err := s.db.Exec(query, retentionDays)
	if err != nil {
		return fmt.Errorf("failed to cleanup inactive recipients: %w", err)
	}

	if rowsAffected, _ := result.RowsAffected(); rowsAffected > 0 {
		log.Printf("Cleaned up %d inactive recipients", rowsAffected)
	}

	return nil
}
