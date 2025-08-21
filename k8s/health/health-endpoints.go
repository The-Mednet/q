// Enhanced health check endpoints for Kubernetes
// This should be integrated into the existing webui/server.go

package health

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"relay/internal/config"
	"relay/internal/queue"
)

// HealthChecker provides comprehensive health checking capabilities
type HealthChecker struct {
	db           *sql.DB
	queue        queue.Queue
	config       *config.Config
	startTime    time.Time
	dependencies []DependencyChecker
}

// DependencyChecker represents a service dependency health check
type DependencyChecker interface {
	Check(ctx context.Context) error
	Name() string
}

// HealthStatus represents the overall health status
type HealthStatus struct {
	Status       string                       `json:"status"`
	Timestamp    time.Time                    `json:"timestamp"`
	Uptime       time.Duration                `json:"uptime"`
	Version      string                       `json:"version"`
	Dependencies map[string]DependencyStatus  `json:"dependencies"`
	Metrics      HealthMetrics                `json:"metrics"`
}

// DependencyStatus represents a single dependency's health
type DependencyStatus struct {
	Status      string        `json:"status"`
	ResponseTime time.Duration `json:"response_time"`
	Error       string        `json:"error,omitempty"`
	LastCheck   time.Time     `json:"last_check"`
}

// HealthMetrics provides key operational metrics
type HealthMetrics struct {
	QueueDepth          int     `json:"queue_depth"`
	ProcessingRate      float64 `json:"processing_rate_per_sec"`
	ErrorRate           float64 `json:"error_rate_percent"`
	MemoryUsage         int64   `json:"memory_usage_bytes"`
	DatabaseConnections int     `json:"database_connections"`
	GoroutineCount      int     `json:"goroutine_count"`
}

// NewHealthChecker creates a new health checker instance
func NewHealthChecker(db *sql.DB, q queue.Queue, cfg *config.Config) *HealthChecker {
	hc := &HealthChecker{
		db:        db,
		queue:     q,
		config:    cfg,
		startTime: time.Now(),
	}

	// Register dependency checkers
	hc.dependencies = []DependencyChecker{
		NewDatabaseChecker(db),
		NewRedisChecker(cfg), // For distributed rate limiting
		NewGmailChecker(cfg), // For Gmail API connectivity
	}

	return hc
}

// RegisterHealthEndpoints registers health check endpoints with the HTTP mux
func (hc *HealthChecker) RegisterHealthEndpoints(mux *http.ServeMux) {
	// Kubernetes liveness probe - basic health check
	mux.HandleFunc("/health", hc.LivenessProbe)
	
	// Kubernetes readiness probe - comprehensive readiness check
	mux.HandleFunc("/ready", hc.ReadinessProbe)
	
	// Kubernetes startup probe - quick startup verification
	mux.HandleFunc("/startup", hc.StartupProbe)
	
	// Detailed health status for monitoring systems
	mux.HandleFunc("/health/detailed", hc.DetailedHealth)
	
	// Individual dependency checks
	mux.HandleFunc("/health/database", hc.DatabaseHealth)
	mux.HandleFunc("/health/queue", hc.QueueHealth)
	mux.HandleFunc("/health/dependencies", hc.DependenciesHealth)
}

// LivenessProbe - Kubernetes liveness probe
// This checks if the application is alive and should be restarted if it fails
func (hc *HealthChecker) LivenessProbe(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Basic checks that indicate the process is alive
	checks := []func(context.Context) error{
		hc.checkBasicFunctionality,
		hc.checkCriticalResources,
	}

	for _, check := range checks {
		if err := check(ctx); err != nil {
			http.Error(w, "Service unhealthy: "+err.Error(), http.StatusServiceUnavailable)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now(),
		"uptime":    time.Since(hc.startTime),
	})
}

// ReadinessProbe - Kubernetes readiness probe
// This checks if the application is ready to receive traffic
func (hc *HealthChecker) ReadinessProbe(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	status := hc.getDetailedHealthStatus(ctx)
	
	// Determine if we're ready based on critical dependencies
	ready := status.Status == "healthy"
	for name, dep := range status.Dependencies {
		if isCriticalDependency(name) && dep.Status != "healthy" {
			ready = false
			break
		}
	}

	if ready {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"ready":      ready,
		"status":     status.Status,
		"timestamp":  time.Now(),
		"uptime":     time.Since(hc.startTime),
		"dependencies": status.Dependencies,
	})
}

// StartupProbe - Kubernetes startup probe
// This checks if the application has started successfully
func (hc *HealthChecker) StartupProbe(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	// Quick startup checks
	if err := hc.checkBasicFunctionality(ctx); err != nil {
		http.Error(w, "Startup failed: "+err.Error(), http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"started":   true,
		"timestamp": time.Now(),
		"uptime":    time.Since(hc.startTime),
	})
}

// DetailedHealth provides comprehensive health information
func (hc *HealthChecker) DetailedHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	status := hc.getDetailedHealthStatus(ctx)

	w.Header().Set("Content-Type", "application/json")
	if status.Status == "healthy" {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	json.NewEncoder(w).Encode(status)
}

// DatabaseHealth checks database connectivity and performance
func (hc *HealthChecker) DatabaseHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	start := time.Now()
	status := DependencyStatus{
		LastCheck: start,
		Status:    "healthy",
	}

	if err := hc.checkDatabaseHealth(ctx); err != nil {
		status.Status = "unhealthy"
		status.Error = err.Error()
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusOK)
	}

	status.ResponseTime = time.Since(start)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// QueueHealth checks queue status and processing capabilities
func (hc *HealthChecker) QueueHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	start := time.Now()
	status := DependencyStatus{
		LastCheck: start,
		Status:    "healthy",
	}

	if err := hc.checkQueueHealth(ctx); err != nil {
		status.Status = "unhealthy"
		status.Error = err.Error()
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusOK)
	}

	status.ResponseTime = time.Since(start)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// DependenciesHealth checks all external dependencies
func (hc *HealthChecker) DependenciesHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	dependencies := make(map[string]DependencyStatus)
	allHealthy := true

	for _, dep := range hc.dependencies {
		start := time.Now()
		status := DependencyStatus{
			LastCheck: start,
			Status:    "healthy",
		}

		if err := dep.Check(ctx); err != nil {
			status.Status = "unhealthy"
			status.Error = err.Error()
			allHealthy = false
		}

		status.ResponseTime = time.Since(start)
		dependencies[dep.Name()] = status
	}

	if allHealthy {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"all_healthy":    allHealthy,
		"dependencies":   dependencies,
		"timestamp":      time.Now(),
	})
}

// Helper functions for health checks
func (hc *HealthChecker) checkBasicFunctionality(ctx context.Context) error {
	// Check if basic application components are working
	// This could include checking if goroutines are responsive,
	// basic memory allocation works, etc.
	return nil
}

func (hc *HealthChecker) checkCriticalResources(ctx context.Context) error {
	// Check critical system resources like memory, file descriptors, etc.
	// This should catch situations where the process is alive but degraded
	return nil
}

func (hc *HealthChecker) getDetailedHealthStatus(ctx context.Context) HealthStatus {
	status := HealthStatus{
		Status:       "healthy",
		Timestamp:    time.Now(),
		Uptime:       time.Since(hc.startTime),
		Version:      "1.0.0", // Should come from build info
		Dependencies: make(map[string]DependencyStatus),
		Metrics:      hc.getHealthMetrics(),
	}

	// Check all dependencies
	for _, dep := range hc.dependencies {
		start := time.Now()
		depStatus := DependencyStatus{
			LastCheck: start,
			Status:    "healthy",
		}

		if err := dep.Check(ctx); err != nil {
			depStatus.Status = "unhealthy"
			depStatus.Error = err.Error()
			if isCriticalDependency(dep.Name()) {
				status.Status = "degraded"
			}
		}

		depStatus.ResponseTime = time.Since(start)
		status.Dependencies[dep.Name()] = depStatus
	}

	// Additional health checks
	if err := hc.checkDatabaseHealth(ctx); err != nil {
		status.Dependencies["database"] = DependencyStatus{
			Status:    "unhealthy",
			Error:     err.Error(),
			LastCheck: time.Now(),
		}
		status.Status = "unhealthy" // Database is critical
	}

	if err := hc.checkQueueHealth(ctx); err != nil {
		status.Dependencies["queue"] = DependencyStatus{
			Status:    "unhealthy",
			Error:     err.Error(),
			LastCheck: time.Now(),
		}
		status.Status = "degraded" // Queue issues are serious but not immediately fatal
	}

	return status
}

func (hc *HealthChecker) checkDatabaseHealth(ctx context.Context) error {
	if hc.db == nil {
		return nil // Database is optional in some configurations
	}

	// Check database connectivity
	if err := hc.db.PingContext(ctx); err != nil {
		return err
	}

	// Check database performance with a simple query
	var result int
	err := hc.db.QueryRowContext(ctx, "SELECT 1").Scan(&result)
	return err
}

func (hc *HealthChecker) checkQueueHealth(ctx context.Context) error {
	// This would depend on your queue implementation
	// For now, we'll assume the queue is healthy if it exists
	if hc.queue == nil {
		return fmt.Errorf("queue not initialized")
	}
	return nil
}

func (hc *HealthChecker) getHealthMetrics() HealthMetrics {
	// Implement metrics collection based on your application
	// This should integrate with your existing metrics system
	return HealthMetrics{
		QueueDepth:          0, // Get from queue
		ProcessingRate:      0, // Calculate from recent metrics
		ErrorRate:           0, // Calculate from error counters
		MemoryUsage:         0, // Get from runtime
		DatabaseConnections: 0, // Get from database pool
		GoroutineCount:      0, // Get from runtime
	}
}

func isCriticalDependency(name string) bool {
	critical := []string{"database", "mysql", "primary-email-provider"}
	for _, c := range critical {
		if name == c {
			return true
		}
	}
	return false
}