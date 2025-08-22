package webui

import (
	"database/sql"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"relay/internal/api"

	"github.com/gorilla/mux"
)

// DashboardServer handles serving the Next.js dashboard and API endpoints
type DashboardServer struct {
	db               *sql.DB
	providersAPI     *api.ProvidersAPI
	poolsAPI         *api.PoolsAPI
	metricsAPI       *api.MetricsAPI
	messagesAPI      *api.MessagesAPI
	dashboardDir     string
	developmentProxy *httputil.ReverseProxy
}

// NewDashboardServer creates a new dashboard server
func NewDashboardServer(db *sql.DB) *DashboardServer {
	ds := &DashboardServer{
		db:           db,
		providersAPI: api.NewProvidersAPI(db),
		poolsAPI:     api.NewPoolsAPI(db),
		metricsAPI:   api.NewMetricsAPI(db),
		messagesAPI:  api.NewMessagesAPI(db),
		dashboardDir: "dashboard/out", // Next.js static export directory
	}

	// In development mode, proxy to Next.js dev server
	if os.Getenv("NODE_ENV") == "development" {
		nextURL, _ := url.Parse("http://localhost:3000")
		ds.developmentProxy = httputil.NewSingleHostReverseProxy(nextURL)
	}

	return ds
}

// RegisterRoutes registers all dashboard and API routes
func (ds *DashboardServer) RegisterRoutes(router *mux.Router) {
	// Register API routes first (more specific)
	ds.providersAPI.RegisterRoutes(router)
	ds.poolsAPI.RegisterRoutes(router)
	ds.metricsAPI.RegisterRoutes(router)
	ds.messagesAPI.RegisterRoutes(router)

	// In development, proxy all dashboard requests to Next.js dev server
	if ds.developmentProxy != nil {
		router.PathPrefix("/dashboard").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ds.developmentProxy.ServeHTTP(w, r)
		})
		return
	}

	// In production, serve static files from the Next.js build
	router.PathPrefix("/dashboard").HandlerFunc(ds.serveDashboard)
}

// serveDashboard serves the static Next.js dashboard files
func (ds *DashboardServer) serveDashboard(w http.ResponseWriter, r *http.Request) {
	// Remove /dashboard prefix
	path := strings.TrimPrefix(r.URL.Path, "/dashboard")
	if path == "" || path == "/" {
		path = "/index.html"
	}

	// Prevent directory traversal
	if strings.Contains(path, "..") {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	// Construct full file path
	fullPath := filepath.Join(ds.dashboardDir, path)

	// Check if file exists
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		// For client-side routing, serve index.html for unknown paths
		if !strings.Contains(path, ".") {
			fullPath = filepath.Join(ds.dashboardDir, "index.html")
		}
	}

	// Set proper MIME types
	ext := strings.ToLower(filepath.Ext(fullPath))
	switch ext {
	case ".css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case ".js":
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	case ".html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	case ".json":
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	case ".png":
		w.Header().Set("Content-Type", "image/png")
	case ".jpg", ".jpeg":
		w.Header().Set("Content-Type", "image/jpeg")
	case ".svg":
		w.Header().Set("Content-Type", "image/svg+xml")
	case ".woff":
		w.Header().Set("Content-Type", "font/woff")
	case ".woff2":
		w.Header().Set("Content-Type", "font/woff2")
	}

	// Set cache headers for static assets
	if ext != ".html" && ext != "" {
		w.Header().Set("Cache-Control", "public, max-age=3600")
	}

	http.ServeFile(w, r, fullPath)
}

