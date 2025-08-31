package dashboard

import (
	"encoding/json"
	"net/http"
	"time"

	"sneak-link/database"
	"sneak-link/geolocation"
	"sneak-link/logger"
	"sneak-link/metrics"
)

// Server represents the dashboard HTTP server
type Server struct {
	db        *database.DB
	collector *metrics.Collector
	geoSvc    *geolocation.Service
}

// NewServer creates a new dashboard server
func NewServer(db *database.DB, collector *metrics.Collector) *Server {
	return &Server{
		db:        db,
		collector: collector,
		geoSvc:    geolocation.NewService(db),
	}
}

// Start starts the dashboard HTTP server on the specified port
func (s *Server) Start(port string) error {
	mux := http.NewServeMux()
	
	// Static dashboard page
	mux.HandleFunc("/", s.handleDashboard)
	
	// API endpoints
	mux.HandleFunc("/api/stats", s.handleStats)
	mux.HandleFunc("/api/sessions", s.handleSessions)
	mux.HandleFunc("/api/requests", s.handleRecentRequests)
	mux.HandleFunc("/api/security", s.handleSecurityEvents)
	mux.HandleFunc("/api/health", s.handleHealth)
	
	server := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}
	
	logger.Log.WithField("port", port).Info("Dashboard server starting")
	return server.ListenAndServe()
}

// handleDashboard serves the main dashboard HTML page
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(dashboardHTML))
}

// handleStats returns current system statistics
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	stats := s.collector.GetStats()
	
	if err := json.NewEncoder(w).Encode(stats); err != nil {
		http.Error(w, "Failed to encode stats", http.StatusInternalServerError)
		return
	}
}

// handleRecentRequests returns recent HTTP requests
func (s *Server) handleRecentRequests(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	// Get requests from the last hour
	since := time.Now().Add(-1 * time.Hour)
	requests, err := s.db.GetRecentRequests(100, since)
	if err != nil {
		http.Error(w, "Failed to get requests", http.StatusInternalServerError)
		return
	}
	
	if err := json.NewEncoder(w).Encode(requests); err != nil {
		http.Error(w, "Failed to encode requests", http.StatusInternalServerError)
		return
	}
}

// handleSessions returns sessions with activity data
func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	logger.Log.Debug("handleSessions called")
	w.Header().Set("Content-Type", "application/json")
	
	sessions, err := s.db.GetSessionsWithActivity(50)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to get sessions from database")
		http.Error(w, "Failed to get sessions", http.StatusInternalServerError)
		return
	}
	
	logger.Log.WithField("session_count", len(sessions)).Debug("Retrieved sessions from database")
	
	// Populate location data for sessions with IP addresses
	for i := range sessions {
		if sessions[i].LastIP != "" {
			if location, err := s.geoSvc.GetLocation(sessions[i].LastIP); err == nil {
				sessions[i].Location = geolocation.FormatLocation(location)
			} else {
				logger.Log.WithError(err).WithField("ip", sessions[i].LastIP).Debug("Failed to get location for IP")
				sessions[i].Location = "Unknown"
			}
		} else {
			sessions[i].Location = "No activity"
		}
	}
	
	if err := json.NewEncoder(w).Encode(sessions); err != nil {
		logger.Log.WithError(err).Error("Failed to encode sessions to JSON")
		http.Error(w, "Failed to encode sessions", http.StatusInternalServerError)
		return
	}
	
	logger.Log.Debug("handleSessions completed successfully")
}

// handleSecurityEvents returns recent security events
func (s *Server) handleSecurityEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	// Get events from the last 24 hours
	since := time.Now().Add(-24 * time.Hour)
	events, err := s.db.GetRecentSecurityEvents(50, since)
	if err != nil {
		http.Error(w, "Failed to get security events", http.StatusInternalServerError)
		return
	}
	
	if err := json.NewEncoder(w).Encode(events); err != nil {
		http.Error(w, "Failed to encode events", http.StatusInternalServerError)
		return
	}
}

// handleHealth returns health status
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	health := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now(),
		"uptime":    time.Since(time.Now()).Seconds(), // This would be calculated properly
	}
	
	if err := json.NewEncoder(w).Encode(health); err != nil {
		http.Error(w, "Failed to encode health", http.StatusInternalServerError)
		return
	}
}

// dashboardHTML contains the HTML for the dashboard interface
const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Sneak Link Dashboard</title>
    <style>
        :root {
            /* Light theme colors */
            --bg-primary: #f5f5f5;
            --bg-secondary: #ffffff;
            --bg-tertiary: #f8f9fa;
            --text-primary: #333333;
            --text-secondary: #7f8c8d;
            --text-tertiary: #495057;
            --border-color: #ecf0f1;
            --shadow: rgba(0,0,0,0.1);
            --accent-primary: #2c3e50;
            
            /* Status colors */
            --status-active-bg: #d4edda;
            --status-active-text: #155724;
            --status-expired-bg: #f8d7da;
            --status-expired-text: #721c24;
            
            /* Session element colors */
            --session-share-bg: #f1f3f4;
            --session-token-bg: #e8f4f8;
            --session-ip-bg: #fff3cd;
            --session-ip-text: #856404;
        }
        
        [data-theme="dark"] {
            /* Dark theme colors */
            --bg-primary: #1a1a1a;
            --bg-secondary: #2d2d2d;
            --bg-tertiary: #404040;
            --text-primary: #e0e0e0;
            --text-secondary: #b0b0b0;
            --text-tertiary: #c0c0c0;
            --border-color: #404040;
            --shadow: rgba(0,0,0,0.3);
            --accent-primary: #4a90e2;
            
            /* Status colors for dark theme */
            --status-active-bg: #1e4d2b;
            --status-active-text: #4ade80;
            --status-expired-bg: #4d1e1e;
            --status-expired-text: #f87171;
            
            /* Session element colors for dark theme */
            --session-share-bg: #3a3a3a;
            --session-token-bg: #2a4a5a;
            --session-ip-bg: #4a4a2a;
            --session-ip-text: #fbbf24;
        }

        [data-masked] .session-share {
            color: transparent;
            text-shadow: 0 0 15px color-mix(in srgb, var(--text-primary) 50%, transparent);
        }

        [data-masked] .session-ip {
            color: transparent;
            text-shadow: 0 0 15px color-mix(in srgb, var(--session-ip-text) 50%, transparent);
        }

        [data-masked] .session-location {
            color: transparent;
            text-shadow: 0 0 15px color-mix(in srgb, var(--text-tertiary) 50%, transparent);
        }
        
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background-color: var(--bg-primary);
            color: var(--text-primary);
            line-height: 1.5;
            transition: background-color 0.3s ease, color 0.3s ease;
        }
        
        .container {
            margin: 0 auto;
            padding: 20px;
        }
        
        .header {
            background: var(--bg-secondary);
            padding: 15px 20px;
            border-radius: 8px;
            box-shadow: 0 2px 4px var(--shadow);
            margin-bottom: 20px;
            display: flex;
            justify-content: space-between;
            align-items: center;
            transition: background-color 0.3s ease, box-shadow 0.3s ease;
        }
        
        .header-content h1 {
            color: var(--accent-primary);
            margin-bottom: 5px;
            font-size: 24px;
        }
        
        .header-content p {
            color: var(--text-secondary);
            font-size: 14px;
        }
        
        .theme-toggle {
            background: var(--bg-tertiary);
            border: 1px solid var(--border-color);
            border-radius: 6px;
            padding: 8px 12px;
            cursor: pointer;
            font-size: 16px;
            transition: all 0.3s ease;
            color: var(--text-primary);
        }
        
        .theme-toggle:hover {
            background: var(--border-color);
        }
        
        .stats-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 15px;
            margin-bottom: 25px;
        }
        
        .stat-card {
            background: var(--bg-secondary);
            padding: 15px;
            border-radius: 8px;
            box-shadow: 0 2px 4px var(--shadow);
            transition: background-color 0.3s ease, box-shadow 0.3s ease;
        }
        
        .stat-card h3 {
            color: var(--text-secondary);
            font-size: 12px;
            text-transform: uppercase;
            margin-bottom: 8px;
            font-weight: 600;
        }
        
        .stat-value {
            font-size: 24px;
            font-weight: bold;
            color: var(--accent-primary);
        }
        
        .sessions-panel {
            background: var(--bg-secondary);
            border-radius: 8px;
            box-shadow: 0 2px 4px var(--shadow);
            transition: background-color 0.3s ease, box-shadow 0.3s ease;
        }
        
        .panel-header {
            padding: 15px 20px;
            border-bottom: 1px solid var(--border-color);
        }
        
        .panel-header h2 {
            color: var(--accent-primary);
            font-size: 16px;
            font-weight: 600;
        }
        
        .panel-content {
            padding: 0;
        }
        
        .sessions-table {
            width: 100%;
            border-collapse: collapse;
        }
        
        .sessions-table th {
            background-color: var(--bg-tertiary);
            padding: 10px 12px;
            text-align: left;
            font-weight: 600;
            color: var(--text-primary);
            border-bottom: 1px solid var(--border-color);
            font-size: 13px;
        }
        
        .sessions-table td {
            padding: 10px 12px;
            border-bottom: 1px solid var(--border-color);
            vertical-align: middle;
            font-size: 13px;
        }
        
        .sessions-table tr:hover {
            background-color: var(--bg-tertiary);
        }
        
        .session-share {
            font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;
            background-color: var(--session-share-bg);
            padding: 3px 6px;
            border-radius: 3px;
            font-size: 11px;
            color: var(--text-primary);
        }
        
        .session-token {
            font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;
            background-color: var(--session-token-bg);
            padding: 3px 6px;
            border-radius: 3px;
            font-size: 11px;
            color: var(--text-primary);
        }
        
        .session-ip {
            font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;
            background-color: var(--session-ip-bg);
            padding: 3px 6px;
            border-radius: 3px;
            font-size: 11px;
            color: var(--session-ip-text);
        }
        
        .session-location {
            color: var(--text-tertiary);
            font-size: 12px;
        }
        
        .session-service {
            display: inline-block;
            padding: 3px 6px;
            border-radius: 3px;
            font-size: 11px;
            font-weight: 500;
            color: white;
        }
        
        .service-nextcloud { background-color: #0082c9; }
        .service-immich { background-color: #4250a4; }
        .service-paperless { background-color: #2d4a3e; }
        .service-default { background-color: #6c757d; }
        
        .session-status {
            display: inline-block;
            padding: 3px 6px;
            border-radius: 3px;
            font-size: 11px;
            font-weight: 500;
        }
        
        .status-active {
            background-color: var(--status-active-bg);
            color: var(--status-active-text);
        }
        
        .status-expired {
            background-color: var(--status-expired-bg);
            color: var(--status-expired-text);
        }
        
        .request-count {
            font-weight: 600;
            color: var(--text-primary);
            font-size: 13px;
        }
        
        .timestamp {
            color: var(--text-secondary);
            font-size: 12px;
        }
        
        .loading {
            text-align: center;
            color: var(--text-secondary);
            padding: 30px;
            font-size: 14px;
        }
        
        .no-sessions {
            text-align: center;
            color: var(--text-secondary);
            padding: 30px;
            font-size: 14px;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <div class="header-content">
                <h1>🔗 Sneak Link Dashboard</h1>
                <p>Real-time monitoring of your secure link proxy</p>
            </div>
            <button class="theme-toggle" id="theme-toggle" title="Toggle dark mode">
                <span id="theme-icon">🌙</span>
            </button>
        </div>
        
        <div class="stats-grid">
            <div class="stat-card">
                <h3>Total Requests (24h)</h3>
                <div class="stat-value" id="total-requests">-</div>
            </div>
            <div class="stat-card">
                <h3>Request Success Rate</h3>
                <div class="stat-value" id="success-rate">-</div>
            </div>
            <div class="stat-card">
                <h3>Active Sessions</h3>
                <div class="stat-value" id="active-sessions">-</div>
            </div>
            <div class="stat-card">
                <h3>Uptime</h3>
                <div class="stat-value" id="uptime">-</div>
            </div>
        </div>
        
        <div class="sessions-panel">
            <div class="panel-header">
                <h2>Active Sessions</h2>
            </div>
            <div class="panel-content" id="sessions-content">
                <div class="loading">Loading sessions...</div>
            </div>
        </div>
    </div>

    <script>
        // Utility functions
        function formatDuration(seconds) {
            const hours = Math.floor(seconds / 3600);
            const minutes = Math.floor((seconds % 3600) / 60);
            if (hours > 0) {
                return hours + 'h ' + minutes + 'm';
            }
            return minutes + 'm';
        }
        
        function formatTimestamp(timestamp) {
            return new Date(timestamp).toLocaleTimeString();
        }
        
        function getStatusClass(status) {
            if (status >= 200 && status < 300) return 'status-2xx';
            if (status >= 300 && status < 400) return 'status-3xx';
            if (status >= 400 && status < 500) return 'status-4xx';
            return 'status-5xx';
        }
        
        // API calls
        async function fetchStats() {
            try {
                const response = await fetch('/api/stats');
                const stats = await response.json();
                
                document.getElementById('total-requests').textContent = stats.total_requests || 0;
                document.getElementById('active-sessions').textContent = stats.active_sessions || 0;
                document.getElementById('uptime').textContent = formatDuration(stats.uptime_seconds || 0);
                
                const successRate = stats.total_requests > 0 
                    ? Math.round((stats.success_requests / stats.total_requests) * 100) + '%'
                    : '100%';
                document.getElementById('success-rate').textContent = successRate;
            } catch (error) {
                console.error('Failed to fetch stats:', error);
            }
        }
        
        function getServiceClass(service) {
            const serviceLower = service.toLowerCase();
            if (serviceLower.includes('nextcloud')) return 'service-nextcloud';
            if (serviceLower.includes('immich')) return 'service-immich';
            if (serviceLower.includes('paperless')) return 'service-paperless';
            return 'service-default';
        }
        
        function formatRelativeTime(timestamp) {
            if (!timestamp) return 'Never';
            
            const now = new Date();
            const time = new Date(timestamp);
            const diffMs = now - time;
            const diffMins = Math.floor(diffMs / 60000);
            const diffHours = Math.floor(diffMins / 60);
            const diffDays = Math.floor(diffHours / 24);
            
            if (diffMins < 1) return 'Just now';
            if (diffMins < 60) return diffMins + 'm ago';
            if (diffHours < 24) return diffHours + 'h ago';
            return diffDays + 'd ago';
        }
        
        async function fetchSessions() {
            try {
                const response = await fetch('/api/sessions');
                const sessions = await response.json();
                
                const container = document.getElementById('sessions-content');
                
                if (!sessions || sessions.length === 0) {
                    container.innerHTML = '<div class="no-sessions">No active sessions found</div>';
                    return;
                }
                
                const tableHTML = 
                    '<table class="sessions-table">' +
                        '<thead>' +
                            '<tr>' +
                                '<th>Share URL</th>' +
                                '<th>Token</th>' +
                                '<th>Service</th>' +
                                '<th>Status</th>' +
                                '<th>Successful Requests</th>' +
                                '<th>Last IP</th>' +
                                '<th>Location</th>' +
                                '<th>Last Activity</th>' +
                            '</tr>' +
                        '</thead>' +
                        '<tbody>' +
                            sessions.map(session => 
                                '<tr>' +
                                    '<td>' +
                                        '<span class="session-share">' + session.share + '</span>' +
                                    '</td>' +
                                    '<td>' +
                                        '<span class="session-token">' + session.token_hash.substring(0, 8) + '...</span>' +
                                    '</td>' +
                                    '<td>' +
                                        '<span class="session-service ' + getServiceClass(session.service) + '">' + session.service + '</span>' +
                                    '</td>' +
                                    '<td>' +
                                        '<span class="session-status ' + (session.is_active ? 'status-active' : 'status-expired') + '">' +
                                            (session.is_active ? 'Active' : 'Expired') +
                                        '</span>' +
                                    '</td>' +
                                    '<td>' +
                                        '<span class="request-count">' + session.successful_requests + '</span>' +
                                    '</td>' +
                                    '<td>' +
                                        '<span class="session-ip">' + (session.last_ip || 'N/A') + '</span>' +
                                    '</td>' +
                                    '<td>' +
                                        '<span class="session-location">' + (session.location || 'Unknown') + '</span>' +
                                    '</td>' +
                                    '<td>' +
                                        '<span class="timestamp">' + formatRelativeTime(session.last_activity) + '</span>' +
                                    '</td>' +
                                '</tr>'
                            ).join('') +
                        '</tbody>' +
                    '</table>';
                
                container.innerHTML = tableHTML;
            } catch (error) {
                console.error('Failed to fetch sessions:', error);
                document.getElementById('sessions-content').innerHTML = '<div class="loading">Failed to load sessions</div>';
            }
        }
        
        // Theme management
        function initTheme() {
            const savedTheme = localStorage.getItem('dashboard-theme');
            const systemPrefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
            const initialTheme = savedTheme || (systemPrefersDark ? 'dark' : 'light');
            
            setTheme(initialTheme);
        }
        
        function setTheme(theme) {
            const body = document.body;
            const themeIcon = document.getElementById('theme-icon');
            
            if (theme === 'dark') {
                body.setAttribute('data-theme', 'dark');
                themeIcon.textContent = '☀️';
            } else {
                body.removeAttribute('data-theme');
                themeIcon.textContent = '🌙';
            }
            
            localStorage.setItem('dashboard-theme', theme);
        }
        
        function toggleTheme() {
            const currentTheme = document.body.getAttribute('data-theme');
            const newTheme = currentTheme === 'dark' ? 'light' : 'dark';
            setTheme(newTheme);
        }
        
        // Initialize dashboard
        function updateDashboard() {
            fetchStats();
            fetchSessions();
        }
        
        // Event listeners
        document.getElementById('theme-toggle').addEventListener('click', toggleTheme);
        
        // Listen for system theme changes
        window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', (e) => {
            if (!localStorage.getItem('dashboard-theme')) {
                setTheme(e.matches ? 'dark' : 'light');
            }
        });
        
        // Initialize theme and dashboard
        initTheme();
        updateDashboard();
        
        // Auto-refresh every 10 seconds
        setInterval(updateDashboard, 10000);
    </script>
</body>
</html>`
