package metrics

import (
	"net/http"

	"sneak-link/logger"
)

// StartMetricsServer starts the Prometheus metrics HTTP server
func StartMetricsServer(port string, collector *Collector) error {
	mux := http.NewServeMux()
	
	// Prometheus metrics endpoint
	mux.Handle("/metrics", collector.Handler())
	
	// Health check endpoint for metrics server
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
	
	server := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}
	
	logger.Log.WithField("port", port).Info("Metrics server starting")
	return server.ListenAndServe()
}
