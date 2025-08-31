package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"
)

type ServiceType struct {
	Name                 string
	SharePaths           []string
	ValidateMethod       string
	FullAccessAfterKnock bool // true: set cookie for full app access, false: direct proxy without session
}

var SupportedServices = map[string]ServiceType{
	"nextcloud": {Name: "nextcloud", SharePaths: []string{"/s/"}, ValidateMethod: "head", FullAccessAfterKnock: true},
	"immich":    {Name: "immich", SharePaths: []string{"/share/"}, ValidateMethod: "immichApi", FullAccessAfterKnock: true},
	"paperless": {Name: "paperless", SharePaths: []string{"/share/"}, ValidateMethod: "head", FullAccessAfterKnock: false},
}

type ServiceConfig struct {
	Type   string
	URL    string
	Domain string
}

type Config struct {
	Services          map[string]*ServiceConfig // key = request hostname
	ListenPort        string
	MetricsPort       string
	DashboardPort     string
	DatabasePath      string
	CookieMaxAge      time.Duration
	RateLimitRequests int
	RateLimitWindow   time.Duration
	LogLevel          string
	SigningKey        []byte
	MetricsRetentionDays int
}

func Load() (*Config, error) {
	services := make(map[string]*ServiceConfig)

	// Check for NextCloud
	if nextcloudURL := os.Getenv("NEXTCLOUD_URL"); nextcloudURL != "" {
		config, err := parseServiceConfig("nextcloud", nextcloudURL)
		if err != nil {
			return nil, fmt.Errorf("invalid NEXTCLOUD_URL: %v", err)
		}
		services[config.Domain] = config
	}

	// Check for Immich
	if immichURL := os.Getenv("IMMICH_URL"); immichURL != "" {
		config, err := parseServiceConfig("immich", immichURL)
		if err != nil {
			return nil, fmt.Errorf("invalid IMMICH_URL: %v", err)
		}
		services[config.Domain] = config
	}

	// Check for Paperless-ngx
	if paperlessURL := os.Getenv("PAPERLESS_URL"); paperlessURL != "" {
		config, err := parseServiceConfig("paperless", paperlessURL)
		if err != nil {
			return nil, fmt.Errorf("invalid PAPERLESS_URL: %v", err)
		}
		services[config.Domain] = config
	}

	if len(services) == 0 {
		return nil, fmt.Errorf("at least one service URL must be configured (NEXTCLOUD_URL, IMMICH_URL, or PAPERLESS_URL)")
	}

	signingKey := os.Getenv("SIGNING_KEY")
	if signingKey == "" {
		return nil, fmt.Errorf("SIGNING_KEY environment variable is required")
	}

	// Optional environment variables with defaults
	listenPort := getEnvWithDefault("LISTEN_PORT", "8080")
	metricsPort := getEnvWithDefault("METRICS_PORT", "9090")
	dashboardPort := getEnvWithDefault("DASHBOARD_PORT", "3000")
	databasePath := getEnvWithDefault("DB_PATH", "/data/sneak-link.db")
	
	cookieMaxAgeStr := getEnvWithDefault("COOKIE_MAX_AGE", "86400") // 24 hours
	cookieMaxAge, err := strconv.Atoi(cookieMaxAgeStr)
	if err != nil {
		return nil, fmt.Errorf("invalid COOKIE_MAX_AGE: %v", err)
	}

	rateLimitRequestsStr := getEnvWithDefault("RATE_LIMIT_REQUESTS", "10")
	rateLimitRequests, err := strconv.Atoi(rateLimitRequestsStr)
	if err != nil {
		return nil, fmt.Errorf("invalid RATE_LIMIT_REQUESTS: %v", err)
	}

	rateLimitWindowStr := getEnvWithDefault("RATE_LIMIT_WINDOW", "300") // 5 minutes
	rateLimitWindow, err := strconv.Atoi(rateLimitWindowStr)
	if err != nil {
		return nil, fmt.Errorf("invalid RATE_LIMIT_WINDOW: %v", err)
	}

	metricsRetentionStr := getEnvWithDefault("METRICS_RETENTION_DAYS", "30")
	metricsRetention, err := strconv.Atoi(metricsRetentionStr)
	if err != nil {
		return nil, fmt.Errorf("invalid METRICS_RETENTION_DAYS: %v", err)
	}

	logLevel := getEnvWithDefault("LOG_LEVEL", "info")

	return &Config{
		Services:             services,
		ListenPort:           listenPort,
		MetricsPort:          metricsPort,
		DashboardPort:        dashboardPort,
		DatabasePath:         databasePath,
		CookieMaxAge:         time.Duration(cookieMaxAge) * time.Second,
		RateLimitRequests:    rateLimitRequests,
		RateLimitWindow:      time.Duration(rateLimitWindow) * time.Second,
		LogLevel:             logLevel,
		SigningKey:           []byte(signingKey),
		MetricsRetentionDays: metricsRetention,
	}, nil
}

func parseServiceConfig(serviceType, serviceURL string) (*ServiceConfig, error) {
	parsedURL, err := url.Parse(serviceURL)
	if err != nil {
		return nil, err
	}

	return &ServiceConfig{
		Type:   serviceType,
		URL:    serviceURL,
		Domain: parsedURL.Hostname(),
	}, nil
}

func getEnvWithDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
