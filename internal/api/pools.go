package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

type PoolsAPI struct {
	db *sql.DB
}

func NewPoolsAPI(db *sql.DB) *PoolsAPI {
	return &PoolsAPI{db: db}
}

type LoadBalancingPool struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Algorithm      string     `json:"algorithm"`
	Providers      []string   `json:"providers"`
	DomainPatterns []string   `json:"domain_patterns,omitempty"`
	Enabled        bool       `json:"enabled"`
	Stats          *PoolStats `json:"stats,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type PoolStats struct {
	TotalRequests      int64 `json:"total_requests"`
	SuccessfulRequests int64 `json:"successful_requests"`
	FailedRequests     int64 `json:"failed_requests"`
}

type LoadBalancingSelection struct {
	PoolID     string    `json:"pool_id"`
	ProviderID string    `json:"provider_id"`
	Success    bool      `json:"success"`
	Timestamp  time.Time `json:"timestamp"`
}

func (api *PoolsAPI) RegisterRoutes(router *mux.Router) {
	router.HandleFunc("/api/loadbalancing/pools", api.ListPools).Methods("GET")
	router.HandleFunc("/api/loadbalancing/pools", api.CreatePool).Methods("POST")
	router.HandleFunc("/api/loadbalancing/pools/{id}", api.GetPool).Methods("GET")
	router.HandleFunc("/api/loadbalancing/pools/{id}", api.UpdatePool).Methods("PUT")
	router.HandleFunc("/api/loadbalancing/pools/{id}", api.DeletePool).Methods("DELETE")
	router.HandleFunc("/api/loadbalancing/selections", api.GetSelections).Methods("GET")
}

func (api *PoolsAPI) ListPools(w http.ResponseWriter, r *http.Request) {
	query := `
		SELECT p.id, p.name, p.strategy, p.enabled, p.domain_patterns, p.created_at, p.updated_at,
		       COALESCE(SUM(s.total_requests), 0) as total_requests,
		       COALESCE(SUM(s.successful_requests), 0) as successful_requests,
		       COALESCE(SUM(s.failed_requests), 0) as failed_requests
		FROM load_balancing_pools p
		LEFT JOIN pool_statistics s ON p.id = s.pool_id
		GROUP BY p.id, p.name, p.strategy, p.enabled, p.domain_patterns, p.created_at, p.updated_at
		ORDER BY p.created_at DESC
	`

	rows, err := api.db.Query(query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var pools []LoadBalancingPool
	for rows.Next() {
		var pool LoadBalancingPool
		stats := &PoolStats{}
		var domainPatternsJSON sql.NullString

		err := rows.Scan(
			&pool.ID, &pool.Name, &pool.Algorithm, &pool.Enabled,
			&domainPatternsJSON,
			&pool.CreatedAt, &pool.UpdatedAt,
			&stats.TotalRequests, &stats.SuccessfulRequests, &stats.FailedRequests,
		)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		pool.Stats = stats

		// Parse domain patterns JSON
		if domainPatternsJSON.Valid {
			json.Unmarshal([]byte(domainPatternsJSON.String), &pool.DomainPatterns)
		}

		// Get pool members
		memberQuery := `
			SELECT workspace_id FROM pool_members 
			WHERE pool_id = ? AND enabled = true
			ORDER BY priority, weight DESC
		`
		memberRows, err := api.db.Query(memberQuery, pool.ID)
		if err == nil {
			defer memberRows.Close()
			for memberRows.Next() {
				var workspaceID string
				if err := memberRows.Scan(&workspaceID); err == nil {
					pool.Providers = append(pool.Providers, workspaceID)
				}
			}
		}

		pools = append(pools, pool)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"pools": pools,
	})
}

func (api *PoolsAPI) CreatePool(w http.ResponseWriter, r *http.Request) {
	var req LoadBalancingPool
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.ID == "" {
		req.ID = uuid.NewString()
	}

	tx, err := api.db.Begin()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// Create the pool
	poolQuery := `
		INSERT INTO load_balancing_pools (id, name, strategy, enabled)
		VALUES (?, ?, ?, ?)
	`
	_, err = tx.Exec(poolQuery, req.ID, req.Name, req.Algorithm, req.Enabled)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Add pool members
	for i, providerID := range req.Providers {
		memberQuery := `
			INSERT INTO pool_members (pool_id, workspace_id, weight, priority, enabled)
			VALUES (?, ?, ?, ?, ?)
		`
		_, err = tx.Exec(memberQuery, req.ID, providerID, 1, i, true)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	if err = tx.Commit(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	req.CreatedAt = time.Now()
	req.UpdatedAt = time.Now()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(req)
}

func (api *PoolsAPI) GetPool(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	query := `
		SELECT id, name, strategy, enabled, created_at, updated_at
		FROM load_balancing_pools
		WHERE id = ?
	`

	var pool LoadBalancingPool
	err := api.db.QueryRow(query, id).Scan(
		&pool.ID, &pool.Name, &pool.Algorithm, &pool.Enabled,
		&pool.CreatedAt, &pool.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		http.Error(w, "Pool not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Get pool members
	memberQuery := `
		SELECT workspace_id FROM pool_members 
		WHERE pool_id = ? AND enabled = true
		ORDER BY priority, weight DESC
	`
	rows, err := api.db.Query(memberQuery, pool.ID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var workspaceID string
			if err := rows.Scan(&workspaceID); err == nil {
				pool.Providers = append(pool.Providers, workspaceID)
			}
		}
	}

	// Get statistics
	statsQuery := `
		SELECT COALESCE(SUM(total_requests), 0), 
		       COALESCE(SUM(successful_requests), 0),
		       COALESCE(SUM(failed_requests), 0)
		FROM pool_statistics
		WHERE pool_id = ?
	`
	stats := &PoolStats{}
	api.db.QueryRow(statsQuery, pool.ID).Scan(
		&stats.TotalRequests,
		&stats.SuccessfulRequests,
		&stats.FailedRequests,
	)
	pool.Stats = stats

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pool)
}

func (api *PoolsAPI) UpdatePool(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var req LoadBalancingPool
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	tx, err := api.db.Begin()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// Update the pool
	poolQuery := `
		UPDATE load_balancing_pools SET
			name = ?, strategy = ?, enabled = ?, updated_at = NOW()
		WHERE id = ?
	`
	result, err := tx.Exec(poolQuery, req.Name, req.Algorithm, req.Enabled, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, "Pool not found", http.StatusNotFound)
		return
	}

	// Update pool members
	// First, disable all existing members
	_, err = tx.Exec("UPDATE pool_members SET enabled = false WHERE pool_id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Then add/update the new members
	for i, providerID := range req.Providers {
		memberQuery := `
			INSERT INTO pool_members (pool_id, workspace_id, weight, priority, enabled)
			VALUES (?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE
				weight = VALUES(weight),
				priority = VALUES(priority),
				enabled = VALUES(enabled)
		`
		_, err = tx.Exec(memberQuery, id, providerID, 1, i, true)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	if err = tx.Commit(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	req.ID = id
	req.UpdatedAt = time.Now()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(req)
}

func (api *PoolsAPI) DeletePool(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	query := `DELETE FROM load_balancing_pools WHERE id = ?`
	result, err := api.db.Exec(query, id)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, "Pool not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (api *PoolsAPI) GetSelections(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit := 20
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	query := `
		SELECT pool_id, workspace_id, success, created_at
		FROM provider_selections
		ORDER BY created_at DESC
		LIMIT ?
	`

	rows, err := api.db.Query(query, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var selections []LoadBalancingSelection
	for rows.Next() {
		var sel LoadBalancingSelection
		err := rows.Scan(&sel.PoolID, &sel.ProviderID, &sel.Success, &sel.Timestamp)
		if err != nil {
			continue
		}
		selections = append(selections, sel)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"selections": selections,
	})
}

