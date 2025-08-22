package workspace

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"relay/internal/config"
)

// DBManager handles workspace configuration from database
type DBManager struct {
	db                *sql.DB
	workspaces        map[string]*config.WorkspaceConfig
	domainToWorkspace map[string]string
	loadBalancer      LoadBalancer
	mu                sync.RWMutex
	lastRefresh       time.Time
}

// NewDBManager creates a new workspace manager that loads from database
func NewDBManager(db *sql.DB) (*Manager, error) {
	dbManager := &DBManager{
		db:                db,
		workspaces:        make(map[string]*config.WorkspaceConfig),
		domainToWorkspace: make(map[string]string),
		lastRefresh:       time.Now(),
	}
	
	// Load initial workspaces from database
	if err := dbManager.loadWorkspacesFromDB(); err != nil {
		return nil, fmt.Errorf("failed to load workspaces from database: %w", err)
	}
	
	// Start background refresh goroutine
	go dbManager.refreshLoop()
	
	// Convert to regular Manager for compatibility
	return dbManager.toManager(), nil
}

// loadWorkspacesFromDB loads all workspaces from the database
func (m *DBManager) loadWorkspacesFromDB() error {
	query := `
		SELECT id, display_name, domain, 
		       rate_limit_workspace_daily, rate_limit_per_user_daily,
		       rate_limit_custom_users, provider_type, provider_config,
		       enabled, service_account_json
		FROM workspaces
		WHERE enabled = 1
		ORDER BY created_at DESC
	`
	
	rows, err := m.db.Query(query)
	if err != nil {
		return fmt.Errorf("failed to query workspaces: %w", err)
	}
	defer rows.Close()
	
	newWorkspaces := make(map[string]*config.WorkspaceConfig)
	newDomainMap := make(map[string]string)
	
	for rows.Next() {
		var ws config.WorkspaceConfig
		var displayName, domain, providerType string
		var workspaceDaily, perUserDaily int
		var customLimits, providerConfig sql.NullString
		var enabled bool
		var serviceAccountJSON sql.NullString
		
		err := rows.Scan(
			&ws.ID, &displayName, &domain,
			&workspaceDaily, &perUserDaily,
			&customLimits, &providerType, &providerConfig,
			&enabled, &serviceAccountJSON,
		)
		if err != nil {
			log.Printf("Error scanning workspace row: %v", err)
			continue
		}
		
		// Set basic fields
		ws.DisplayName = displayName
		ws.Domains = []string{domain}
		ws.Domain = domain
		
		// Set rate limits
		ws.RateLimits.WorkspaceDaily = workspaceDaily
		ws.RateLimits.PerUserDaily = perUserDaily
		
		// Parse custom user limits
		if customLimits.Valid && customLimits.String != "" {
			var limits map[string]int
			if err := json.Unmarshal([]byte(customLimits.String), &limits); err == nil {
				ws.RateLimits.CustomUserLimits = limits
			}
		}
		
		// Parse provider configuration
		if providerConfig.Valid && providerConfig.String != "" {
			switch providerType {
			case "gmail":
				var gmailConfig config.WorkspaceGmailConfig
				if err := json.Unmarshal([]byte(providerConfig.String), &gmailConfig); err == nil {
					// If credentials are in the database, don't use file path
					if serviceAccountJSON.Valid && serviceAccountJSON.String != "" {
						gmailConfig.ServiceAccountFile = "" // Clear file path when using DB credentials
					}
					ws.Gmail = &gmailConfig
				}
			case "mailgun":
				var mailgunConfig config.WorkspaceMailgunConfig
				if err := json.Unmarshal([]byte(providerConfig.String), &mailgunConfig); err == nil {
					ws.Mailgun = &mailgunConfig
				}
			case "mandrill":
				var mandrillConfig config.WorkspaceMandrillConfig
				if err := json.Unmarshal([]byte(providerConfig.String), &mandrillConfig); err == nil {
					ws.Mandrill = &mandrillConfig
				}
			}
		}
		
		newWorkspaces[ws.ID] = &ws
		newDomainMap[domain] = ws.ID
		
		// Also map additional domains if they exist
		for _, d := range ws.Domains {
			newDomainMap[d] = ws.ID
		}
		
		log.Printf("Loaded workspace from DB: ID='%s', Domain='%s', Provider='%s'", 
			ws.ID, domain, providerType)
	}
	
	// Update the maps atomically
	m.mu.Lock()
	m.workspaces = newWorkspaces
	m.domainToWorkspace = newDomainMap
	m.lastRefresh = time.Now()
	m.mu.Unlock()
	
	log.Printf("Successfully loaded %d workspaces from database", len(newWorkspaces))
	return nil
}

// refreshLoop periodically refreshes workspaces from the database
func (m *DBManager) refreshLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for range ticker.C {
		if err := m.loadWorkspacesFromDB(); err != nil {
			log.Printf("Error refreshing workspaces from database: %v", err)
		}
	}
}

// toManager converts DBManager to regular Manager for compatibility
func (m *DBManager) toManager() *Manager {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	manager := &Manager{
		workspaces:        make(map[string]*config.WorkspaceConfig),
		domainToWorkspace: make(map[string]string),
		loadBalancer:      m.loadBalancer,
	}
	
	// Copy workspaces
	for id, ws := range m.workspaces {
		manager.workspaces[id] = ws
	}
	
	// Copy domain mapping
	for domain, id := range m.domainToWorkspace {
		manager.domainToWorkspace[domain] = id
	}
	
	return manager
}

// GetCredentialsFromDB retrieves service account credentials from the database
func GetCredentialsFromDB(db *sql.DB, workspaceID string) ([]byte, error) {
	query := `
		SELECT service_account_json 
		FROM workspaces 
		WHERE id = ? AND provider_type = 'gmail' 
		  AND service_account_json IS NOT NULL
	`
	
	var credentials sql.NullString
	err := db.QueryRow(query, workspaceID).Scan(&credentials)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("no credentials in database for workspace %s", workspaceID)
		}
		return nil, fmt.Errorf("failed to load credentials from database: %w", err)
	}
	
	if !credentials.Valid || credentials.String == "" {
		return nil, fmt.Errorf("empty credentials in database for workspace %s", workspaceID)
	}
	
	return []byte(credentials.String), nil
}