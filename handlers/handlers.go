package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"sneak-link/auth"
	"sneak-link/config"
	"sneak-link/logger"
	"sneak-link/proxy"
	"sneak-link/ratelimit"
)

type Handler struct {
	config      *config.Config
	proxy       *proxy.ReverseProxy
	rateLimiter *ratelimit.RateLimiter
}

// NewHandler creates a new request handler
func NewHandler(cfg *config.Config, rp *proxy.ReverseProxy, rl *ratelimit.RateLimiter) *Handler {
	return &Handler{
		config:      cfg,
		proxy:       rp,
		rateLimiter: rl,
	}
}

// ServeHTTP is the main request handler
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	clientIP := getClientIP(r)

	// Check if user already has a valid token first
	if cookie, err := r.Cookie("sneak-link-token"); err == nil {
		if _, err := auth.ValidateToken(cookie.Value, h.config.SigningKey); err == nil {
			// Valid token - proxy the request without rate limiting
			h.proxy.ServeHTTP(w, r)
			logger.LogAccess(clientIP, r.Method, r.URL.Path, http.StatusOK, time.Since(start))
			return
		} else {
			// Invalid token - log security event
			logger.LogSecurity("invalid_token", clientIP, err.Error())
		}
	}

	// Check if this is a NextCloud share path (URL knock)
	if strings.HasPrefix(r.URL.Path, "/s/") {
		// No valid token - apply rate limiting for unauthenticated requests
		if !h.rateLimiter.IsAllowed(clientIP) {
			logger.LogSecurity("rate_limit_exceeded", clientIP, 
				fmt.Sprintf("requests: %d, window: %v", 
					h.rateLimiter.GetRequestCount(clientIP), 
					h.config.RateLimitWindow))
			
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			logger.LogAccess(clientIP, r.Method, r.URL.Path, http.StatusTooManyRequests, time.Since(start))
			return
		}

		h.handleShareKnock(w, r, clientIP, start)
		return
	}

	// No valid token and not a share path - deny access
	http.Error(w, "Access Denied", http.StatusForbidden)
	logger.LogAccess(clientIP, r.Method, r.URL.Path, http.StatusForbidden, time.Since(start))
}

// handleShareKnock processes NextCloud share URL knocks
func (h *Handler) handleShareKnock(w http.ResponseWriter, r *http.Request, clientIP string, start time.Time) {
	sharePath := r.URL.Path

	// Validate the share with NextCloud backend
	valid, status, err := h.proxy.ValidateShare(sharePath)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to validate share")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		logger.LogAccess(clientIP, r.Method, sharePath, http.StatusInternalServerError, time.Since(start))
		return
	}

	logger.LogValidation(clientIP, sharePath, valid, status)

	if !valid {
		// Share doesn't exist or is invalid
		if status == http.StatusNotFound {
			logger.LogSecurity("invalid_share_attempt", clientIP, fmt.Sprintf("share: %s", sharePath))
		}
		http.Error(w, "Not Found", http.StatusNotFound)
		logger.LogAccess(clientIP, r.Method, sharePath, http.StatusNotFound, time.Since(start))
		return
	}

	// Share is valid - generate and set authentication token
	token, err := auth.GenerateToken(h.config.CookieMaxAge, h.config.SigningKey)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to generate token")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		logger.LogAccess(clientIP, r.Method, sharePath, http.StatusInternalServerError, time.Since(start))
		return
	}

	// Set secure cookie
	cookie := &http.Cookie{
		Name:     "sneak-link-token",
		Value:    token,
		Domain:   h.config.NextCloudDomain,
		Path:     "/",
		MaxAge:   int(h.config.CookieMaxAge.Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}
	http.SetCookie(w, cookie)

	logger.LogSecurity("access_granted", clientIP, fmt.Sprintf("share: %s", sharePath))

	// Proxy the original request to NextCloud
	h.proxy.ServeHTTP(w, r)
	logger.LogAccess(clientIP, r.Method, sharePath, http.StatusOK, time.Since(start))
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
