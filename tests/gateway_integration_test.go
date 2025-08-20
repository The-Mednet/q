package tests

import (
	"context"
	"errors"
	"testing"
	"time"

	"relay/internal/config"
	"relay/internal/gateway"
	"relay/internal/gateway/mailgun"
	"relay/internal/gateway/manager"
	"relay/internal/gateway/migration"
	"relay/internal/gateway/ratelimit"
	"relay/internal/gateway/reliability"
	"relay/internal/gateway/router"
	"relay/pkg/models"
)

// TestGatewaySystemIntegration performs comprehensive integration testing
func TestGatewaySystemIntegration(t *testing.T) {
	// Skip if not in integration test mode
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	// Test configuration
	testConfig := createTestGatewayConfig()

	t.Run("ConfigurationLoading", func(t *testing.T) {
		testConfigurationLoading(t, testConfig)
	})

	t.Run("GatewayRegistration", func(t *testing.T) {
		testGatewayRegistration(t, testConfig)
	})

	t.Run("CircuitBreakerFunctionality", func(t *testing.T) {
		testCircuitBreaker(t, testConfig)
	})

	t.Run("RateLimiting", func(t *testing.T) {
		testRateLimiting(t, testConfig)
	})

	t.Run("MessageRouting", func(t *testing.T) {
		testMessageRouting(t, testConfig)
	})

	t.Run("HealthMonitoring", func(t *testing.T) {
		testHealthMonitoring(t, testConfig)
	})

	t.Run("MailgunWebhooks", func(t *testing.T) {
		testMailgunWebhooks(t)
	})

	t.Run("BackwardCompatibility", func(t *testing.T) {
		testBackwardCompatibility(t, testConfig)
	})
}

func createTestGatewayConfig() *config.EnhancedGatewayConfig {
	return &config.EnhancedGatewayConfig{
		Gateways: []config.GatewayConfig{
			{
				ID:          "test-mailgun-primary",
				Type:        gateway.GatewayTypeMailgun,
				DisplayName: "Test Mailgun Primary",
				Domain:      "test.example.com",
				Enabled:     true,
				Priority:    1,
				Weight:      100,
				Mailgun: &config.MailgunConfig{
					APIKey:  "test-api-key",
					Domain:  "test.example.com",
					BaseURL: "https://api.mailgun.net/v3",
					Region:  "us",
					Tracking: config.MailgunTracking{
						Clicks: true,
						Opens:  true,
					},
					Tags: config.MailgunTagsConfig{
						Default:            []string{"test"},
						CampaignTagEnabled: true,
						UserTagEnabled:     true,
					},
				},
				RateLimits: config.GatewayRateLimitConfig{
					WorkspaceDaily: 1000,
					PerUserDaily:   100,
					PerHour:        50,
					BurstLimit:     10,
				},
				Routing: config.GatewayRoutingConfig{
					CanRoute:        []string{"*"},
					ExcludePatterns: []string{"@internal.example.com"},
					FailoverTo:      []string{"test-mailgun-secondary"},
				},
				CircuitBreaker: config.CircuitBreakerConfig{
					Enabled:          true,
					FailureThreshold: 5,
					SuccessThreshold: 3,
					Timeout:          "30s",
					MaxRequests:      10,
				},
			},
			{
				ID:          "test-mailgun-secondary",
				Type:        gateway.GatewayTypeMailgun,
				DisplayName: "Test Mailgun Secondary",
				Domain:      "backup.example.com",
				Enabled:     true,
				Priority:    2,
				Weight:      50,
				Mailgun: &config.MailgunConfig{
					APIKey:  "test-backup-api-key",
					Domain:  "backup.example.com",
					BaseURL: "https://api.mailgun.net/v3",
					Region:  "us",
					Tracking: config.MailgunTracking{
						Clicks: false,
						Opens:  false,
					},
				},
				RateLimits: config.GatewayRateLimitConfig{
					WorkspaceDaily: 500,
					PerUserDaily:   50,
					PerHour:        25,
					BurstLimit:     5,
				},
				Routing: config.GatewayRoutingConfig{
					CanRoute: []string{"*"},
				},
				CircuitBreaker: config.CircuitBreakerConfig{
					Enabled:          true,
					FailureThreshold: 3,
					SuccessThreshold: 2,
					Timeout:          "60s",
				},
			},
		},
		GlobalDefaults: config.GlobalGatewayDefaults{
			RateLimits: config.GatewayRateLimitConfig{
				WorkspaceDaily: 2000,
				PerUserDaily:   200,
				PerHour:        50,
				BurstLimit:     10,
			},
			CircuitBreaker: config.CircuitBreakerConfig{
				Enabled:          true,
				FailureThreshold: 10,
				SuccessThreshold: 5,
				Timeout:          "60s",
			},
			HealthCheck: config.HealthCheckConfig{
				Enabled:          true,
				Interval:         "30s",
				Timeout:          "10s",
				FailureThreshold: 3,
				SuccessThreshold: 2,
			},
			RoutingStrategy: gateway.StrategyPriority,
		},
		Routing: config.GlobalRoutingConfig{
			Strategy:               gateway.StrategyPriority,
			FailoverEnabled:        true,
			LoadBalancingEnabled:   false,
			HealthCheckRequired:    true,
			CircuitBreakerRequired: true,
		},
	}
}

func testConfigurationLoading(t *testing.T, testConfig *config.EnhancedGatewayConfig) {
	t.Log("Testing configuration loading and validation")

	// Test gateway configuration validation
	for _, gatewayConfig := range testConfig.Gateways {
		if err := gatewayConfig.Validate(); err != nil {
			t.Errorf("Gateway config validation failed for %s: %v", gatewayConfig.ID, err)
		}
	}

	// Test effective rate limits calculation
	for _, gatewayConfig := range testConfig.Gateways {
		effectiveLimits := gatewayConfig.GetEffectiveRateLimits(&testConfig.GlobalDefaults)
		if effectiveLimits.WorkspaceDaily == 0 {
			t.Error("Effective workspace daily limit should not be 0")
		}
		if effectiveLimits.PerUserDaily == 0 {
			t.Error("Effective per-user daily limit should not be 0")
		}
	}

	t.Log("Configuration loading tests passed")
}

func testGatewayRegistration(t *testing.T, testConfig *config.EnhancedGatewayConfig) {
	t.Log("Testing gateway registration and management")

	// Create components
	cbManager := reliability.NewCircuitBreakerManager(testConfig.GlobalDefaults.CircuitBreaker)
	rateLimiter := ratelimit.NewMultiGatewayRateLimiter(&testConfig.GlobalDefaults, nil)
	gatewayRouter := router.NewGatewayRouter(testConfig.Routing.Strategy, cbManager)
	gatewayManager := manager.NewGatewayManager(gatewayRouter, rateLimiter, cbManager)

	// Test gateway registration
	for _, gatewayConfig := range testConfig.Gateways {
		// Create Mailgun client
		rateLimit := gateway.RateLimit{
			DailyLimit:   gatewayConfig.RateLimits.WorkspaceDaily,
			PerUserLimit: gatewayConfig.RateLimits.PerUserDaily,
			PerHourLimit: gatewayConfig.RateLimits.PerHour,
			BurstLimit:   gatewayConfig.RateLimits.BurstLimit,
		}

		mailgunClient, err := mailgun.NewMailgunClient(
			gatewayConfig.ID,
			gatewayConfig.Mailgun,
			rateLimit,
			gatewayConfig.Priority,
			gatewayConfig.Weight,
		)
		if err != nil {
			t.Errorf("Failed to create Mailgun client for %s: %v", gatewayConfig.ID, err)
			continue
		}

		// Register with manager
		if err := gatewayManager.RegisterGateway(mailgunClient); err != nil {
			t.Errorf("Failed to register gateway %s: %v", gatewayConfig.ID, err)
			continue
		}

		// Register with router
		if err := gatewayRouter.RegisterGateway(mailgunClient, &gatewayConfig); err != nil {
			t.Errorf("Failed to register gateway with router %s: %v", gatewayConfig.ID, err)
			continue
		}

		// Register with rate limiter
		if err := rateLimiter.RegisterGateway(gatewayConfig.ID, mailgunClient.GetType(), gatewayConfig.RateLimits); err != nil {
			t.Errorf("Failed to register gateway with rate limiter %s: %v", gatewayConfig.ID, err)
			continue
		}
	}

	// Verify registration
	allGateways := gatewayManager.GetAllGateways()
	if len(allGateways) != len(testConfig.Gateways) {
		t.Errorf("Expected %d gateways, got %d", len(testConfig.Gateways), len(allGateways))
	}

	for _, expectedGateway := range testConfig.Gateways {
		gateway, err := gatewayManager.GetGateway(expectedGateway.ID)
		if err != nil {
			t.Errorf("Failed to retrieve gateway %s: %v", expectedGateway.ID, err)
			continue
		}
		if gateway.GetID() != expectedGateway.ID {
			t.Errorf("Gateway ID mismatch: expected %s, got %s", expectedGateway.ID, gateway.GetID())
		}
		if gateway.GetType() != expectedGateway.Type {
			t.Errorf("Gateway type mismatch: expected %s, got %s", expectedGateway.Type, gateway.GetType())
		}
	}

	t.Log("Gateway registration tests passed")
}

func testCircuitBreaker(t *testing.T, testConfig *config.EnhancedGatewayConfig) {
	t.Log("Testing circuit breaker functionality")

	cbConfig := testConfig.GlobalDefaults.CircuitBreaker
	cb, err := reliability.NewCircuitBreaker("test-cb", cbConfig)
	if err != nil {
		t.Fatalf("Failed to create circuit breaker: %v", err)
	}

	// Test initial state
	if cb.GetState() != gateway.CircuitBreakerClosed {
		t.Error("Circuit breaker should start in closed state")
	}

	// Test failure threshold
	for i := 0; i < cbConfig.FailureThreshold; i++ {
		err := cb.Execute(context.Background(), func() error {
			return errors.New("test failure")
		})
		if err == nil {
			t.Error("Expected error from failing function")
		}
	}

	// Should be open now
	if cb.GetState() != gateway.CircuitBreakerOpen {
		t.Error("Circuit breaker should be open after failure threshold")
	}

	// Test that requests are blocked
	err = cb.Execute(context.Background(), func() error {
		return nil
	})
	if err == nil {
		t.Error("Circuit breaker should block requests when open")
	}

	t.Log("Circuit breaker tests passed")
}

func testRateLimiting(t *testing.T, testConfig *config.EnhancedGatewayConfig) {
	t.Log("Testing rate limiting functionality")

	rateLimiter := ratelimit.NewMultiGatewayRateLimiter(&testConfig.GlobalDefaults, nil)

	// Register test gateway
	gatewayConfig := testConfig.Gateways[0]
	err := rateLimiter.RegisterGateway(
		gatewayConfig.ID,
		gatewayConfig.Type,
		gatewayConfig.RateLimits,
	)
	if err != nil {
		t.Fatalf("Failed to register gateway for rate limiting: %v", err)
	}

	testUser := "test@example.com"

	// Test that initial requests are allowed
	result := rateLimiter.Allow(gatewayConfig.ID, testUser)
	if !result.Allowed {
		t.Errorf("First request should be allowed: %s", result.Reason)
	}

	// Record the send
	err = rateLimiter.RecordSend(gatewayConfig.ID, testUser)
	if err != nil {
		t.Errorf("Failed to record send: %v", err)
	}

	// Test rate limit status
	sent, remainingDaily, remainingHourly, resetTime := rateLimiter.GetStatus(gatewayConfig.ID, testUser)
	if sent != 1 {
		t.Errorf("Expected 1 sent message, got %d", sent)
	}
	if remainingDaily != gatewayConfig.RateLimits.PerUserDaily-1 {
		t.Errorf("Expected %d remaining daily, got %d", gatewayConfig.RateLimits.PerUserDaily-1, remainingDaily)
	}
	if remainingHourly != gatewayConfig.RateLimits.PerHour-1 {
		t.Errorf("Expected %d remaining hourly, got %d", gatewayConfig.RateLimits.PerHour-1, remainingHourly)
	}
	if resetTime.IsZero() {
		t.Error("Reset time should not be zero")
	}

	t.Log("Rate limiting tests passed")
}

func testMessageRouting(t *testing.T, testConfig *config.EnhancedGatewayConfig) {
	t.Log("Testing message routing functionality")

	cbManager := reliability.NewCircuitBreakerManager(testConfig.GlobalDefaults.CircuitBreaker)
	gatewayRouter := router.NewGatewayRouter(testConfig.Routing.Strategy, cbManager)

	// Register test gateways
	for _, gatewayConfig := range testConfig.Gateways {
		rateLimit := gateway.RateLimit{
			DailyLimit:   gatewayConfig.RateLimits.WorkspaceDaily,
			PerUserLimit: gatewayConfig.RateLimits.PerUserDaily,
			PerHourLimit: gatewayConfig.RateLimits.PerHour,
			BurstLimit:   gatewayConfig.RateLimits.BurstLimit,
		}

		mailgunClient, err := mailgun.NewMailgunClient(
			gatewayConfig.ID,
			gatewayConfig.Mailgun,
			rateLimit,
			gatewayConfig.Priority,
			gatewayConfig.Weight,
		)
		if err != nil {
			t.Fatalf("Failed to create Mailgun client: %v", err)
		}

		if err := gatewayRouter.RegisterGateway(mailgunClient, &gatewayConfig); err != nil {
			t.Fatalf("Failed to register gateway: %v", err)
		}
	}

	// Test message routing
	testMessage := &models.Message{
		ID:      "test-message-1",
		From:    "sender@example.com",
		To:      []string{"recipient@example.com"},
		Subject: "Test Message",
		HTML:    "<p>Test HTML content</p>",
		Text:    "Test text content",
	}

	selectedGateway, err := gatewayRouter.RouteMessage(context.Background(), testMessage)
	if err != nil {
		t.Fatalf("Failed to route message: %v", err)
	}

	if selectedGateway == nil {
		t.Fatal("No gateway selected for routing")
	}

	// Should select the highest priority gateway (lowest priority number)
	expectedGateway := testConfig.Gateways[0] // Priority 1
	if selectedGateway.GetID() != expectedGateway.ID {
		t.Errorf("Expected gateway %s, got %s", expectedGateway.ID, selectedGateway.GetID())
	}

	// Test excluded patterns
	excludedMessage := &models.Message{
		ID:      "test-message-2",
		From:    "sender@internal.example.com",
		To:      []string{"recipient@example.com"},
		Subject: "Internal Test Message",
	}

	availableGateways := gatewayRouter.GetAvailableGateways(excludedMessage.From)
	// Should find at least one gateway that can handle this (secondary doesn't have exclude patterns)
	if len(availableGateways) == 0 {
		t.Error("Should have at least one available gateway for internal sender")
	}

	t.Log("Message routing tests passed")
}

func testHealthMonitoring(t *testing.T, testConfig *config.EnhancedGatewayConfig) {
	t.Log("Testing health monitoring functionality")

	cbManager := reliability.NewCircuitBreakerManager(testConfig.GlobalDefaults.CircuitBreaker)
	rateLimiter := ratelimit.NewMultiGatewayRateLimiter(&testConfig.GlobalDefaults, nil)
	gatewayRouter := router.NewGatewayRouter(testConfig.Routing.Strategy, cbManager)
	gatewayManager := manager.NewGatewayManager(gatewayRouter, rateLimiter, cbManager)

	// Register a test gateway
	gatewayConfig := testConfig.Gateways[0]
	rateLimit := gateway.RateLimit{
		DailyLimit:   gatewayConfig.RateLimits.WorkspaceDaily,
		PerUserLimit: gatewayConfig.RateLimits.PerUserDaily,
	}

	mailgunClient, err := mailgun.NewMailgunClient(
		gatewayConfig.ID,
		gatewayConfig.Mailgun,
		rateLimit,
		gatewayConfig.Priority,
		gatewayConfig.Weight,
	)
	if err != nil {
		t.Fatalf("Failed to create Mailgun client: %v", err)
	}

	if err := gatewayManager.RegisterGateway(mailgunClient); err != nil {
		t.Fatalf("Failed to register gateway: %v", err)
	}

	// Test health monitoring
	ctx := context.Background()
	gatewayManager.StartHealthMonitoring(ctx, 1*time.Second)
	defer gatewayManager.StopHealthMonitoring()

	// Wait for a health check cycle
	time.Sleep(2 * time.Second)

	// Get system health
	systemHealth := gatewayManager.GetSystemHealth()
	if systemHealth.TotalGateways != 1 {
		t.Errorf("Expected 1 total gateway, got %d", systemHealth.TotalGateways)
	}

	// Gateway should be in some health state
	if len(systemHealth.GatewayHealth) != 1 {
		t.Errorf("Expected 1 gateway health record, got %d", len(systemHealth.GatewayHealth))
	}

	t.Log("Health monitoring tests passed")
}

func testMailgunWebhooks(t *testing.T) {
	t.Log("Testing Mailgun webhook functionality")

	// Create a mock recipient service
	mockRecipientService := &MockRecipientService{}

	webhookHandler := mailgun.NewMailgunWebhookHandler("test-signing-key", mockRecipientService)

	// Test webhook event conversion
	testEvent := &mailgun.MailgunWebhookEvent{
		EventData: mailgun.MailgunEventData{
			Event:     "delivered",
			Timestamp: float64(time.Now().Unix()),
			ID:        "test-event-id",
			Recipient: "test@example.com",
			UserVars: map[string]interface{}{
				"message_id": "test-message-123",
			},
		},
		Signature: mailgun.MailgunSignature{
			Token:     "test-token",
			Timestamp: "1234567890",
			Signature: "test-signature",
		},
	}

	// Test Mandrill event conversion
	mandrillEvent := webhookHandler.ConvertToMandrillEvent(testEvent)
	if mandrillEvent.Event != "send" {
		t.Errorf("Expected 'send' event, got '%s'", mandrillEvent.Event)
	}
	if mandrillEvent.Msg.Email != "test@example.com" {
		t.Errorf("Expected recipient 'test@example.com', got '%s'", mandrillEvent.Msg.Email)
	}
	if mandrillEvent.Msg.ID != "test-message-123" {
		t.Errorf("Expected message ID 'test-message-123', got '%s'", mandrillEvent.Msg.ID)
	}

	t.Log("Mailgun webhook tests passed")
}

func testBackwardCompatibility(t *testing.T, testConfig *config.EnhancedGatewayConfig) {
	t.Log("Testing backward compatibility features")

	// Test legacy workspace configuration conversion
	legacyConfig := &config.GmailConfig{
		Workspaces: []config.WorkspaceConfig{
			{
				ID:                 "legacy-workspace-1",
				Domain:             "legacy.example.com",
				ServiceAccountFile: "path/to/service-account.json",
				DisplayName:        "Legacy Workspace",
				RateLimits: config.WorkspaceRateLimitConfig{
					WorkspaceDaily: 1000,
					PerUserDaily:   100,
				},
			},
		},
	}

	// Test conversion from legacy format
	convertedConfig, err := config.LoadGatewayConfig("test-config.json")
	if err == nil { // If the file exists
		if len(convertedConfig.Gateways) > 0 {
			gatewayConfig := convertedConfig.Gateways[0]
			if gatewayConfig.Type != gateway.GatewayTypeGoogleWorkspace {
				t.Errorf("Expected Google Workspace type, got %s", gatewayConfig.Type)
			}
		}
	}

	// Test migration manager
	migrationManager := migration.NewMigrationManager(
		legacyConfig,
		testConfig,
		migration.MigrationModeCompatibility,
	)

	// Test migration status
	status := migrationManager.GetMigrationStatus()
	if status.Mode != migration.MigrationModeCompatibility {
		t.Errorf("Expected compatibility migration mode, got %s", status.Mode)
	}

	t.Log("Backward compatibility tests passed")
}

// MockRecipientService for testing
type MockRecipientService struct {
	updateDeliveryStatusCalls []UpdateDeliveryStatusCall
	recordEventCalls          []RecordEventCall
}

type UpdateDeliveryStatusCall struct {
	MessageID      string
	RecipientEmail string
	Status         models.DeliveryStatus
	BounceReason   *string
}

type RecordEventCall struct {
	MessageID      string
	RecipientEmail string
	EventType      models.EventType
	EventData      map[string]interface{}
}

func (mrs *MockRecipientService) UpdateDeliveryStatus(messageID, recipientEmail string, status models.DeliveryStatus, bounceReason *string) error {
	mrs.updateDeliveryStatusCalls = append(mrs.updateDeliveryStatusCalls, UpdateDeliveryStatusCall{
		MessageID:      messageID,
		RecipientEmail: recipientEmail,
		Status:         status,
		BounceReason:   bounceReason,
	})
	return nil
}

func (mrs *MockRecipientService) RecordEvent(messageID, recipientEmail string, eventType models.EventType, eventData map[string]interface{}) error {
	mrs.recordEventCalls = append(mrs.recordEventCalls, RecordEventCall{
		MessageID:      messageID,
		RecipientEmail: recipientEmail,
		EventType:      eventType,
		EventData:      eventData,
	})
	return nil
}

// TestGatewayConfigurationValidation tests various configuration scenarios
func TestGatewayConfigurationValidation(t *testing.T) {
	t.Run("ValidMailgunConfig", func(t *testing.T) {
		config := &config.GatewayConfig{
			ID:       "valid-mailgun",
			Type:     gateway.GatewayTypeMailgun,
			Domain:   "example.com",
			Enabled:  true,
			Priority: 1,
			Weight:   100,
			Mailgun: &config.MailgunConfig{
				APIKey:  "valid-key",
				Domain:  "example.com",
				BaseURL: "https://api.mailgun.net/v3",
			},
			CircuitBreaker: config.CircuitBreakerConfig{
				Enabled:          true,
				FailureThreshold: 5,
				SuccessThreshold: 3,
				Timeout:          "60s",
			},
		}

		if err := config.Validate(); err != nil {
			t.Errorf("Valid config should not return error: %v", err)
		}
	})

	t.Run("InvalidMailgunConfig", func(t *testing.T) {
		config := &config.GatewayConfig{
			ID:      "invalid-mailgun",
			Type:    gateway.GatewayTypeMailgun,
			Domain:  "example.com",
			Enabled: true,
			// Missing required Mailgun config
		}

		if err := config.Validate(); err == nil {
			t.Error("Invalid config should return error")
		}
	})

	t.Run("InvalidCircuitBreakerTimeout", func(t *testing.T) {
		config := &config.GatewayConfig{
			ID:      "invalid-cb-timeout",
			Type:    gateway.GatewayTypeMailgun,
			Domain:  "example.com",
			Enabled: true,
			Mailgun: &config.MailgunConfig{
				APIKey: "valid-key",
				Domain: "example.com",
			},
			CircuitBreaker: config.CircuitBreakerConfig{
				Enabled: true,
				Timeout: "invalid-timeout",
			},
		}

		if err := config.Validate(); err == nil {
			t.Error("Config with invalid circuit breaker timeout should return error")
		}
	})
}

// BenchmarkGatewayRouting benchmarks the gateway routing performance
func BenchmarkGatewayRouting(b *testing.B) {
	testConfig := createTestGatewayConfig()
	cbManager := reliability.NewCircuitBreakerManager(testConfig.GlobalDefaults.CircuitBreaker)
	gatewayRouter := router.NewGatewayRouter(testConfig.Routing.Strategy, cbManager)

	// Register test gateways
	for _, gatewayConfig := range testConfig.Gateways {
		rateLimit := gateway.RateLimit{
			DailyLimit:   gatewayConfig.RateLimits.WorkspaceDaily,
			PerUserLimit: gatewayConfig.RateLimits.PerUserDaily,
		}

		mailgunClient, _ := mailgun.NewMailgunClient(
			gatewayConfig.ID,
			gatewayConfig.Mailgun,
			rateLimit,
			gatewayConfig.Priority,
			gatewayConfig.Weight,
		)

		gatewayRouter.RegisterGateway(mailgunClient, &gatewayConfig)
	}

	testMessage := &models.Message{
		ID:      "bench-message",
		From:    "sender@example.com",
		To:      []string{"recipient@example.com"},
		Subject: "Benchmark Message",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := gatewayRouter.RouteMessage(context.Background(), testMessage)
		if err != nil {
			b.Fatalf("Routing failed: %v", err)
		}
	}
}
