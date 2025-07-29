package main

import (
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"smtp_relay/internal/config"
	"smtp_relay/internal/gmail"
	"smtp_relay/internal/llm"
	"smtp_relay/internal/processor"
	"smtp_relay/internal/queue"
	"smtp_relay/internal/smtp"
	"smtp_relay/internal/webhook"
	"smtp_relay/internal/webui"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	var q queue.Queue
	var statsQueue webui.QueueStats

	// Try MySQL first, fall back to memory queue
	mysqlQueue, err := queue.NewMySQLQueue(&cfg.MySQL)
	if err != nil {
		log.Printf("Failed to initialize MySQL queue: %v", err)
		log.Println("Using in-memory queue instead")
		memQueue := queue.NewMemoryQueue()
		q = memQueue
		statsQueue = memQueue
	} else {
		q = mysqlQueue
		statsQueue = mysqlQueue
		defer mysqlQueue.Close()
	}

	// Initialize Gmail client
	gmailClient, err := gmail.NewClient(&cfg.Gmail)
	if err != nil {
		log.Printf("Warning: Failed to initialize Gmail client: %v", err)
		log.Println("Gmail sending will not be available until credentials are configured")
	}

	// Initialize webhook client
	webhookClient := webhook.NewClient(&cfg.Webhook)

	// Initialize LLM personalizer
	personalizer := llm.NewPersonalizer(&cfg.LLM)
	if personalizer.IsEnabled() {
		log.Println("LLM personalization enabled")
	}

	// Initialize queue processor
	queueProcessor := processor.NewQueueProcessor(q, cfg, gmailClient, webhookClient, personalizer)

	// Get workspace router from Gmail client for SMTP server
	var workspaceRouter *gmail.WorkspaceRouter
	if gmailClient != nil {
		workspaceRouter = gmailClient.GetRouter()
	}

	smtpServer := smtp.NewServer(&cfg.SMTP, q, workspaceRouter)
	webServer := webui.NewServer(statsQueue, gmailClient, queueProcessor)

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
		queueProcessor.Start()
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down servers...")
	smtpServer.Stop()

	wg.Wait()
	log.Println("Servers stopped")
}
