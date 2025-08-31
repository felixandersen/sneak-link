package metrics

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"sync"
	"time"

	"sneak-link/database"
	"sneak-link/logger"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Collector holds all Prometheus metrics
type Collector struct {
	db *database.DB
	
	// HTTP metrics
	httpRequestsTotal    *prometheus.CounterVec
	httpRequestDuration  *prometheus.HistogramVec
	httpRequestsInFlight prometheus.Gauge
	
	// Security metrics
	securityEventsTotal  *prometheus.CounterVec
	rateLimitHitsTotal   prometheus.Counter
	
	// Service metrics
	activeSessionsGauge  *prometheus.GaugeVec
	shareValidationsTotal *prometheus.CounterVec
	
	// System metrics
	uptimeSeconds        prometheus.Gauge
	
	// Session tracking
	activeSessions       map[string]time.Time
	sessionsMutex        sync.RWMutex
	
	startTime            time.Time
}

// NewCollector creates a new metrics collector
func NewCollector(db *database.DB) *Collector {
	c := &Collector{
		db:             db,
		activeSessions: make(map[string]time.Time),
		startTime:      time.Now(),
		
		httpRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "sneak_link_http_requests_total",
				Help: "Total number of HTTP requests",
			},
			[]string{"method", "status", "service"},
		),
		
		httpRequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "sneak_link_http_request_duration_seconds",
				Help:    "HTTP request duration in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"method", "service"},
		),
		
		httpRequestsInFlight: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "sneak_link_http_requests_in_flight",
				Help: "Current number of HTTP requests being processed",
			},
		),
		
		securityEventsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "sneak_link_security_events_total",
				Help: "Total number of security events",
			},
			[]string{"event_type"},
		),
		
		rateLimitHitsTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "sneak_link_rate_limit_hits_total",
				Help: "Total number of rate limit hits",
			},
		),
		
		activeSessionsGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "sneak_link_active_sessions",
				Help: "Number of active sessions by service",
			},
			[]string{"service"},
		),
		
		shareValidationsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "sneak_link_share_validations_total",
				Help: "Total number of share validations",
			},
			[]string{"service", "result"},
		),
		
		uptimeSeconds: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "sneak_link_uptime_seconds",
				Help: "Uptime in seconds",
			},
		),
	}
	
	// Register metrics with Prometheus
	prometheus.MustRegister(
		c.httpRequestsTotal,
		c.httpRequestDuration,
		c.httpRequestsInFlight,
		c.securityEventsTotal,
		c.rateLimitHitsTotal,
		c.activeSessionsGauge,
		c.shareValidationsTotal,
		c.uptimeSeconds,
	)
	
	// Start background updater
	go c.updateMetrics()
	
	return c
}

// RecordHTTPRequest records metrics for an HTTP request
func (c *Collector) RecordHTTPRequest(method, service string, status int, duration time.Duration, ip, path, tokenHash string) {
	statusStr := fmt.Sprintf("%d", status)
	
	c.httpRequestsTotal.WithLabelValues(method, statusStr, service).Inc()
	c.httpRequestDuration.WithLabelValues(method, service).Observe(duration.Seconds())
	
	// Store in database for historical data
	if c.db != nil {
		go func() {
			if err := c.db.RecordRequest(ip, method, path, status, duration, service, tokenHash); err != nil {
				logger.Log.WithError(err).Error("Failed to record request in database")
			}
		}()
	}
}

// RecordSecurityEvent records a security event
func (c *Collector) RecordSecurityEvent(eventType, ip, details string) {
	c.securityEventsTotal.WithLabelValues(eventType).Inc()
	
	if eventType == "rate_limit_exceeded" {
		c.rateLimitHitsTotal.Inc()
	}
	
	// Store in database
	if c.db != nil {
		go func() {
			if err := c.db.RecordSecurityEvent(eventType, ip, details); err != nil {
				logger.Log.WithError(err).Error("Failed to record security event in database")
			}
		}()
	}
}

// RecordShareValidation records a share validation attempt
func (c *Collector) RecordShareValidation(service string, valid bool) {
	result := "invalid"
	if valid {
		result = "valid"
	}
	c.shareValidationsTotal.WithLabelValues(service, result).Inc()
}

// RecordActiveSession records a new active session
func (c *Collector) RecordActiveSession(tokenHash, shareURL, service string, expiresAt time.Time) {
	c.sessionsMutex.Lock()
	defer c.sessionsMutex.Unlock()
	
	// Use a hash of the token for tracking (privacy)
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(tokenHash)))
	c.activeSessions[hash] = expiresAt
	
	// Store in database
	if c.db != nil {
		go func() {
			if err := c.db.RecordSession(hash, shareURL, service, expiresAt); err != nil {
				logger.Log.WithError(err).Error("Failed to record session in database")
			}
		}()
	}
}

// IncrementInFlight increments the in-flight requests counter
func (c *Collector) IncrementInFlight() {
	c.httpRequestsInFlight.Inc()
}

// DecrementInFlight decrements the in-flight requests counter
func (c *Collector) DecrementInFlight() {
	c.httpRequestsInFlight.Dec()
}

// updateMetrics runs in the background to update gauge metrics
func (c *Collector) updateMetrics() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	
	for range ticker.C {
		// Update uptime
		c.uptimeSeconds.Set(time.Since(c.startTime).Seconds())
		
		// Clean up expired sessions and update active session counts
		c.updateActiveSessions()
	}
}

// updateActiveSessions cleans up expired sessions and updates gauges
func (c *Collector) updateActiveSessions() {
	c.sessionsMutex.Lock()
	defer c.sessionsMutex.Unlock()
	
	now := time.Now()
	serviceCounts := make(map[string]int)
	
	// Clean up expired sessions
	for hash, expiresAt := range c.activeSessions {
		if now.After(expiresAt) {
			delete(c.activeSessions, hash)
		}
	}
	
	// Count active sessions by service (would need service info stored)
	// For now, just set total active sessions
	totalActive := len(c.activeSessions)
	c.activeSessionsGauge.WithLabelValues("total").Set(float64(totalActive))
	
	// Update individual service counts if we had that data
	for service, count := range serviceCounts {
		c.activeSessionsGauge.WithLabelValues(service).Set(float64(count))
	}
}

// Handler returns the Prometheus metrics HTTP handler
func (c *Collector) Handler() http.Handler {
	return promhttp.Handler()
}

// GetStats returns current metrics for the dashboard
func (c *Collector) GetStats() map[string]interface{} {
	c.sessionsMutex.RLock()
	activeSessions := len(c.activeSessions)
	c.sessionsMutex.RUnlock()
	
	stats := map[string]interface{}{
		"uptime_seconds":    time.Since(c.startTime).Seconds(),
		"active_sessions":   activeSessions,
		"start_time":        c.startTime,
	}
	
	// Get database stats if available
	if c.db != nil {
		since := time.Now().Add(-24 * time.Hour)
		if dbStats, err := c.db.GetRequestStats(since); err == nil {
			for k, v := range dbStats {
				stats[k] = v
			}
		}
	}
	
	return stats
}
