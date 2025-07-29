package main

import (
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"sneak-link/config"
	"sneak-link/handlers"
	"sneak-link/logger"
	"sneak-link/proxy"
	"sneak-link/ratelimit"
)

func main() {
	// Read version from VERSION file
	versionBytes, err := os.ReadFile("VERSION")
	version := "unknown"
	if err == nil {
		version = strings.TrimSpace(string(versionBytes))
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	logger.Init(cfg.LogLevel)
	logger.Log.WithField("version", version).Info("Starting Sneak Link server")

	// Create proxy manager for all services
	pm, err := proxy.NewProxyManager(cfg.Services)
	if err != nil {
		logger.Log.WithError(err).Fatal("Failed to create proxy manager")
	}

	// Create rate limiter
	rl := ratelimit.NewRateLimiter(cfg.RateLimitRequests, cfg.RateLimitWindow)

	// Create main handler
	handler := handlers.NewHandler(cfg, pm, rl)

	// Create HTTP server
	server := &http.Server{
		Addr:    ":" + cfg.ListenPort,
		Handler: handler,
	}

	// Start server in a goroutine
	go func() {
		logger.Log.WithField("port", cfg.ListenPort).Info("Server starting")
		
		// Log all configured services
		for hostname, serviceConfig := range cfg.Services {
			logger.Log.WithField("hostname", hostname).
				WithField("service_type", serviceConfig.Type).
				WithField("backend_url", serviceConfig.URL).
				Info("Service configured")
		}
		
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Log.WithError(err).Fatal("Server failed to start")
		}
	}()

	// Wait for interrupt signal to gracefully shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Log.Info("Shutting down server...")
	
	// Graceful shutdown would go here if needed
	// For now, just exit
	logger.Log.Info("Server stopped")
}
