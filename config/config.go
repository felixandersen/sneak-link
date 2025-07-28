package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"
)

type Config struct {
	NextCloudURL       string
	NextCloudDomain    string
	ListenPort         string
	CookieMaxAge       time.Duration
	RateLimitRequests  int
	RateLimitWindow    time.Duration
	LogLevel           string
	SigningKey         []byte
}

func Load() (*Config, error) {
	// Required environment variables
	nextCloudURL := os.Getenv("NEXTCLOUD_URL")
	if nextCloudURL == "" {
		return nil, fmt.Errorf("NEXTCLOUD_URL environment variable is required")
	}

	signingKey := os.Getenv("SIGNING_KEY")
	if signingKey == "" {
		return nil, fmt.Errorf("SIGNING_KEY environment variable is required")
	}

	// Parse domain from NextCloud URL
	parsedURL, err := url.Parse(nextCloudURL)
	if err != nil {
		return nil, fmt.Errorf("invalid NEXTCLOUD_URL: %v", err)
	}

	// Optional environment variables with defaults
	listenPort := getEnvWithDefault("LISTEN_PORT", "8080")
	
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

	logLevel := getEnvWithDefault("LOG_LEVEL", "info")

	return &Config{
		NextCloudURL:       nextCloudURL,
		NextCloudDomain:    parsedURL.Hostname(),
		ListenPort:         listenPort,
		CookieMaxAge:       time.Duration(cookieMaxAge) * time.Second,
		RateLimitRequests:  rateLimitRequests,
		RateLimitWindow:    time.Duration(rateLimitWindow) * time.Second,
		LogLevel:           logLevel,
		SigningKey:         []byte(signingKey),
	}, nil
}

func getEnvWithDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
