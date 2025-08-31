package geolocation

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"sneak-link/database"
	"sneak-link/logger"
)

// LocationInfo represents geolocation data for an IP address
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
	Status      string  `json:"status"`
}

// Service handles IP geolocation lookups with caching
type Service struct {
	db     *database.DB
	client *http.Client
}

// NewService creates a new geolocation service
func NewService(db *database.DB) *Service {
	return &Service{
		db: db,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// GetLocation returns location information for an IP address
// Uses cached data if available, otherwise fetches from ip-api.com
func (s *Service) GetLocation(ip string) (*LocationInfo, error) {
	// Skip private/local IPs
	if isPrivateIP(ip) {
		return &LocationInfo{
			IP:      ip,
			Country: "Local",
			City:    "Private Network",
		}, nil
	}

	// Check cache first
	if cached, err := s.getCachedLocation(ip); err == nil && cached != nil {
		return cached, nil
	}

	// Fetch from API
	location, err := s.fetchFromAPI(ip)
	if err != nil {
		logger.Log.WithError(err).WithField("ip", ip).Warn("Failed to fetch geolocation")
		return &LocationInfo{
			IP:      ip,
			Country: "Unknown",
			City:    "Unknown",
		}, nil
	}

	// Cache the result
	if err := s.cacheLocation(location); err != nil {
		logger.Log.WithError(err).WithField("ip", ip).Warn("Failed to cache geolocation")
	}

	return location, nil
}

// fetchFromAPI fetches location data from ip-api.com
func (s *Service) fetchFromAPI(ip string) (*LocationInfo, error) {
	url := fmt.Sprintf("http://ip-api.com/json/%s", ip)
	
	resp, err := s.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch geolocation: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("geolocation API returned status %d", resp.StatusCode)
	}

	var location LocationInfo
	if err := json.NewDecoder(resp.Body).Decode(&location); err != nil {
		return nil, fmt.Errorf("failed to decode geolocation response: %v", err)
	}

	if location.Status != "success" {
		return nil, fmt.Errorf("geolocation API returned status: %s", location.Status)
	}

	return &location, nil
}

// getCachedLocation retrieves cached location data from database
func (s *Service) getCachedLocation(ip string) (*LocationInfo, error) {
	dbLocation, err := s.db.GetCachedLocation(ip)
	if err != nil {
		return nil, err
	}
	
	// Convert database.LocationInfo to geolocation.LocationInfo
	return &LocationInfo{
		IP:          dbLocation.IP,
		Country:     dbLocation.Country,
		CountryCode: dbLocation.CountryCode,
		Region:      dbLocation.Region,
		City:        dbLocation.City,
		Latitude:    dbLocation.Latitude,
		Longitude:   dbLocation.Longitude,
		Timezone:    dbLocation.Timezone,
		ISP:         dbLocation.ISP,
	}, nil
}

// cacheLocation stores location data in the database
func (s *Service) cacheLocation(location *LocationInfo) error {
	return s.db.CacheLocation(location.IP, location.Country, location.CountryCode,
		location.Region, location.City, location.Latitude, location.Longitude,
		location.Timezone, location.ISP)
}

// isPrivateIP checks if an IP address is private/local
func isPrivateIP(ip string) bool {
	// Simple check for common private IP ranges
	if ip == "127.0.0.1" || ip == "::1" || ip == "localhost" {
		return true
	}
	
	// Check for private IPv4 ranges (simplified)
	if len(ip) >= 7 {
		if ip[:4] == "192." || ip[:3] == "10." || ip[:4] == "172." {
			return true
		}
	}
	
	return false
}

// FormatLocation returns a human-readable location string
func FormatLocation(location *LocationInfo) string {
	if location == nil {
		return "Unknown"
	}
	
	if location.Country == "Local" {
		return "Local Network"
	}
	
	if location.City != "" && location.Country != "" {
		return fmt.Sprintf("%s, %s", location.City, location.Country)
	}
	
	if location.Country != "" {
		return location.Country
	}
	
	return "Unknown"
}
