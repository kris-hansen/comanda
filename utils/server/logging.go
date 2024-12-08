package server

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/kris-hansen/comanda/utils/config"
)

// logger is a custom logger for HTTP requests, shared across the package
var logger = log.New(os.Stdout, "", log.LstdFlags)

func logRequest(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		// Build auth info string, masking the token
		var authInfo string
		if auth := r.Header.Get("Authorization"); auth != "" {
			authInfo = strings.Replace(auth, auth[7:], "********", 1)
		}

		// Debug level logging - more detailed internal information
		config.DebugLog("Request details:")
		config.DebugLog("- Headers: %v", r.Header)
		config.DebugLog("- Remote Address: %s", r.RemoteAddr)
		config.DebugLog("- TLS: %v", r.TLS != nil)
		config.DebugLog("- Content Length: %d", r.ContentLength)
		config.DebugLog("- Transfer Encoding: %v", r.TransferEncoding)
		config.DebugLog("- Host: %s", r.Host)

		// Verbose level logging - high-level operation information
		config.VerboseLog("Incoming request: %s %s", r.Method, r.URL.String())

		// Call the handler
		handler(wrapped, r)

		// Calculate duration
		duration := time.Since(start)

		// Basic log entry for all requests
		logEntry := fmt.Sprintf("Request: method=%s path=%s query=%s auth=%s status=%d duration=%v",
			r.Method,
			r.URL.Path,
			r.URL.RawQuery,
			authInfo,
			wrapped.statusCode,
			duration)

		// Additional verbose logging
		config.VerboseLog("Response: status=%d bytes=%d duration=%v",
			wrapped.statusCode,
			wrapped.written,
			duration)

		// Additional debug logging for responses
		if wrapped.statusCode >= 400 {
			config.DebugLog("Error response details:")
			config.DebugLog("- Status Code: %d", wrapped.statusCode)
			config.DebugLog("- Bytes Written: %d", wrapped.written)
			config.DebugLog("- Duration: %v", duration)
			config.DebugLog("- Path: %s", r.URL.Path)
			config.DebugLog("- Query: %s", r.URL.RawQuery)
		}

		logger.Print(logEntry)
	}
}
