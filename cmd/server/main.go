package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"relay/internal/config"
	"relay/internal/llm"
	"relay/internal/processor"
	"relay/internal/provider"
	"relay/internal/queue"
	"relay/internal/recipient"
	"relay/internal/smtp"
	"relay/internal/webhook"
	"relay/internal/webui"
	"relay/internal/workspace"

	_ "github.com/go-sql-driver/mysql"
)

// LegacyProcessorAdapter adapts the unified processor for WebUI compatibility
type LegacyProcessorAdapter struct {
	unifiedProcessor *processor.UnifiedProcessor
}

func (l *LegacyProcessorAdapter) GetStatus() (bool, time.Time, any) {
	return l.unifiedProcessor.GetStatus()
}

func (l *LegacyProcessorAdapter) GetRateLimitStatus() (int, int, map[string]interface{}) {
	totalSent, workspaceCount, workspaces := l.unifiedProcessor.GetRateLimitStatus()
	
	// Convert workspaces map to interface{} map for compatibility
	workspacesInterface := make(map[string]interface{})
	for k, v := range workspaces {
		workspacesInterface[k] = v
	}
	
	return totalSent, workspaceCount, workspacesInterface
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	var q queue.Queue
	var statsQueue webui.QueueStats
	var recipientService *recipient.Service

	// Try MySQL first, fall back to memory queue
	mysqlQueue, err := queue.NewMySQLQueue(&cfg.MySQL)
	if err != nil {
		log.Printf("Failed to initialize MySQL queue: %v", err)
		log.Println("Using in-memory queue instead")
		memQueue := queue.NewMemoryQueue()
		q = memQueue
		statsQueue = memQueue
		log.Println("Warning: Recipient tracking disabled - requires MySQL database")
	} else {
		q = mysqlQueue
		statsQueue = mysqlQueue
		defer mysqlQueue.Close()

		// Initialize recipient service with the same database connection
		db, err := sql.Open("mysql", cfg.MySQL.GetDSN())
		if err != nil {
			log.Printf("Warning: Failed to initialize recipient service database connection: %v", err)
		} else {
			// Configure connection pool for optimal performance and reliability
			db.SetMaxOpenConns(25)                 // Maximum number of open connections
			db.SetMaxIdleConns(10)                 // Maximum number of idle connections
			db.SetConnMaxLifetime(5 * time.Minute) // Connection lifetime (5 minutes)

			// Test the connection
			if err := db.Ping(); err != nil {
				log.Printf("Warning: Failed to ping recipient service database: %v", err)
				db.Close()
			} else {
				recipientService = recipient.NewService(db)
				log.Println("Recipient tracking service initialized successfully with optimized connection pool")

				// Ensure database cleanup on shutdown
				defer func() {
					if err := db.Close(); err != nil {
						log.Printf("Warning: Error closing recipient service database connection: %v", err)
					}
				}()
			}
		}
	}

	// Initialize workspace manager
	workspaceFile := os.Getenv("WORKSPACE_CONFIG_FILE")
	if workspaceFile == "" {
		workspaceFile = "workspace.json" // Default workspace file
	}
	
	workspaceManager, err := workspace.NewManager(workspaceFile)
	if err != nil {
		log.Fatalf("Failed to initialize workspace manager: %v", err)
	}
	
	// Validate workspace configuration
	if err := workspaceManager.ValidateConfiguration(); err != nil {
		log.Fatalf("Invalid workspace configuration: %v", err)
	}
	
	log.Printf("Workspace manager initialized with %d workspaces", len(workspaceManager.GetWorkspaceIDs()))

	// Initialize provider router
	providerRouter := provider.NewRouter(workspaceManager)
	
	// Initialize all providers based on workspace configuration
	if err := providerRouter.InitializeProviders(); err != nil {
		log.Fatalf("Failed to initialize providers: %v", err)
	}
	
	// Perform initial health checks on all providers
	healthResults := providerRouter.HealthCheckAll(context.Background())
	healthyProviders := 0
	for providerID, healthErr := range healthResults {
		if healthErr == nil {
			healthyProviders++
			log.Printf("Provider %s is healthy", providerID)
		} else {
			log.Printf("Provider %s is unhealthy: %v", providerID, healthErr)
		}
	}
	
	if healthyProviders == 0 {
		log.Printf("Warning: No providers are currently healthy, but continuing startup")
	} else {
		log.Printf("%d out of %d providers are healthy", healthyProviders, len(healthResults))
	}

	// Initialize webhook client
	webhookClient := webhook.NewClient(&cfg.Webhook)

	// Initialize LLM personalizer
	personalizer := llm.NewPersonalizer(&cfg.LLM)
	if personalizer.IsEnabled() {
		log.Println("LLM personalization enabled")
	}

	// Initialize unified processor
	log.Println("Starting with unified email provider system")
	
	unifiedProcessor := processor.NewUnifiedProcessor(
		q, cfg, workspaceManager, providerRouter,
		webhookClient, personalizer, recipientService,
	)

	// SMTP server needs workspace manager for header rewriting
	smtpServer := smtp.NewServer(&cfg.SMTP, q, workspaceManager)
	
	// For WebUI server, create a compatibility processor interface  
	legacyProcessor := &LegacyProcessorAdapter{unifiedProcessor: unifiedProcessor}
	webServer := webui.NewServer(statsQueue, nil, legacyProcessor, recipientService)

	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		if err := smtpServer.Start(); err != nil {
			log.Printf("SMTP server error: %v", err)
		}
	}()

	go func() {
		defer wg.Done()
		if err := webServer.Start(cfg.Server.WebUIPort); err != nil {
			log.Printf("Web UI server error: %v", err)
		}
	}()

	go func() {
		defer wg.Done()
		unifiedProcessor.Start()
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down servers...")
	smtpServer.Stop()
	
	// Stop the unified processor gracefully
	unifiedProcessor.Stop()
	
	// Shutdown provider router
	providerRouter.Shutdown(nil)

	wg.Wait()
	log.Println("Servers stopped")
}
