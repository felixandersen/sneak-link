package proxy

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sneak-link/config"
	"strings"
)

type ServiceProxy struct {
	proxy  *httputil.ReverseProxy
	target *url.URL
	config *config.ServiceConfig
}

type ProxyManager struct {
	proxies map[string]*ServiceProxy // key = hostname
}

// NewProxyManager creates a new proxy manager for multiple services
func NewProxyManager(services map[string]*config.ServiceConfig) (*ProxyManager, error) {
	proxies := make(map[string]*ServiceProxy)

	for hostname, serviceConfig := range services {
		proxy, err := newServiceProxy(serviceConfig)
		if err != nil {
			return nil, err
		}
		proxies[hostname] = proxy
	}

	return &ProxyManager{
		proxies: proxies,
	}, nil
}

// newServiceProxy creates a new reverse proxy for a specific service
func newServiceProxy(serviceConfig *config.ServiceConfig) (*ServiceProxy, error) {
	target, err := url.Parse(serviceConfig.URL)
	if err != nil {
		return nil, err
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	// Customize the director to handle headers properly
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		
		// Ensure the Host header is set correctly for the backend
		req.Host = target.Host
	}

	// Customize error handler
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		http.Error(w, "Backend service unavailable", http.StatusBadGateway)
	}

	return &ServiceProxy{
		proxy:  proxy,
		target: target,
		config: serviceConfig,
	}, nil
}

// GetProxy returns the proxy for the given hostname
func (pm *ProxyManager) GetProxy(hostname string) *ServiceProxy {
	return pm.proxies[hostname]
}

// ServeHTTP handles the proxy request
func (sp *ServiceProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	sp.proxy.ServeHTTP(w, r)
}

// ValidateShare checks if a share exists using service-specific validation
func (sp *ServiceProxy) ValidateShare(sharePath string) (bool, int, error) {
	serviceType, exists := config.SupportedServices[sp.config.Type]
	if !exists {
		return false, 0, fmt.Errorf("unsupported service type: %s", sp.config.Type)
	}

	switch serviceType.ValidateMethod {
	case "head":
		return sp.validateByHead(sharePath)
	case "immichApi":
		return sp.validateImmichAPI(sharePath)
	default:
		return sp.validateByHead(sharePath) // fallback
	}
}

// validateByHead validates share by making a HEAD request to the share path
func (sp *ServiceProxy) validateByHead(sharePath string) (bool, int, error) {
	shareURL := sp.target.ResolveReference(&url.URL{Path: sharePath})
	
	resp, err := http.Head(shareURL.String())
	if err != nil {
		return false, 0, err
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, resp.StatusCode, nil
}

// validateImmichAPI validates Immich share by calling the API endpoint
func (sp *ServiceProxy) validateImmichAPI(sharePath string) (bool, int, error) {
	// Extract key from /share/xyz789
	key := extractShareKey(sharePath, "/share/")
	if key == "" {
		return false, 400, fmt.Errorf("invalid share path format")
	}

	// Create API URL: /api/shared-links/me?key=xyz789
	apiURL := sp.target.ResolveReference(&url.URL{
		Path:     "/api/shared-links/me",
		RawQuery: "key=" + key,
	})
	
	resp, err := http.Head(apiURL.String())
	if err != nil {
		return false, 0, err
	}
	defer resp.Body.Close()

	// Immich API returns 200 for valid shares, 401 for invalid
	return resp.StatusCode == http.StatusOK, resp.StatusCode, nil
}

// extractShareKey extracts the share key from a share path
func extractShareKey(sharePath, prefix string) string {
	if !strings.HasPrefix(sharePath, prefix) {
		return ""
	}
	
	key := strings.TrimPrefix(sharePath, prefix)
	// Remove any trailing slashes or query parameters
	if idx := strings.Index(key, "/"); idx != -1 {
		key = key[:idx]
	}
	if idx := strings.Index(key, "?"); idx != -1 {
		key = key[:idx]
	}
	
	return key
}

// GetServiceConfig returns the service configuration
func (sp *ServiceProxy) GetServiceConfig() *config.ServiceConfig {
	return sp.config
}
