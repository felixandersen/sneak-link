package handlers

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"
	"time"

	"sneak-link/auth"
	"sneak-link/config"
	"sneak-link/logger"
	"sneak-link/metrics"
	"sneak-link/proxy"
	"sneak-link/ratelimit"
)

type Handler struct {
	config       *config.Config
	proxyManager *proxy.ProxyManager
	rateLimiter  *ratelimit.RateLimiter
	collector    *metrics.Collector
}

// NewHandler creates a new request handler
func NewHandler(cfg *config.Config, pm *proxy.ProxyManager, rl *ratelimit.RateLimiter, collector *metrics.Collector) *Handler {
	return &Handler{
		config:       cfg,
		proxyManager: pm,
		rateLimiter:  rl,
		collector:    collector,
	}
}

// ServeHTTP is the main request handler
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	clientIP := getClientIP(r)
	
	// Track in-flight requests
	if h.collector != nil {
		h.collector.IncrementInFlight()
		defer h.collector.DecrementInFlight()
	}

	// Get the service proxy for this hostname
	serviceProxy := h.proxyManager.GetProxy(r.Host)
	if serviceProxy == nil {
		duration := time.Since(start)
		http.Error(w, "Service Not Found", http.StatusNotFound)
		logger.LogAccess(clientIP, r.Method, r.URL.Path, http.StatusNotFound, duration)
		if h.collector != nil {
			h.collector.RecordHTTPRequest(r.Method, "unknown", http.StatusNotFound, duration, clientIP, r.URL.Path, "")
		}
		return
	}

	serviceConfig := serviceProxy.GetServiceConfig()
	serviceName := serviceConfig.Type

	// Get service type configuration
	serviceType, exists := config.SupportedServices[serviceName]
	if !exists {
		duration := time.Since(start)
		http.Error(w, "Unsupported Service", http.StatusInternalServerError)
		logger.LogAccess(clientIP, r.Method, r.URL.Path, http.StatusInternalServerError, duration)
		if h.collector != nil {
			h.collector.RecordHTTPRequest(r.Method, serviceName, http.StatusInternalServerError, duration, clientIP, r.URL.Path, "")
		}
		return
	}

	// For services with full access after knock, check for valid token
	var tokenHash string
	if serviceType.FullAccessAfterKnock {
		if cookie, err := r.Cookie("sneak-link-token"); err == nil {
			if _, err := auth.ValidateToken(cookie.Value, h.config.SigningKey); err == nil {
				// Valid token - proxy the request without rate limiting
				tokenHash = fmt.Sprintf("%x", sha256.Sum256([]byte(cookie.Value)))
				serviceProxy.ServeHTTP(w, r)
				duration := time.Since(start)
				logger.LogAccess(clientIP, r.Method, r.URL.Path, http.StatusOK, duration)
				if h.collector != nil {
					h.collector.RecordHTTPRequest(r.Method, serviceName, http.StatusOK, duration, clientIP, r.URL.Path, tokenHash)
				}
				return
			} else {
				// Invalid token - log security event
				logger.LogSecurity("invalid_token", clientIP, err.Error())
				if h.collector != nil {
					h.collector.RecordSecurityEvent("invalid_token", clientIP, err.Error())
				}
			}
		}
	}

	// Check if this is a share path for this service
	if h.isSharePath(r.URL.Path, serviceType) {
		// Apply rate limiting for unauthenticated requests
		if !h.rateLimiter.IsAllowed(clientIP) {
			details := fmt.Sprintf("requests: %d, window: %v", 
				h.rateLimiter.GetRequestCount(clientIP), 
				h.config.RateLimitWindow)
			
			logger.LogSecurity("rate_limit_exceeded", clientIP, details)
			if h.collector != nil {
				h.collector.RecordSecurityEvent("rate_limit_exceeded", clientIP, details)
			}
			
			duration := time.Since(start)
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			logger.LogAccess(clientIP, r.Method, r.URL.Path, http.StatusTooManyRequests, duration)
			if h.collector != nil {
				h.collector.RecordHTTPRequest(r.Method, serviceName, http.StatusTooManyRequests, duration, clientIP, r.URL.Path, "")
			}
			return
		}

		h.handleShareKnock(w, r, clientIP, start, serviceProxy, serviceType)
		return
	}

	// For services without full access after knock, deny all non-share paths
	if !serviceType.FullAccessAfterKnock {
		duration := time.Since(start)
		http.Error(w, "Access Denied", http.StatusForbidden)
		logger.LogAccess(clientIP, r.Method, r.URL.Path, http.StatusForbidden, duration)
		if h.collector != nil {
			h.collector.RecordHTTPRequest(r.Method, serviceName, http.StatusForbidden, duration, clientIP, r.URL.Path, "")
		}
		return
	}

	// For services with full access after knock, deny access without valid token
	duration := time.Since(start)
	http.Error(w, "Access Denied", http.StatusForbidden)
	logger.LogAccess(clientIP, r.Method, r.URL.Path, http.StatusForbidden, duration)
	if h.collector != nil {
		h.collector.RecordHTTPRequest(r.Method, serviceName, http.StatusForbidden, duration, clientIP, r.URL.Path, "")
	}
}

// isSharePath checks if the given path is a share path for the service
func (h *Handler) isSharePath(path string, serviceType config.ServiceType) bool {
	for _, sharePath := range serviceType.SharePaths {
		if strings.HasPrefix(path, sharePath) {
			return true
		}
	}
	return false
}


// handleShareKnock processes share URL knocks for any service
func (h *Handler) handleShareKnock(w http.ResponseWriter, r *http.Request, clientIP string, start time.Time, serviceProxy *proxy.ServiceProxy, serviceType config.ServiceType) {
	sharePath := r.URL.Path
	serviceConfig := serviceProxy.GetServiceConfig()
	serviceName := serviceConfig.Type

	// Validate the share with the service backend
	valid, status, err := serviceProxy.ValidateShare(sharePath)
	if err != nil {
		duration := time.Since(start)
		logger.Log.WithError(err).Error("Failed to validate share")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		logger.LogAccess(clientIP, r.Method, sharePath, http.StatusInternalServerError, duration)
		if h.collector != nil {
			h.collector.RecordHTTPRequest(r.Method, serviceName, http.StatusInternalServerError, duration, clientIP, sharePath, "")
		}
		return
	}

	logger.LogValidation(clientIP, sharePath, valid, status)
	
	// Record share validation metrics
	if h.collector != nil {
		h.collector.RecordShareValidation(serviceName, valid)
	}

	if !valid {
		// Share doesn't exist or is invalid
		if status == http.StatusNotFound {
			details := fmt.Sprintf("share: %s, service: %s", sharePath, serviceName)
			logger.LogSecurity("invalid_share_attempt", clientIP, details)
			if h.collector != nil {
				h.collector.RecordSecurityEvent("invalid_share_attempt", clientIP, details)
			}
		}
		duration := time.Since(start)
		http.Error(w, "Not Found", http.StatusNotFound)
		logger.LogAccess(clientIP, r.Method, sharePath, http.StatusNotFound, duration)
		if h.collector != nil {
			h.collector.RecordHTTPRequest(r.Method, serviceName, http.StatusNotFound, duration, clientIP, sharePath, "")
		}
		return
	}

	// For services with full access after knock, generate and set authentication token
	var tokenHash string
	if serviceType.FullAccessAfterKnock {
		token, err := auth.GenerateToken(h.config.CookieMaxAge, h.config.SigningKey)
		if err != nil {
			duration := time.Since(start)
			logger.Log.WithError(err).Error("Failed to generate token")
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			logger.LogAccess(clientIP, r.Method, sharePath, http.StatusInternalServerError, duration)
			if h.collector != nil {
				h.collector.RecordHTTPRequest(r.Method, serviceName, http.StatusInternalServerError, duration, clientIP, sharePath, "")
			}
			return
		}

		// Set secure cookie with service-specific domain
		cookie := &http.Cookie{
			Name:     "sneak-link-token",
			Value:    token,
			Domain:   serviceConfig.Domain,
			Path:     "/",
			MaxAge:   int(h.config.CookieMaxAge.Seconds()),
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
		}
		http.SetCookie(w, cookie)
		
		// Record active session
		if h.collector != nil {
			expiresAt := time.Now().Add(h.config.CookieMaxAge)
			h.collector.RecordActiveSession(token, sharePath, serviceName, expiresAt)
		}
		
		// Set token hash for request recording
		tokenHash = fmt.Sprintf("%x", sha256.Sum256([]byte(token)))
	}

	details := fmt.Sprintf("share: %s, service: %s", sharePath, serviceName)
	logger.LogSecurity("access_granted", clientIP, details)
	if h.collector != nil {
		h.collector.RecordSecurityEvent("access_granted", clientIP, details)
	}

	// Proxy the original request to the service
	serviceProxy.ServeHTTP(w, r)
	duration := time.Since(start)
	logger.LogAccess(clientIP, r.Method, sharePath, http.StatusOK, duration)
	if h.collector != nil {
		h.collector.RecordHTTPRequest(r.Method, serviceName, http.StatusOK, duration, clientIP, sharePath, tokenHash)
	}
}

// getClientIP extracts the real client IP from the request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the chain
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	// Fall back to RemoteAddr
	ip := r.RemoteAddr
	if colon := strings.LastIndex(ip, ":"); colon != -1 {
		ip = ip[:colon]
	}
	
	// Remove brackets for IPv6
	ip = strings.Trim(ip, "[]")
	
	return ip
}
