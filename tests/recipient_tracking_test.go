package tests

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"testing"
	"time"

	"relay/internal/config"
	"relay/internal/recipient"
	"relay/pkg/models"

	_ "github.com/go-sql-driver/mysql"
)

// TestRecipientTracking tests the complete recipient tracking system
func TestRecipientTracking(t *testing.T) {
	// Skip if no database connection available
	if testing.Short() {
		t.Skip("Skipping database tests in short mode")
	}

	db, err := setupTestDatabase()
	if err != nil {
		t.Skip(fmt.Sprintf("Skipping test - database not available: %v", err))
	}
	defer db.Close()

	// Clean up test data
	cleanupTestData(t, db)

	// Initialize recipient service
	recipientService := recipient.NewService(db)

	t.Run("ProcessMessageRecipients", func(t *testing.T) {
		testProcessMessageRecipients(t, db, recipientService)
	})

	t.Run("UpdateDeliveryStatus", func(t *testing.T) {
		testUpdateDeliveryStatus(t, db, recipientService)
	})

	t.Run("RecordEngagementEvent", func(t *testing.T) {
		testRecordEngagementEvent(t, db, recipientService)
	})

	t.Run("GetRecipientSummary", func(t *testing.T) {
		testGetRecipientSummary(t, db, recipientService)
	})

	t.Run("GetCampaignStats", func(t *testing.T) {
		testGetCampaignStats(t, db, recipientService)
	})

	t.Run("BounceHandling", func(t *testing.T) {
		testBounceHandling(t, db, recipientService)
	})

	t.Run("UnsubscribeHandling", func(t *testing.T) {
		testUnsubscribeHandling(t, db, recipientService)
	})

	// Clean up test data
	cleanupTestData(t, db)
}

func setupTestDatabase() (*sql.DB, error) {
	// Try to connect to test database
	cfg := &config.MySQLConfig{
		Host:     "localhost",
		Port:     3306,
		User:     "relay",
		Password: "password",
		Database: "relay_test",
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&multiStatements=true",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Database)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		// Try regular database if test database doesn't exist
		cfg.Database = "relay"
		dsn = fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&multiStatements=true",
			cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Database)

		db.Close()
		db, err = sql.Open("mysql", dsn)
		if err != nil {
			return nil, fmt.Errorf("failed to open database: %w", err)
		}

		if err := db.Ping(); err != nil {
			return nil, fmt.Errorf("failed to ping database: %w", err)
		}
	}

	return db, nil
}

func cleanupTestData(t *testing.T, db *sql.DB) {
	tables := []string{
		"recipient_events",
		"message_recipients",
		"recipients",
		"messages",
	}

	for _, table := range tables {
		query := fmt.Sprintf("DELETE FROM %s WHERE workspace_id LIKE 'test-%%'", table)
		if _, err := db.Exec(query); err != nil {
			log.Printf("Warning: Failed to clean up %s: %v", table, err)
		}
	}
}

func createTestMessage(workspaceID, messageID, campaignID string, to, cc, bcc []string) *models.Message {
	return &models.Message{
		ID:          messageID,
		From:        "sender@test.com",
		To:          to,
		CC:          cc,
		BCC:         bcc,
		Subject:     "Test Message",
		HTML:        "<p>Test HTML</p>",
		Text:        "Test Text",
		CampaignID:  campaignID,
		UserID:      "test-user-123",
		WorkspaceID: workspaceID,
		Status:      models.StatusQueued,
		QueuedAt:    time.Now(),
	}
}

func testProcessMessageRecipients(t *testing.T, db *sql.DB, service *recipient.Service) {
	workspaceID := "test-workspace-1"
	messageID := "test-msg-1"
	campaignID := "test-campaign-1"

	// Create test message
	message := createTestMessage(
		workspaceID,
		messageID,
		campaignID,
		[]string{"recipient1@test.com", "recipient2@test.com"},
		[]string{"cc@test.com"},
		[]string{"bcc@test.com"},
	)

	// First insert the message into messages table
	if err := insertTestMessage(db, message); err != nil {
		t.Fatalf("Failed to insert test message: %v", err)
	}

	// Process recipients
	err := service.ProcessMessageRecipients(message)
	if err != nil {
		t.Fatalf("Failed to process message recipients: %v", err)
	}

	// Verify recipients were created
	recipient1, err := service.GetRecipient("recipient1@test.com", workspaceID)
	if err != nil {
		t.Fatalf("Failed to get recipient1: %v", err)
	}
	if recipient1 == nil {
		t.Fatal("Recipient1 was not created")
	}
	if recipient1.Status != models.RecipientStatusActive {
		t.Errorf("Expected recipient1 status to be ACTIVE, got %s", recipient1.Status)
	}

	// Verify message recipient relationships
	var count int
	query := `SELECT COUNT(*) FROM message_recipients WHERE message_id = ?`
	err = db.QueryRow(query, messageID).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count message recipients: %v", err)
	}

	expectedCount := len(message.To) + len(message.CC) + len(message.BCC)
	if count != expectedCount {
		t.Errorf("Expected %d message recipient records, got %d", expectedCount, count)
	}
}

func testUpdateDeliveryStatus(t *testing.T, db *sql.DB, service *recipient.Service) {
	workspaceID := "test-workspace-2"
	messageID := "test-msg-2"
	email := "delivery@test.com"

	// Create test data
	message := createTestMessage(workspaceID, messageID, "test-campaign-2", []string{email}, nil, nil)
	if err := insertTestMessage(db, message); err != nil {
		t.Fatalf("Failed to insert test message: %v", err)
	}

	if err := service.ProcessMessageRecipients(message); err != nil {
		t.Fatalf("Failed to process recipients: %v", err)
	}

	// Test successful delivery
	err := service.UpdateDeliveryStatus(messageID, email, models.DeliveryStatusSent, nil)
	if err != nil {
		t.Fatalf("Failed to update delivery status to sent: %v", err)
	}

	// Verify status was updated
	var status string
	var sentAt sql.NullTime
	query := `
		SELECT mr.delivery_status, mr.sent_at
		FROM message_recipients mr
		JOIN recipients r ON mr.recipient_id = r.id
		WHERE mr.message_id = ? AND r.email_address = ?
	`
	err = db.QueryRow(query, messageID, email).Scan(&status, &sentAt)
	if err != nil {
		t.Fatalf("Failed to query delivery status: %v", err)
	}

	if status != string(models.DeliveryStatusSent) {
		t.Errorf("Expected status SENT, got %s", status)
	}

	if !sentAt.Valid {
		t.Error("Expected sent_at to be set")
	}

	// Test bounce
	bounceReason := "Hard bounce - invalid email"
	err = service.UpdateDeliveryStatus(messageID, email, models.DeliveryStatusBounced, &bounceReason)
	if err != nil {
		t.Fatalf("Failed to update delivery status to bounced: %v", err)
	}

	// Verify bounce was recorded
	query = `
		SELECT mr.delivery_status, mr.bounce_reason, r.bounce_count
		FROM message_recipients mr
		JOIN recipients r ON mr.recipient_id = r.id
		WHERE mr.message_id = ? AND r.email_address = ?
	`
	var bounceReasonDB sql.NullString
	var bounceCount int
	err = db.QueryRow(query, messageID, email).Scan(&status, &bounceReasonDB, &bounceCount)
	if err != nil {
		t.Fatalf("Failed to query bounce status: %v", err)
	}

	if status != string(models.DeliveryStatusBounced) {
		t.Errorf("Expected status BOUNCED, got %s", status)
	}

	if !bounceReasonDB.Valid || bounceReasonDB.String != bounceReason {
		t.Errorf("Expected bounce reason '%s', got '%s'", bounceReason, bounceReasonDB.String)
	}

	if bounceCount != 1 {
		t.Errorf("Expected bounce count 1, got %d", bounceCount)
	}
}

func testRecordEngagementEvent(t *testing.T, db *sql.DB, service *recipient.Service) {
	workspaceID := "test-workspace-3"
	messageID := "test-msg-3"
	email := "engagement@test.com"

	// Setup test data
	message := createTestMessage(workspaceID, messageID, "test-campaign-3", []string{email}, nil, nil)
	if err := insertTestMessage(db, message); err != nil {
		t.Fatalf("Failed to insert test message: %v", err)
	}

	if err := service.ProcessMessageRecipients(message); err != nil {
		t.Fatalf("Failed to process recipients: %v", err)
	}

	if err := service.UpdateDeliveryStatus(messageID, email, models.DeliveryStatusSent, nil); err != nil {
		t.Fatalf("Failed to update delivery status: %v", err)
	}

	// Record open event
	eventData := map[string]interface{}{
		"user_agent": "Test Browser",
		"timestamp":  time.Now().Unix(),
	}
	ipAddress := "192.168.1.1"
	userAgent := "Test Browser 1.0"

	err := service.RecordEngagementEvent(messageID, email, models.EventTypeOpen, eventData, &ipAddress, &userAgent)
	if err != nil {
		t.Fatalf("Failed to record open event: %v", err)
	}

	// Verify open was recorded
	var opens int
	var lastOpenAt sql.NullTime
	query := `
		SELECT mr.opens, mr.last_open_at
		FROM message_recipients mr
		JOIN recipients r ON mr.recipient_id = r.id
		WHERE mr.message_id = ? AND r.email_address = ?
	`
	err = db.QueryRow(query, messageID, email).Scan(&opens, &lastOpenAt)
	if err != nil {
		t.Fatalf("Failed to query opens: %v", err)
	}

	if opens != 1 {
		t.Errorf("Expected 1 open, got %d", opens)
	}

	if !lastOpenAt.Valid {
		t.Error("Expected last_open_at to be set")
	}

	// Record click event
	clickData := map[string]interface{}{
		"url": "https://example.com/link",
	}

	err = service.RecordEngagementEvent(messageID, email, models.EventTypeClick, clickData, &ipAddress, &userAgent)
	if err != nil {
		t.Fatalf("Failed to record click event: %v", err)
	}

	// Verify click was recorded
	var clicks int
	query = `
		SELECT mr.clicks
		FROM message_recipients mr
		JOIN recipients r ON mr.recipient_id = r.id
		WHERE mr.message_id = ? AND r.email_address = ?
	`
	err = db.QueryRow(query, messageID, email).Scan(&clicks)
	if err != nil {
		t.Fatalf("Failed to query clicks: %v", err)
	}

	if clicks != 1 {
		t.Errorf("Expected 1 click, got %d", clicks)
	}

	// Verify events were recorded in recipient_events table
	var eventCount int
	query = `
		SELECT COUNT(*)
		FROM recipient_events re
		JOIN message_recipients mr ON re.message_recipient_id = mr.id
		JOIN recipients r ON mr.recipient_id = r.id
		WHERE mr.message_id = ? AND r.email_address = ?
	`
	err = db.QueryRow(query, messageID, email).Scan(&eventCount)
	if err != nil {
		t.Fatalf("Failed to count events: %v", err)
	}

	if eventCount != 2 {
		t.Errorf("Expected 2 events, got %d", eventCount)
	}
}

func testGetRecipientSummary(t *testing.T, db *sql.DB, service *recipient.Service) {
	workspaceID := "test-workspace-4"
	email := "summary@test.com"

	// Create multiple messages for this recipient
	for i := 1; i <= 3; i++ {
		messageID := fmt.Sprintf("test-msg-summary-%d", i)
		message := createTestMessage(workspaceID, messageID, "test-campaign-4", []string{email}, nil, nil)

		if err := insertTestMessage(db, message); err != nil {
			t.Fatalf("Failed to insert test message %d: %v", i, err)
		}

		if err := service.ProcessMessageRecipients(message); err != nil {
			t.Fatalf("Failed to process recipients for message %d: %v", i, err)
		}

		if err := service.UpdateDeliveryStatus(messageID, email, models.DeliveryStatusSent, nil); err != nil {
			t.Fatalf("Failed to update delivery status for message %d: %v", i, err)
		}

		// Record some engagement
		if i <= 2 {
			eventData := map[string]interface{}{"test": true}
			ip := "192.168.1.1"
			ua := "Test"
			err := service.RecordEngagementEvent(messageID, email, models.EventTypeOpen, eventData, &ip, &ua)
			if err != nil {
				t.Fatalf("Failed to record engagement for message %d: %v", i, err)
			}
		}
	}

	// Get recipient summary
	summary, err := service.GetRecipientSummary(email, workspaceID)
	if err != nil {
		t.Fatalf("Failed to get recipient summary: %v", err)
	}

	if summary == nil {
		t.Fatal("Expected recipient summary, got nil")
	}

	if summary.TotalMessages != 3 {
		t.Errorf("Expected 3 total messages, got %d", summary.TotalMessages)
	}

	if summary.TotalOpens != 2 {
		t.Errorf("Expected 2 total opens, got %d", summary.TotalOpens)
	}

	expectedEngagementRate := float64(2) / float64(3) // 2 opens / 3 messages
	if summary.EngagementRate < expectedEngagementRate-0.01 || summary.EngagementRate > expectedEngagementRate+0.01 {
		t.Errorf("Expected engagement rate ~%.2f, got %.2f", expectedEngagementRate, summary.EngagementRate)
	}
}

func testGetCampaignStats(t *testing.T, db *sql.DB, service *recipient.Service) {
	workspaceID := "test-workspace-5"
	campaignID := "test-campaign-stats"

	// Create multiple recipients for this campaign
	recipients := []string{"stats1@test.com", "stats2@test.com", "stats3@test.com", "stats4@test.com"}

	for i, email := range recipients {
		messageID := fmt.Sprintf("test-msg-stats-%d", i+1)
		message := createTestMessage(workspaceID, messageID, campaignID, []string{email}, nil, nil)

		if err := insertTestMessage(db, message); err != nil {
			t.Fatalf("Failed to insert test message for %s: %v", email, err)
		}

		if err := service.ProcessMessageRecipients(message); err != nil {
			t.Fatalf("Failed to process recipients for %s: %v", email, err)
		}

		// Send most messages successfully
		if i < 3 {
			if err := service.UpdateDeliveryStatus(messageID, email, models.DeliveryStatusSent, nil); err != nil {
				t.Fatalf("Failed to update delivery status for %s: %v", email, err)
			}

			// Record opens for some
			if i < 2 {
				eventData := map[string]interface{}{"test": true}
				ip := "192.168.1.1"
				ua := "Test"
				err := service.RecordEngagementEvent(messageID, email, models.EventTypeOpen, eventData, &ip, &ua)
				if err != nil {
					t.Fatalf("Failed to record open for %s: %v", email, err)
				}

				// Record clicks for one
				if i == 0 {
					clickData := map[string]interface{}{"url": "https://test.com"}
					err := service.RecordEngagementEvent(messageID, email, models.EventTypeClick, clickData, &ip, &ua)
					if err != nil {
						t.Fatalf("Failed to record click for %s: %v", email, err)
					}
				}
			}
		} else {
			// Last message bounces
			bounceReason := "Test bounce"
			if err := service.UpdateDeliveryStatus(messageID, email, models.DeliveryStatusBounced, &bounceReason); err != nil {
				t.Fatalf("Failed to update bounce status for %s: %v", email, err)
			}
		}
	}

	// Get campaign stats
	stats, err := service.GetCampaignStats(campaignID, workspaceID)
	if err != nil {
		t.Fatalf("Failed to get campaign stats: %v", err)
	}

	if stats == nil {
		t.Fatal("Expected campaign stats, got nil")
	}

	if stats.TotalRecipients != 4 {
		t.Errorf("Expected 4 total recipients, got %d", stats.TotalRecipients)
	}

	if stats.Sent != 3 {
		t.Errorf("Expected 3 sent, got %d", stats.Sent)
	}

	if stats.Bounced != 1 {
		t.Errorf("Expected 1 bounced, got %d", stats.Bounced)
	}

	if stats.Opened != 2 {
		t.Errorf("Expected 2 opened, got %d", stats.Opened)
	}

	if stats.Clicked != 1 {
		t.Errorf("Expected 1 clicked, got %d", stats.Clicked)
	}

	expectedOpenRate := float64(2) / float64(3) // 2 opens / 3 sent
	if stats.OpenRate < expectedOpenRate-0.01 || stats.OpenRate > expectedOpenRate+0.01 {
		t.Errorf("Expected open rate ~%.2f, got %.2f", expectedOpenRate, stats.OpenRate)
	}

	expectedBounceRate := float64(1) / float64(4) // 1 bounce / 4 total
	if stats.BounceRate < expectedBounceRate-0.01 || stats.BounceRate > expectedBounceRate+0.01 {
		t.Errorf("Expected bounce rate ~%.2f, got %.2f", expectedBounceRate, stats.BounceRate)
	}
}

func testBounceHandling(t *testing.T, db *sql.DB, service *recipient.Service) {
	workspaceID := "test-workspace-bounce"
	email := "bounce@test.com"

	// Send multiple messages that bounce
	for i := 1; i <= 5; i++ {
		messageID := fmt.Sprintf("test-msg-bounce-%d", i)
		message := createTestMessage(workspaceID, messageID, "bounce-campaign", []string{email}, nil, nil)

		if err := insertTestMessage(db, message); err != nil {
			t.Fatalf("Failed to insert bounce test message %d: %v", i, err)
		}

		if err := service.ProcessMessageRecipients(message); err != nil {
			t.Fatalf("Failed to process recipients for bounce message %d: %v", i, err)
		}

		bounceReason := fmt.Sprintf("Test bounce %d", i)
		if err := service.UpdateDeliveryStatus(messageID, email, models.DeliveryStatusBounced, &bounceReason); err != nil {
			t.Fatalf("Failed to update bounce status for message %d: %v", i, err)
		}
	}

	// Check that recipient status was updated to BOUNCED after multiple bounces
	recipient, err := service.GetRecipient(email, workspaceID)
	if err != nil {
		t.Fatalf("Failed to get bounced recipient: %v", err)
	}

	if recipient == nil {
		t.Fatal("Bounced recipient not found")
	}

	if recipient.Status != models.RecipientStatusBounced {
		t.Errorf("Expected recipient status BOUNCED, got %s", recipient.Status)
	}

	if recipient.BounceCount != 5 {
		t.Errorf("Expected bounce count 5, got %d", recipient.BounceCount)
	}

	if recipient.LastBounceDate == nil {
		t.Error("Expected last bounce date to be set")
	}
}

func testUnsubscribeHandling(t *testing.T, db *sql.DB, service *recipient.Service) {
	workspaceID := "test-workspace-unsub"
	messageID := "test-msg-unsub"
	email := "unsub@test.com"

	// Setup test data
	message := createTestMessage(workspaceID, messageID, "unsub-campaign", []string{email}, nil, nil)
	if err := insertTestMessage(db, message); err != nil {
		t.Fatalf("Failed to insert unsub test message: %v", err)
	}

	if err := service.ProcessMessageRecipients(message); err != nil {
		t.Fatalf("Failed to process recipients for unsub message: %v", err)
	}

	if err := service.UpdateDeliveryStatus(messageID, email, models.DeliveryStatusSent, nil); err != nil {
		t.Fatalf("Failed to update delivery status for unsub message: %v", err)
	}

	// Record unsubscribe event
	eventData := map[string]interface{}{
		"method": "form",
		"reason": "test unsubscribe",
	}
	ip := "192.168.1.1"
	ua := "Test Browser"

	err := service.RecordEngagementEvent(messageID, email, models.EventTypeUnsubscribe, eventData, &ip, &ua)
	if err != nil {
		t.Fatalf("Failed to record unsubscribe event: %v", err)
	}

	// Check that recipient status was updated
	recipient, err := service.GetRecipient(email, workspaceID)
	if err != nil {
		t.Fatalf("Failed to get unsubscribed recipient: %v", err)
	}

	if recipient == nil {
		t.Fatal("Unsubscribed recipient not found")
	}

	if recipient.Status != models.RecipientStatusUnsubscribed {
		t.Errorf("Expected recipient status UNSUBSCRIBED, got %s", recipient.Status)
	}

	if recipient.OptOutDate == nil {
		t.Error("Expected opt out date to be set")
	}
}

// insertTestMessage inserts a test message into the messages table
func insertTestMessage(db *sql.DB, message *models.Message) error {
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
			metadata, campaign_id, user_id, workspace_id, status, queued_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := db.Exec(query,
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
		message.UserID,
		message.WorkspaceID,
		message.Status,
		message.QueuedAt,
	)

	return err
}

// BenchmarkRecipientProcessing benchmarks the recipient processing performance
func BenchmarkRecipientProcessing(b *testing.B) {
	db, err := setupTestDatabase()
	if err != nil {
		b.Skip(fmt.Sprintf("Skipping benchmark - database not available: %v", err))
	}
	defer db.Close()

	service := recipient.NewService(db)

	// Clean up before and after
	cleanupBenchmarkData(b, db)
	defer cleanupBenchmarkData(b, db)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		workspaceID := fmt.Sprintf("bench-workspace-%d", i)
		messageID := fmt.Sprintf("bench-msg-%d", i)
		campaignID := fmt.Sprintf("bench-campaign-%d", i%100) // Reuse campaigns

		message := createTestMessage(
			workspaceID,
			messageID,
			campaignID,
			[]string{fmt.Sprintf("bench%d@test.com", i)},
			nil,
			nil,
		)

		// Insert message
		if err := insertTestMessage(db, message); err != nil {
			b.Fatalf("Failed to insert message: %v", err)
		}

		// Process recipients
		if err := service.ProcessMessageRecipients(message); err != nil {
			b.Fatalf("Failed to process recipients: %v", err)
		}
	}
}

func cleanupBenchmarkData(b *testing.B, db *sql.DB) {
	tables := []string{
		"recipient_events",
		"message_recipients",
		"recipients",
		"messages",
	}

	for _, table := range tables {
		query := fmt.Sprintf("DELETE FROM %s WHERE workspace_id LIKE 'bench-%%'", table)
		if _, err := db.Exec(query); err != nil {
			log.Printf("Warning: Failed to clean up %s: %v", table, err)
		}
	}
}
