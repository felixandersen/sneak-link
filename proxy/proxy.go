package proxy

import (
	"net/http"
	"net/http/httputil"
	"net/url"
)

type ReverseProxy struct {
	proxy  *httputil.ReverseProxy
	target *url.URL
}

// NewReverseProxy creates a new reverse proxy to the target URL
func NewReverseProxy(targetURL string) (*ReverseProxy, error) {
	target, err := url.Parse(targetURL)
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

	return &ReverseProxy{
		proxy:  proxy,
		target: target,
	}, nil
}

// ServeHTTP handles the proxy request
func (rp *ReverseProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rp.proxy.ServeHTTP(w, r)
}

// ValidateShare checks if a NextCloud share exists by making a HEAD request
func (rp *ReverseProxy) ValidateShare(sharePath string) (bool, int, error) {
	// Create URL for the share validation
	shareURL := rp.target.ResolveReference(&url.URL{Path: sharePath})
	
	// Make HEAD request to check if share exists
	resp, err := http.Head(shareURL.String())
	if err != nil {
		return false, 0, err
	}
	defer resp.Body.Close()

	// Return true if status is 200, false for 404, and the actual status code
	return resp.StatusCode == http.StatusOK, resp.StatusCode, nil
}
