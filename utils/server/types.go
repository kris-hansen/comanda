package server

import (
	"io"
	"net/http"
	"strings"
	"time"
)

// CORSConfig holds CORS-related configuration options
type CORSConfig struct {
	AllowedOrigins []string `json:"allowedOrigins"`
	AllowedMethods []string `json:"allowedMethods"`
	AllowedHeaders []string `json:"allowedHeaders"`
	MaxAge         int      `json:"maxAge"`
	Enabled        bool     `json:"enabled"`
}

// ServerConfig holds the configuration for the HTTP server
type ServerConfig struct {
	Port        int        `json:"port"`
	DataDir     string     `json:"dataDir"`
	BearerToken string     `json:"bearerToken,omitempty"`
	Enabled     bool       `json:"enabled"`
	CORS        CORSConfig `json:"cors"`
}

// ProcessResponse represents the response for process operations
type ProcessResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
	Output  string `json:"output,omitempty"`
}

// HealthResponse represents the health check response
type HealthResponse struct {
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
}

// FileInfo represents detailed information about a file
type FileInfo struct {
	Name       string    `json:"name"`
	Path       string    `json:"path"`
	Size       int64     `json:"size"`
	IsDir      bool      `json:"isDir"`
	CreatedAt  time.Time `json:"createdAt"`
	ModifiedAt time.Time `json:"modifiedAt"`
	Methods    string    `json:"methods,omitempty"`
}

// FileRequest represents a request to create/edit a file
type FileRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// FileResponse represents a response for file operations
type FileResponse struct {
	Success bool     `json:"success"`
	Message string   `json:"message,omitempty"`
	Error   string   `json:"error,omitempty"`
	File    FileInfo `json:"file,omitempty"`
}

// FileUploadResponse represents a response for file upload operations
type FileUploadResponse struct {
	Success bool     `json:"success"`
	Message string   `json:"message,omitempty"`
	Error   string   `json:"error,omitempty"`
	File    FileInfo `json:"file,omitempty"`
}

// ListResponse represents the response for file listing
type ListResponse struct {
	Success bool       `json:"success"`
	Files   []FileInfo `json:"files"`
	Error   string     `json:"error,omitempty"`
}

// BulkFileRequest represents a request for bulk file operations
type BulkFileRequest struct {
	Files []FileRequest `json:"files"`
}

// BulkFileResponse represents a response for bulk file operations
type BulkFileResponse struct {
	Success bool         `json:"success"`
	Message string       `json:"message,omitempty"`
	Error   string       `json:"error,omitempty"`
	Results []FileResult `json:"results,omitempty"`
}

// FileResult represents the result of a single file operation
type FileResult struct {
	Path    string `json:"path"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// BackupResponse represents a response for backup operations
type BackupResponse struct {
	Success  bool   `json:"success"`
	Message  string `json:"message,omitempty"`
	Error    string `json:"error,omitempty"`
	Filename string `json:"filename,omitempty"`
}

// RestoreRequest represents a request for restore operations
type RestoreRequest struct {
	Backup string `json:"backup"`
}

// ProviderInfo represents information about a provider
type ProviderInfo struct {
	Name    string   `json:"name"`
	Models  []string `json:"models"`
	Enabled bool     `json:"enabled"`
}

// ProviderListResponse represents the response for provider listing
type ProviderListResponse struct {
	Success   bool           `json:"success"`
	Providers []ProviderInfo `json:"providers"`
	Error     string         `json:"error,omitempty"`
}

// ProviderRequest represents a request to modify a provider
type ProviderRequest struct {
	Name    string   `json:"name"`
	APIKey  string   `json:"apiKey"`
	Models  []string `json:"models,omitempty"`
	Enabled bool     `json:"enabled"`
}

// EnvironmentRequest represents a request for environment operations
type EnvironmentRequest struct {
	Password string `json:"password"`
}

// EnvironmentResponse represents a response for environment operations
type EnvironmentResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
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
	s := string(p)
	if strings.HasPrefix(s, "[DEBUG]") || strings.HasPrefix(s, "[VERBOSE]") {
		return w.debug.Write(p)
	}
	return w.output.Write(p)
}
