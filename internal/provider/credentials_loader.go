package provider

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
)

// CredentialsLoader handles loading Gmail credentials from various sources
type CredentialsLoader struct {
	db    *sql.DB
	mu    sync.RWMutex
	cache map[string][]byte // Cache credentials by workspace ID
}

var (
	credLoader     *CredentialsLoader
	credLoaderOnce sync.Once
)

// InitCredentialsLoader initializes the global credentials loader
func InitCredentialsLoader(db *sql.DB) {
	credLoaderOnce.Do(func() {
		credLoader = &CredentialsLoader{
			db:    db,
			cache: make(map[string][]byte),
		}
		log.Println("Credentials loader initialized with database support")
	})
}

// GetCredentialsLoader returns the global credentials loader instance
func GetCredentialsLoader() *CredentialsLoader {
	return credLoader
}

// LoadGmailCredentials loads Gmail service account credentials from various sources
// Priority order: 1) Database, 2) Environment variable, 3) File
func (cl *CredentialsLoader) LoadGmailCredentials(workspaceID string, envVar string, filePath string) ([]byte, error) {
	// Check cache first
	cl.mu.RLock()
	if cached, exists := cl.cache[workspaceID]; exists {
		cl.mu.RUnlock()
		log.Printf("Using cached credentials for workspace %s", workspaceID)
		return cached, nil
	}
	cl.mu.RUnlock()

	// Try loading from database first
	if cl.db != nil {
		credentials, err := cl.loadFromDatabase(workspaceID)
		if err == nil && len(credentials) > 0 {
			// Validate it's valid JSON
			var test map[string]interface{}
			if err := json.Unmarshal(credentials, &test); err == nil {
				cl.cacheCredentials(workspaceID, credentials)
				log.Printf("Loaded credentials from database for workspace %s", workspaceID)
				return credentials, nil
			}
		}
	}

	// Try environment variable
	if envVar != "" {
		if envContent := os.Getenv(envVar); envContent != "" {
			credentials := []byte(envContent)
			cl.cacheCredentials(workspaceID, credentials)
			log.Printf("Loaded credentials from environment variable for workspace %s", workspaceID)
			return credentials, nil
		}
	}

	// Try file
	if filePath != "" {
		if fileContent, err := os.ReadFile(filePath); err == nil {
			cl.cacheCredentials(workspaceID, fileContent)
			log.Printf("Loaded credentials from file for workspace %s", workspaceID)
			return fileContent, nil
		}
	}

	return nil, fmt.Errorf("no credentials available for workspace %s", workspaceID)
}

// loadFromDatabase loads credentials from the database
func (cl *CredentialsLoader) loadFromDatabase(workspaceID string) ([]byte, error) {
	if cl.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `
		SELECT service_account_json 
		FROM providers 
		WHERE provider_id = ? AND provider_type = 'gmail' 
		  AND service_account_json IS NOT NULL
	`

	var credentials sql.NullString
	err := cl.db.QueryRow(query, workspaceID).Scan(&credentials)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("no credentials in database for workspace %s", workspaceID)
		}
		return nil, fmt.Errorf("failed to load credentials from database: %v", err)
	}

	if !credentials.Valid || credentials.String == "" {
		return nil, fmt.Errorf("empty credentials in database for workspace %s", workspaceID)
	}

	return []byte(credentials.String), nil
}

// cacheCredentials stores credentials in memory cache
func (cl *CredentialsLoader) cacheCredentials(workspaceID string, credentials []byte) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	cl.cache[workspaceID] = credentials
}

// ClearCache clears the credentials cache for a workspace
func (cl *CredentialsLoader) ClearCache(workspaceID string) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	delete(cl.cache, workspaceID)
}

// ClearAllCache clears all cached credentials
func (cl *CredentialsLoader) ClearAllCache() {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	cl.cache = make(map[string][]byte)
}