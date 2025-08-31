package main

import (
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"sneak-link/config"
	"sneak-link/dashboard"
	"sneak-link/database"
	"sneak-link/handlers"
	"sneak-link/logger"
	"sneak-link/metrics"
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

	// Initialize database
	db, err := database.New(cfg.DatabasePath)
	if err != nil {
		logger.Log.WithError(err).Fatal("Failed to initialize database")
	}
	defer db.Close()

	// Initialize metrics collector
	collector := metrics.NewCollector(db)

	// Create proxy manager for all services
	pm, err := proxy.NewProxyManager(cfg.Services)
	if err != nil {
		logger.Log.WithError(err).Fatal("Failed to create proxy manager")
	}

	// Create rate limiter
	rl := ratelimit.NewRateLimiter(cfg.RateLimitRequests, cfg.RateLimitWindow)

	// Create main handler with metrics integration
	handler := handlers.NewHandler(cfg, pm, rl, collector)

	// Start metrics server (Prometheus endpoint)
	go func() {
		if err := metrics.StartMetricsServer(cfg.MetricsPort, collector); err != nil {
			logger.Log.WithError(err).Fatal("Failed to start metrics server")
		}
	}()

	// Start dashboard server
	dashboardServer := dashboard.NewServer(db, collector)
	go func() {
		if err := dashboardServer.Start(cfg.DashboardPort); err != nil {
			logger.Log.WithError(err).Fatal("Failed to start dashboard server")
		}
	}()

	// Start cleanup routine for old data
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		
		for range ticker.C {
			if err := db.CleanupOldData(cfg.MetricsRetentionDays); err != nil {
				logger.Log.WithError(err).Error("Failed to cleanup old data")
			}
		}
	}()

	// Create main HTTP server
	server := &http.Server{
		Addr:    ":" + cfg.ListenPort,
		Handler: handler,
	}

	// Start main server in a goroutine
	go func() {
		logger.Log.WithField("port", cfg.ListenPort).Info("Main server starting")
		
		// Log all configured services
		for hostname, serviceConfig := range cfg.Services {
			logger.Log.WithField("hostname", hostname).
				WithField("service_type", serviceConfig.Type).
				WithField("backend_url", serviceConfig.URL).
				Info("Service configured")
		}
		
		// Log observability endpoints
		logger.Log.WithField("metrics_port", cfg.MetricsPort).Info("Metrics endpoint available at /metrics")
		logger.Log.WithField("dashboard_port", cfg.DashboardPort).Info("Dashboard available at /")
		
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
