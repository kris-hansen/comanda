package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	cfg "github.com/kris-hansen/comanda/utils/config" // Added alias cfg
)

// debugLog provides local logging to avoid circular imports
func debugLog(format string, args ...interface{}) {
	log.Printf("[DEBUG] "+format, args...)
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

// YAMLRequest represents a request for YAML operations
type YAMLRequest struct {
	Content   string `json:"content"`
	Input     string `json:"input"`
	Streaming bool   `json:"streaming"`
}

// flushingResponseWriter implements http.Flusher interface
type flushingResponseWriter struct {
	http.ResponseWriter
	flusher http.Flusher
}

func (fw *flushingResponseWriter) Flush() {
	fw.flusher.Flush()
}

// responseWriter wraps http.ResponseWriter to capture the status code and implement http.Flusher
type responseWriter struct {
	http.ResponseWriter
	statusCode  int
	written     int64
	headersSent bool
}

func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.headersSent {
		rw.statusCode = code
		rw.ResponseWriter.WriteHeader(code)
		rw.headersSent = true
	} else if code >= 400 && rw.statusCode < 400 {
		// Allow error status codes to override success codes
		rw.statusCode = code
		rw.ResponseWriter.WriteHeader(code)
	}
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.headersSent {
		// If no status has been set before first write, use 200 OK
		rw.WriteHeader(http.StatusOK)
	}
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

// sseWriter is a custom writer that formats output as Server-Sent Events
type sseWriter struct {
	w http.ResponseWriter
	f http.Flusher
}

func (sw *sseWriter) Write(p []byte) (n int, err error) {
	debugLog("[SSE] Write called with %d bytes", len(p))
	return sw.SendData(string(p))
}

func (sw *sseWriter) SendData(data string) (n int, err error) {
	debugLog("[SSE] Sending data event, length=%d", len(data))
	event := fmt.Sprintf("event: data\ndata: %s\n\n", data)
	n, err = sw.w.Write([]byte(event))
	if err != nil {
		debugLog("[SSE] Error writing data event: %v", err)
		return
	}
	sw.f.Flush()
	debugLog("[SSE] Successfully sent data event: bytes=%d", n)
	return
}

// --- Model Management API Types ---

// ConfiguredModel represents a model as configured within Comanda
type ConfiguredModel struct {
	Name  string          `json:"name"`
	Type  string          `json:"type"`  // e.g., "local", "external"
	Modes []cfg.ModelMode `json:"modes"` // Use alias cfg
}

// AvailableModel represents a model available from the provider's service
type AvailableModel struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"` // Optional description if available
}

// AvailableModelListResponse is the response for listing available models from a provider
type AvailableModelListResponse struct {
	Success bool             `json:"success"`
	Models  []AvailableModel `json:"models"`
	Error   string           `json:"error,omitempty"`
}

// ConfiguredModelListResponse is the response for listing models configured for a provider
type ConfiguredModelListResponse struct {
	Success bool              `json:"success"`
	Models  []ConfiguredModel `json:"models"`
	Error   string            `json:"error,omitempty"`
}

// AddModelRequest is the request body for adding a model to a provider's configuration
type AddModelRequest struct {
	Name  string          `json:"name"`
	Modes []cfg.ModelMode `json:"modes"` // Use alias cfg
}

// UpdateModelRequest is the request body for updating a configured model's modes
type UpdateModelRequest struct {
	Modes []cfg.ModelMode `json:"modes"` // Use alias cfg
}

// SuccessResponse represents a generic successful API response
type SuccessResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// ErrorResponse represents a generic error API response
type ErrorResponse struct {
	Success bool   `json:"success"` // Should always be false
	Error   string `json:"error"`
}

func (sw *sseWriter) SendProgress(data interface{}) (n int, err error) {
	debugLog("[SSE] Sending progress event")
	var eventData string
	switch v := data.(type) {
	case string:
		eventData = v
	default:
		jsonData, err := json.Marshal(data)
		if err != nil {
			debugLog("[SSE] Error marshaling progress data: %v", err)
			return 0, err
		}
		eventData = string(jsonData)
	}
	event := fmt.Sprintf("event: progress\ndata: %s\n\n", eventData)
	n, err = sw.w.Write([]byte(event))
	if err != nil {
		debugLog("[SSE] Error writing progress event: %v", err)
		return
	}
	sw.f.Flush()
	debugLog("[SSE] Successfully sent progress event: bytes=%d", n)
	return
}

func (sw *sseWriter) SendSpinner(msg string) (n int, err error) {
	debugLog("[SSE] Sending spinner event: %s", msg)
	event := fmt.Sprintf("event: spinner\ndata: %s\n\n", msg)
	n, err = sw.w.Write([]byte(event))
	if err != nil {
		debugLog("[SSE] Error writing spinner event: %v", err)
		return
	}
	sw.f.Flush()
	debugLog("[SSE] Successfully sent spinner event: bytes=%d", n)
	return
}

func (sw *sseWriter) SendComplete(msg string) (n int, err error) {
	debugLog("[SSE] Sending complete event: %s", msg)
	event := fmt.Sprintf("event: complete\ndata: %s\n\n", msg)
	n, err = sw.w.Write([]byte(event))
	if err != nil {
		debugLog("[SSE] Error writing complete event: %v", err)
		return
	}
	sw.f.Flush()
	debugLog("[SSE] Successfully sent complete event: bytes=%d", n)
	return
}

func (sw *sseWriter) SendError(err error) (n int, error error) {
	debugLog("[SSE] Sending error event: %v", err)
	data := map[string]interface{}{
		"success": false,
		"error":   err.Error(),
	}
	jsonData, _ := json.Marshal(data)
	event := fmt.Sprintf("event: error\ndata: %s\n\n", string(jsonData))
	n, error = sw.w.Write([]byte(event))
	if error != nil {
		debugLog("[SSE] Error writing error event: %v", error)
		return
	}
	sw.f.Flush()
	debugLog("[SSE] Successfully sent error event: bytes=%d", n)
	return
}

func (sw *sseWriter) SendHeartbeat() (n int, err error) {
	debugLog("[SSE] Sending heartbeat event")
	event := ": heartbeat\n\n"
	n, err = sw.w.Write([]byte(event))
	if err != nil {
		debugLog("[SSE] Error writing heartbeat event: %v", err)
		return
	}
	sw.f.Flush()
	debugLog("[SSE] Successfully sent heartbeat event: bytes=%d", n)
	return
}

// SendOutput sends an output event with the given content
func (sw *sseWriter) SendOutput(content string) (n int, err error) {
	debugLog("[SSE] Sending output event with content length: %d", len(content))
	data := map[string]string{
		"content": content,
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		debugLog("[SSE] Error marshaling output data: %v", err)
		return 0, err
	}
	event := fmt.Sprintf("event: output\ndata: %s\n\n", string(jsonData))
	n, err = sw.w.Write([]byte(event))
	if err != nil {
		debugLog("[SSE] Error writing output event: %v", err)
		return
	}
	sw.f.Flush()
	debugLog("[SSE] Successfully sent output event: bytes=%d", n)
	return
}
