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
	"relay/internal/loadbalancer"
	"relay/internal/processor"
	"relay/internal/provider"
	"relay/internal/queue"
	"relay/internal/recipient"
	"relay/internal/smtp"
	"relay/internal/webhook"
	"relay/internal/webui"
	"relay/internal/workspace"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
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

	// Create a single shared database connection pool
	var sharedDB *sql.DB
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

		// Create shared database connection pool
		sharedDB, err = sql.Open("mysql", cfg.MySQL.GetDSN())
		if err != nil {
			log.Fatalf("Failed to create shared database connection: %v", err)
		}
		
		// Configure connection pool for optimal performance and reliability
		sharedDB.SetMaxOpenConns(50)                 // Increased for shared usage
		sharedDB.SetMaxIdleConns(20)                 // Increased for shared usage
		sharedDB.SetConnMaxLifetime(5 * time.Minute) // Connection lifetime

		// Test the connection
		if err := sharedDB.Ping(); err != nil {
			log.Fatalf("Failed to connect to database: %v", err)
		}
		
		// Ensure database cleanup on shutdown
		defer func() {
			if err := sharedDB.Close(); err != nil {
				log.Printf("Warning: Error closing shared database connection: %v", err)
			}
		}()

		// Initialize recipient service with shared connection
		recipientService = recipient.NewService(sharedDB)
		log.Println("Recipient tracking service initialized successfully")
	}

	// Initialize workspace manager from database
	var workspaceManager *workspace.Manager
	
	// Database is now required for workspace configuration
	if sharedDB == nil {
		log.Fatalf("Database connection is required for workspace configuration")
	}
	
	log.Println("Loading workspace configuration from database")
	workspaceManager, err = workspace.NewDBManager(sharedDB)
	if err != nil {
		log.Fatalf("Failed to load workspaces from database: %v", err)
	}
	
	// Validate workspace configuration
	if err := workspaceManager.ValidateConfiguration(); err != nil {
		log.Fatalf("Invalid workspace configuration: %v", err)
	}
	
	log.Printf("Workspace manager initialized with %d workspaces", len(workspaceManager.GetWorkspaceIDs()))

	// Load balancing initialization will happen after provider router is initialized

	// Initialize credentials loader with shared database connection
	if sharedDB != nil {
		provider.InitCredentialsLoader(sharedDB)
		log.Println("Credentials loader initialized with database support")
	}
	
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

	// Initialize load balancer after provider router (optional - only if database is available)
	if sharedDB != nil {
		// Use shared database connection for load balancer
		// Import sqlx for the load balancer
		dbx := sqlx.NewDb(sharedDB, "mysql")
		
		// Try to load load balancing configuration
		lbConfigLoader := loadbalancer.NewConfigLoader()
		lbConfig, err := lbConfigLoader.LoadFromCommonSources()
		if err != nil {
			log.Printf("No load balancing configuration found: %v", err)
			log.Println("Load balancing disabled - will use direct domain routing only")
			// Create sample configuration file for reference
			if err := lbConfigLoader.SaveSampleConfiguration("load_balancing.json.sample"); err != nil {
				log.Printf("Failed to save sample configuration: %v", err)
			}
		} else if lbConfig.Enabled {
			// Create rate limiter for load balancer
			workspaces := workspaceManager.GetAllWorkspaces()
			lbRateLimiter := queue.NewWorkspaceAwareRateLimiter(workspaces, cfg.Queue.DailyRateLimit)
			
			// Create capacity tracker
			capacityTracker := loadbalancer.NewCapacityTracker(lbRateLimiter, workspaceManager)
			
			// Initialize load balancer
			lb, err := loadbalancer.NewLoadBalancer(
				dbx,
				workspaceManager,
				capacityTracker,
				lbConfig.Config,
			)
			if err != nil {
				log.Printf("Failed to initialize load balancer: %v", err)
			} else {
				// Create pools in the load balancer
				for _, pool := range lbConfig.Pools {
					if err := lb.CreatePool(context.Background(), pool); err != nil {
						log.Printf("Failed to add pool %s: %v", pool.ID, err)
					} else {
						log.Printf("Added load balancing pool: %s (%d workspaces, %d domains)",
							pool.ID, len(pool.Workspaces), len(pool.DomainPatterns))
					}
				}
				log.Printf("Load balancer initialized with %d pools", len(lbConfig.Pools))
				
				// Integrate load balancer with workspace manager for generic domain routing
				workspaceManager.SetLoadBalancer(lb)
			}
		} else {
			log.Println("Load balancing configuration found but disabled")
		}
	} else {
		log.Println("Load balancing disabled - requires MySQL database")
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
	// Load balancer integration will be added through workspace manager
	smtpServer := smtp.NewServer(&cfg.SMTP, q, workspaceManager)
	// TODO: Integrate load balancer with SMTP server for generic domain routing
	
	// For WebUI server, create a compatibility processor interface  
	legacyProcessor := &LegacyProcessorAdapter{unifiedProcessor: unifiedProcessor}
	
	// Pass shared database connection to web server for dashboard API
	var webServer *webui.Server
	if sharedDB != nil {
		webServer = webui.NewServerWithDB(statsQueue, nil, legacyProcessor, recipientService, sharedDB)
		log.Println("Dashboard API endpoints enabled with database connection")
	} else {
		webServer = webui.NewServer(statsQueue, nil, legacyProcessor, recipientService)
	}

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
