package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"sneak-link/logger"

	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	conn *sql.DB
}

type RequestRecord struct {
	ID        int64     `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	IP        string    `json:"ip"`
	Method    string    `json:"method"`
	Path      string    `json:"path"`
	Status    int       `json:"status"`
	Duration  int64     `json:"duration_ms"`
	Service   string    `json:"service"`
}

type SecurityEvent struct {
	ID        int64     `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	EventType string    `json:"event_type"`
	IP        string    `json:"ip"`
	Details   string    `json:"details"`
}


type SessionRecord struct {
	ID        int64     `json:"id"`
	TokenHash string    `json:"token_hash"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Service   string    `json:"service"`
}

// New creates a new database connection and initializes the schema
func New(dbPath string) (*DB, error) {
	// Ensure the directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %v", err)
	}

	conn, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_synchronous=NORMAL&_cache_size=1000")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %v", err)
	}

	db := &DB{conn: conn}
	
	if err := db.initSchema(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to initialize schema: %v", err)
	}

	logger.Log.WithField("path", dbPath).Info("Database initialized")
	return db, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.conn.Close()
}

// initSchema creates the database tables
func (db *DB) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS requests (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		ip TEXT NOT NULL,
		method TEXT NOT NULL,
		path TEXT NOT NULL,
		status INTEGER NOT NULL,
		duration_ms INTEGER NOT NULL,
		service TEXT NOT NULL,
		token_hash TEXT
	);

	CREATE TABLE IF NOT EXISTS security_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		event_type TEXT NOT NULL,
		ip TEXT NOT NULL,
		details TEXT
	);


	CREATE TABLE IF NOT EXISTS sessions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		token_hash TEXT NOT NULL UNIQUE,
		share_url TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		expires_at DATETIME NOT NULL,
		service TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS ip_locations (
		ip TEXT PRIMARY KEY,
		country TEXT,
		country_code TEXT,
		region TEXT,
		city TEXT,
		latitude REAL,
		longitude REAL,
		timezone TEXT,
		isp TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	-- Indexes for better query performance
	CREATE INDEX IF NOT EXISTS idx_requests_timestamp ON requests(timestamp);
	CREATE INDEX IF NOT EXISTS idx_requests_ip ON requests(ip);
	CREATE INDEX IF NOT EXISTS idx_requests_service ON requests(service);
	CREATE INDEX IF NOT EXISTS idx_requests_token_hash ON requests(token_hash);
	CREATE INDEX IF NOT EXISTS idx_security_events_timestamp ON security_events(timestamp);
	CREATE INDEX IF NOT EXISTS idx_security_events_ip ON security_events(ip);
	CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);
	CREATE INDEX IF NOT EXISTS idx_sessions_token_hash ON sessions(token_hash);
	CREATE INDEX IF NOT EXISTS idx_ip_locations_updated_at ON ip_locations(updated_at);
	`

	_, err := db.conn.Exec(schema)
	return err
}

// RecordRequest stores an HTTP request record
func (db *DB) RecordRequest(ip, method, path string, status int, duration time.Duration, service, tokenHash string) error {
	query := `
		INSERT INTO requests (ip, method, path, status, duration_ms, service, token_hash)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	_, err := db.conn.Exec(query, ip, method, path, status, duration.Milliseconds(), service, tokenHash)
	return err
}

// RecordSecurityEvent stores a security event
func (db *DB) RecordSecurityEvent(eventType, ip, details string) error {
	query := `
		INSERT INTO security_events (event_type, ip, details)
		VALUES (?, ?, ?)
	`
	_, err := db.conn.Exec(query, eventType, ip, details)
	return err
}


// RecordSession stores a session record
func (db *DB) RecordSession(tokenHash, shareURL, service string, expiresAt time.Time) error {
	query := `
		INSERT INTO sessions (token_hash, share_url, service, expires_at)
		VALUES (?, ?, ?, ?)
	`
	_, err := db.conn.Exec(query, tokenHash, shareURL, service, expiresAt)
	return err
}

// GetRecentRequests returns recent HTTP requests
func (db *DB) GetRecentRequests(limit int, since time.Time) ([]RequestRecord, error) {
	query := `
		SELECT id, timestamp, ip, method, path, status, duration_ms, service
		FROM requests
		WHERE timestamp >= ?
		ORDER BY timestamp DESC
		LIMIT ?
	`
	
	rows, err := db.conn.Query(query, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []RequestRecord
	for rows.Next() {
		var r RequestRecord
		err := rows.Scan(&r.ID, &r.Timestamp, &r.IP, &r.Method, &r.Path, &r.Status, &r.Duration, &r.Service)
		if err != nil {
			return nil, err
		}
		records = append(records, r)
	}

	return records, rows.Err()
}

// GetRecentSecurityEvents returns recent security events
func (db *DB) GetRecentSecurityEvents(limit int, since time.Time) ([]SecurityEvent, error) {
	query := `
		SELECT id, timestamp, event_type, ip, details
		FROM security_events
		WHERE timestamp >= ?
		ORDER BY timestamp DESC
		LIMIT ?
	`
	
	rows, err := db.conn.Query(query, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []SecurityEvent
	for rows.Next() {
		var e SecurityEvent
		err := rows.Scan(&e.ID, &e.Timestamp, &e.EventType, &e.IP, &e.Details)
		if err != nil {
			return nil, err
		}
		events = append(events, e)
	}

	return events, rows.Err()
}

// GetRequestStats returns aggregated request statistics
func (db *DB) GetRequestStats(since time.Time) (map[string]interface{}, error) {
	query := `
		SELECT 
			COUNT(*) as total_requests,
			COUNT(CASE WHEN status >= 200 AND status < 300 THEN 1 END) as success_requests,
			COUNT(CASE WHEN status >= 400 THEN 1 END) as error_requests,
			AVG(duration_ms) as avg_duration,
			COUNT(DISTINCT ip) as unique_ips,
			COUNT(DISTINCT service) as active_services
		FROM requests
		WHERE timestamp >= ?
	`
	
	row := db.conn.QueryRow(query, since)
	
	var totalRequests, successRequests, errorRequests, uniqueIPs, activeServices int
	var avgDuration float64
	
	err := row.Scan(&totalRequests, &successRequests, &errorRequests, &avgDuration, &uniqueIPs, &activeServices)
	if err != nil {
		return nil, err
	}

	stats := map[string]interface{}{
		"total_requests":   totalRequests,
		"success_requests": successRequests,
		"error_requests":   errorRequests,
		"avg_duration_ms":  avgDuration,
		"unique_ips":       uniqueIPs,
		"active_services":  activeServices,
	}

	return stats, nil
}

// SessionWithActivity represents a session with aggregated activity data
type SessionWithActivity struct {
	ID               int64     `json:"id"`
	TokenHash        string    `json:"token_hash"`
	Share            string    `json:"share"`
	Service          string    `json:"service"`
	CreatedAt        time.Time `json:"created_at"`
	ExpiresAt        time.Time `json:"expires_at"`
	SuccessfulReqs   int       `json:"successful_requests"`
	LastActivity     *time.Time `json:"last_activity"`
	LastIP           string    `json:"last_ip"`
	Location         string    `json:"location"`
	IsActive         bool      `json:"is_active"`
}

// GetSessionsWithActivity returns sessions with their activity metrics
func (db *DB) GetSessionsWithActivity(limit int) ([]SessionWithActivity, error) {
	logger.Log.WithField("limit", limit).Debug("GetSessionsWithActivity called")
	
	query := `
		SELECT 
			s.id,
			s.token_hash,
			s.share_url,
			s.service,
			s.created_at,
			s.expires_at,
			COALESCE(r.successful_requests, 0) as successful_requests,
			r.last_activity,
			COALESCE(r.last_ip, '') as last_ip,
			CASE WHEN s.expires_at > datetime('now') THEN 1 ELSE 0 END as is_active
		FROM sessions s
		LEFT JOIN (
			SELECT 
				token_hash,
				COUNT(CASE WHEN status >= 200 AND status < 300 THEN 1 END) as successful_requests,
				MAX(timestamp) as last_activity,
				(SELECT ip FROM requests r2 WHERE r2.token_hash = requests.token_hash ORDER BY timestamp DESC LIMIT 1) as last_ip
			FROM requests
			WHERE token_hash IS NOT NULL
			GROUP BY token_hash
		) r ON s.token_hash = r.token_hash
		ORDER BY 
			CASE WHEN s.expires_at > datetime('now') THEN 0 ELSE 1 END,
			COALESCE(r.last_activity, s.created_at) DESC
		LIMIT ?
	`
	
	logger.Log.Debug("Executing sessions query")
	rows, err := db.conn.Query(query, limit)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to execute sessions query")
		return nil, err
	}
	defer rows.Close()

	var sessions []SessionWithActivity
	rowCount := 0
	for rows.Next() {
		rowCount++
		var s SessionWithActivity
		var lastActivityStr sql.NullString
		
		err := rows.Scan(
			&s.ID, &s.TokenHash, &s.Share, &s.Service, 
			&s.CreatedAt, &s.ExpiresAt, &s.SuccessfulReqs, 
			&lastActivityStr, &s.LastIP, &s.IsActive,
		)
		if err != nil {
			logger.Log.WithError(err).WithField("row", rowCount).Error("Failed to scan session row")
			return nil, err
		}
		
		// Parse the last_activity timestamp from string if it exists
		if lastActivityStr.Valid && lastActivityStr.String != "" {
			// SQLite stores timestamps in RFC3339 format by default
			if parsedTime, parseErr := time.Parse("2006-01-02 15:04:05", lastActivityStr.String); parseErr == nil {
				s.LastActivity = &parsedTime
			} else if parsedTime, parseErr := time.Parse(time.RFC3339, lastActivityStr.String); parseErr == nil {
				s.LastActivity = &parsedTime
			} else {
				logger.Log.WithError(parseErr).WithField("timestamp", lastActivityStr.String).Warn("Failed to parse last_activity timestamp")
			}
		}
		
		// Set location to empty for now - will be populated by dashboard
		s.Location = ""
		
		sessions = append(sessions, s)
	}

	if err := rows.Err(); err != nil {
		logger.Log.WithError(err).Error("Error iterating over session rows")
		return nil, err
	}

	logger.Log.WithField("session_count", len(sessions)).Debug("GetSessionsWithActivity completed successfully")
	return sessions, nil
}

// CleanupOldData removes old records based on retention policy
func (db *DB) CleanupOldData(retentionDays int) error {
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	
	tables := []string{"requests", "security_events"}
	
	for _, table := range tables {
		query := fmt.Sprintf("DELETE FROM %s WHERE timestamp < ?", table)
		result, err := db.conn.Exec(query, cutoff)
		if err != nil {
			return fmt.Errorf("failed to cleanup %s: %v", table, err)
		}
		
		rowsAffected, _ := result.RowsAffected()
		if rowsAffected > 0 {
			logger.Log.WithField("table", table).WithField("rows_deleted", rowsAffected).Info("Cleaned up old data")
		}
	}

	// Clean up expired sessions
	_, err := db.conn.Exec("DELETE FROM sessions WHERE expires_at < ?", time.Now())
	if err != nil {
		return fmt.Errorf("failed to cleanup expired sessions: %v", err)
	}

	return nil
}

// GetCachedLocation retrieves cached location data from database
func (db *DB) GetCachedLocation(ip string) (*LocationInfo, error) {
	query := `
		SELECT ip, country, country_code, region, city, latitude, longitude, timezone, isp
		FROM ip_locations 
		WHERE ip = ? AND updated_at > datetime('now', '-7 days')
	`
	
	row := db.conn.QueryRow(query, ip)
	
	var location LocationInfo
	err := row.Scan(
		&location.IP, &location.Country, &location.CountryCode,
		&location.Region, &location.City, &location.Latitude,
		&location.Longitude, &location.Timezone, &location.ISP,
	)
	
	if err != nil {
		return nil, err
	}
	
	return &location, nil
}

// CacheLocation stores location data in the database
func (db *DB) CacheLocation(ip, country, countryCode, region, city string, latitude, longitude float64, timezone, isp string) error {
	query := `
		INSERT OR REPLACE INTO ip_locations 
		(ip, country, country_code, region, city, latitude, longitude, timezone, isp, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
	`
	
	_, err := db.conn.Exec(query, ip, country, countryCode, region, city, latitude, longitude, timezone, isp)
	return err
}

// LocationInfo represents geolocation data for an IP address (for database methods)
type LocationInfo struct {
	IP          string  `json:"query"`
	Country     string  `json:"country"`
	CountryCode string  `json:"countryCode"`
	Region      string  `json:"regionName"`
	City        string  `json:"city"`
	Latitude    float64 `json:"lat"`
	Longitude   float64 `json:"lon"`
	Timezone    string  `json:"timezone"`
	ISP         string  `json:"isp"`
}
