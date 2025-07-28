package logger

import (
	"os"
	"time"

	"github.com/sirupsen/logrus"
)

var Log *logrus.Logger

func Init(level string) {
	Log = logrus.New()
	Log.SetOutput(os.Stdout)
	Log.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: time.RFC3339,
	})

	// Set log level
	switch level {
	case "debug":
		Log.SetLevel(logrus.DebugLevel)
	case "info":
		Log.SetLevel(logrus.InfoLevel)
	case "warn":
		Log.SetLevel(logrus.WarnLevel)
	case "error":
		Log.SetLevel(logrus.ErrorLevel)
	default:
		Log.SetLevel(logrus.InfoLevel)
	}
}

// LogAccess logs HTTP access information
func LogAccess(ip, method, path string, status int, duration time.Duration) {
	Log.WithFields(logrus.Fields{
		"type":     "access",
		"ip":       ip,
		"method":   method,
		"path":     path,
		"status":   status,
		"duration": duration.Milliseconds(),
	}).Info("HTTP request")
}

// LogSecurity logs security-related events
func LogSecurity(event, ip, details string) {
	Log.WithFields(logrus.Fields{
		"type":    "security",
		"event":   event,
		"ip":      ip,
		"details": details,
	}).Warn("Security event")
}

// LogValidation logs share validation attempts
func LogValidation(ip, sharePath string, valid bool, status int) {
	Log.WithFields(logrus.Fields{
		"type":       "validation",
		"ip":         ip,
		"share_path": sharePath,
		"valid":      valid,
		"status":     status,
	}).Info("Share validation")
}
