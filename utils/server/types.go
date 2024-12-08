package server

import (
	"io"
	"net/http"
	"strings"
)

type ProcessResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
	Output  string `json:"output,omitempty"`
}

type HealthResponse struct {
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
}

type YAMLFileInfo struct {
	Name    string `json:"name"`
	Methods string `json:"methods"` // "GET" or "POST"
}

type ListResponse struct {
	Success bool           `json:"success"`
	Files   []YAMLFileInfo `json:"files"`
	Error   string         `json:"error,omitempty"`
}

// responseWriter wraps http.ResponseWriter to capture the status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    int64
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.written += int64(n)
	return n, err
}

// filteringWriter is a custom writer that filters debug and verbose messages
type filteringWriter struct {
	output io.Writer // For actual output
	debug  io.Writer // For debug/verbose messages
}

func (w *filteringWriter) Write(p []byte) (n int, err error) {
	// Convert to string for easier handling
	s := string(p)

	// Check if this is a debug or verbose message
	if strings.HasPrefix(s, "[DEBUG]") || strings.HasPrefix(s, "[VERBOSE]") {
		return w.debug.Write(p)
	}

	// This is actual output, write to both
	return w.output.Write(p)
}

// ServerConfig holds the configuration for the HTTP server
type ServerConfig struct {
	Port        int
	DataDir     string
	BearerToken string
	Enabled     bool
}
