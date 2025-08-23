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
	// Defensive programming: validate configuration loading
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	if cfg == nil {
		log.Fatal("Config is nil after loading")
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
		if memQueue == nil {
			log.Fatal("Failed to create memory queue - both MySQL and memory queues unavailable")
		}
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

	// Initialize workspace manager from database or file
	var workspaceManager *workspace.Manager
	
	// Try database first, then fall back to file-based configuration
	if sharedDB != nil {
		log.Println("Attempting to load workspace configuration from database")
		workspaceManager, err = workspace.NewDBManager(sharedDB)
		if err != nil {
			log.Printf("Failed to load workspaces from database: %v", err)
			log.Println("Falling back to file-based configuration")
		}
		// Defensive check for nil workspace manager from database
		if workspaceManager == nil && err == nil {
			log.Printf("Warning: Database workspace manager creation returned nil without error")
		}
	}
	
	// If database loading failed or no database, try file-based configuration
	if workspaceManager == nil {
		// Try environment variable first
		if envConfig := os.Getenv("WORKSPACE_CONFIG_JSON"); envConfig != "" {
			log.Println("Loading workspace configuration from WORKSPACE_CONFIG_JSON environment variable")
			workspaceManager, err = workspace.NewManagerFromJSON([]byte(envConfig))
			if err != nil {
				log.Fatalf("Failed to load workspaces from environment: %v", err)
			}
		} else if fileConfig := os.Getenv("GMAIL_WORKSPACES_FILE"); fileConfig != "" {
			log.Printf("Loading workspace configuration from file: %s", fileConfig)
			workspaceManager, err = workspace.NewManager(fileConfig)
			if err != nil {
				log.Fatalf("Failed to load workspaces from file: %v", err)
			}
		} else {
			// Try default workspace.json file
			log.Println("Attempting to load workspace configuration from workspace.json")
			workspaceManager, err = workspace.NewManager("workspace.json")
			if err != nil {
				log.Fatalf("No workspace configuration available: %v", err)
			}
		}
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
	if providerRouter == nil {
		log.Fatal("Failed to create provider router")
	}
	
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
		if dbx == nil {
			log.Printf("Warning: Failed to create sqlx database connection for load balancer")
		} else {
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
		}
	} else {
		log.Println("Load balancing disabled - requires MySQL database")
	}

	// Initialize webhook client
	webhookClient := webhook.NewClient(&cfg.Webhook)
	if webhookClient == nil {
		log.Printf("Warning: Webhook client is nil - webhook events disabled")
	}

	// Initialize LLM personalizer
	personalizer := llm.NewPersonalizer(&cfg.LLM)
	if personalizer != nil && personalizer.IsEnabled() {
		log.Println("LLM personalization enabled")
	} else if personalizer == nil {
		log.Printf("Warning: LLM personalizer is nil - personalization disabled")
	}

	// Initialize unified processor
	log.Println("Starting with unified email provider system")
	
	unifiedProcessor := processor.NewUnifiedProcessor(
		q, cfg, workspaceManager, providerRouter,
		webhookClient, personalizer, recipientService,
	)
	if unifiedProcessor == nil {
		log.Fatal("Failed to create unified processor")
	}

	// SMTP server needs workspace manager for header rewriting
	// Load balancer integration will be added through workspace manager
	smtpServer := smtp.NewServer(&cfg.SMTP, q, workspaceManager)
	if smtpServer == nil {
		log.Fatal("Failed to create SMTP server")
	}
	// TODO: Integrate load balancer with SMTP server for generic domain routing
	
	// For WebUI server, create a compatibility processor interface  
	legacyProcessor := &LegacyProcessorAdapter{unifiedProcessor: unifiedProcessor}
	
	// Pass shared database connection to web server for dashboard API
	var webServer *webui.Server
	if sharedDB != nil {
		webServer = webui.NewServerWithDB(statsQueue, nil, legacyProcessor, recipientService, sharedDB)
		if webServer == nil {
			log.Fatal("Failed to create web server with database")
		}
		log.Println("Dashboard API endpoints enabled with database connection")
	} else {
		webServer = webui.NewServer(statsQueue, nil, legacyProcessor, recipientService)
		if webServer == nil {
			log.Fatal("Failed to create web server")
		}
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
