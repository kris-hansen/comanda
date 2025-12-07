package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kris-hansen/comanda/utils/config"
)

// Server represents the HTTP server
type Server struct {
	mux       *http.ServeMux
	config    *config.ServerConfig
	envConfig *config.EnvConfig
}

// validatePath ensures a path is relative and within the data directory
func (s *Server) validatePath(path string) (string, error) {
	// Handle empty path by using default filename
	if path == "" || path == "." {
		path = "file.txt"
	}

	// Initial validation before any path manipulation
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("absolute paths are not allowed")
	}

	// Normalize path separators to forward slashes for consistent checking
	normalizedPath := filepath.ToSlash(path)

	// Check for various path traversal patterns
	traversalPatterns := []string{
		"../", "/..", "../", "..\\", "\\..",
		"/../", "\\..\\", "/../../", "\\..\\..\\",
	}
	for _, pattern := range traversalPatterns {
		if strings.Contains(normalizedPath, pattern) {
			return "", fmt.Errorf("access denied")
		}
	}

	// Get absolute path of data directory
	absDataDir, err := filepath.Abs(s.config.DataDir)
	if err != nil {
		config.DebugLog("Failed to get absolute data directory path: %v", err)
		return "", fmt.Errorf("invalid data directory path")
	}
	config.DebugLog("Data directory absolute path: %s", absDataDir)

	// Join with data directory and clean the path
	fullPath := filepath.Clean(filepath.Join(s.config.DataDir, path))
	config.DebugLog("Full path after joining with data directory: %s", fullPath)

	// Get absolute path of the target file
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		config.DebugLog("Failed to get absolute path for target file: %v", err)
		return "", fmt.Errorf("invalid path")
	}
	config.DebugLog("Target file absolute path: %s", absPath)

	// Check if the path is within the data directory
	if !strings.HasPrefix(absPath, absDataDir+string(os.PathSeparator)) {
		config.DebugLog("Path attempts to escape data directory: %s", absPath)
		return "", fmt.Errorf("path attempts to escape data directory")
	}

	// Check if the path is the data directory itself
	if absPath == absDataDir {
		config.DebugLog("Path points to data directory itself")
		return "", fmt.Errorf("invalid path")
	}

	// Get relative path and check for traversal attempts
	relPath, err := filepath.Rel(s.config.DataDir, fullPath)
	if err != nil {
		config.DebugLog("Failed to get relative path: %v", err)
		return "", fmt.Errorf("invalid path")
	}
	config.DebugLog("Relative path: %s", relPath)

	// Check each path component
	components := strings.Split(filepath.ToSlash(relPath), "/")
	for _, comp := range components {
		if comp == ".." || comp == "." || strings.Contains(comp, "..") {
			config.DebugLog("Invalid path component detected: %s", comp)
			return "", fmt.Errorf("path attempts to escape data directory")
		}
	}

	config.DebugLog("Path validation successful: %s", fullPath)
	return fullPath, nil
}

// handleCORS adds CORS headers based on configuration
func (s *Server) handleCORS(w http.ResponseWriter, r *http.Request) {
	config.DebugLog("[CORS] CORS Configuration: %+v", s.config.CORS)

	if !s.config.CORS.Enabled {
		config.DebugLog("[CORS] CORS is disabled")
		return
	}

	requestOrigin := r.Header.Get("Origin")
	config.DebugLog("[CORS] Incoming request origin: %s", requestOrigin)

	// Set allowed origins
	if len(s.config.CORS.AllowedOrigins) > 0 {
		origin := strings.Join(s.config.CORS.AllowedOrigins, ", ")
		config.DebugLog("[CORS] Setting Access-Control-Allow-Origin: %s", origin)
		w.Header().Set("Access-Control-Allow-Origin", origin)
	} else {
		config.DebugLog("[CORS] Setting Access-Control-Allow-Origin: *")
		w.Header().Set("Access-Control-Allow-Origin", "*")
	}

	// Set allowed methods
	if len(s.config.CORS.AllowedMethods) > 0 {
		methods := strings.Join(s.config.CORS.AllowedMethods, ", ")
		config.DebugLog("[CORS] Setting Access-Control-Allow-Methods: %s", methods)
		w.Header().Set("Access-Control-Allow-Methods", methods)
	} else {
		defaultMethods := "GET, POST, PUT, DELETE, OPTIONS"
		config.DebugLog("[CORS] Setting default Access-Control-Allow-Methods: %s", defaultMethods)
		w.Header().Set("Access-Control-Allow-Methods", defaultMethods)
	}

	// Set allowed headers
	if len(s.config.CORS.AllowedHeaders) > 0 {
		headers := strings.Join(s.config.CORS.AllowedHeaders, ", ")
		config.DebugLog("[CORS] Setting Access-Control-Allow-Headers: %s", headers)
		w.Header().Set("Access-Control-Allow-Headers", headers)
	} else {
		defaultHeaders := "Authorization, Content-Type"
		config.DebugLog("[CORS] Setting default Access-Control-Allow-Headers: %s", defaultHeaders)
		w.Header().Set("Access-Control-Allow-Headers", defaultHeaders)
	}

	// Set max age
	if s.config.CORS.MaxAge > 0 {
		maxAge := fmt.Sprintf("%d", s.config.CORS.MaxAge)
		config.DebugLog("[CORS] Setting Access-Control-Max-Age: %s", maxAge)
		w.Header().Set("Access-Control-Max-Age", maxAge)
	} else {
		config.DebugLog("[CORS] Setting default Access-Control-Max-Age: 3600")
		w.Header().Set("Access-Control-Max-Age", "3600")
	}

	// Log all response headers for debugging
	config.DebugLog("[CORS] Final response headers:")
	for k, v := range w.Header() {
		config.DebugLog("[CORS] %s: %v", k, v)
	}
}

// combinedMiddleware applies middleware in the correct order based on request method
func (s *Server) combinedMiddleware(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Create enhanced response writer that supports flushing
		rw := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		// Always set CORS headers first
		config.DebugLog("[CORS] Request details: Method=%s, URL=%s", r.Method, r.URL.String())
		config.DebugLog("[CORS] Request headers:")
		for k, v := range r.Header {
			config.DebugLog("[CORS] %s: %v", k, v)
		}
		s.handleCORS(rw, r)

		// Handle OPTIONS requests immediately
		if r.Method == "OPTIONS" {
			rw.WriteHeader(http.StatusOK)
			return
		}

		// For streaming requests, ensure we have flushing capability
		if r.Header.Get("Accept") == "text/event-stream" || r.URL.Query().Get("streaming") == "true" {
			if flusher, ok := w.(http.Flusher); ok {
				config.DebugLog("Flusher capability confirmed for streaming request")
				// Create a new flushingResponseWriter that wraps the original ResponseWriter
				rw = &responseWriter{
					ResponseWriter: &flushingResponseWriter{
						ResponseWriter: w,
						flusher:        flusher,
					},
					statusCode: http.StatusOK,
				}
			} else {
				config.DebugLog("Flusher not supported for streaming request, type: %T", w)
			}
		}

		// For non-OPTIONS requests, proceed with logging and auth
		logRequest(func(w http.ResponseWriter, r *http.Request) {
			if !checkAuth(s.config, w, r) {
				return
			}
			handler(w, r)
		})(rw, r)
	}
}

// New creates a new HTTP server with the given configuration
func New(envConfig *config.EnvConfig) (*http.Server, error) {
	// Get server configuration
	serverConfig := envConfig.GetServerConfig()
	if serverConfig == nil {
		return nil, fmt.Errorf("server configuration not found")
	}

	// Create data directory if it doesn't exist
	if err := os.MkdirAll(serverConfig.DataDir, 0755); err != nil {
		return nil, fmt.Errorf("error creating data directory: %v", err)
	}

	s := &Server{
		mux:       http.NewServeMux(),
		config:    serverConfig,
		envConfig: envConfig,
	}

	// No default runtime directory is created

	// Register routes
	s.routes()

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", serverConfig.Port),
		Handler:      s.mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	return server, nil
}

// routes sets up the server routes
func (s *Server) routes() {
	// Health check endpoint - no auth required
	s.mux.HandleFunc("/health", s.combinedMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(HealthResponse{
			Status:    "ok",
			Timestamp: time.Now().Format(time.RFC3339),
		})
	}))

	// File operations - require auth
	s.mux.HandleFunc("/list", s.combinedMiddleware(s.handleListFiles))
	s.mux.HandleFunc("/files", s.combinedMiddleware(s.handleFileOperation))
	s.mux.HandleFunc("/files/bulk", s.combinedMiddleware(s.handleBulkFileOperation))
	s.mux.HandleFunc("/files/backup", s.combinedMiddleware(s.handleFileBackup))
	s.mux.HandleFunc("/files/restore", s.combinedMiddleware(s.handleFileRestore))
	s.mux.HandleFunc("/files/content", s.combinedMiddleware(s.handleGetFileContent))
	s.mux.HandleFunc("/files/upload", s.combinedMiddleware(s.handleFileUpload))
	s.mux.HandleFunc("/files/download", s.combinedMiddleware(s.handleFileDownload))

	// Provider operations - require auth
	s.mux.HandleFunc("/providers", s.combinedMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			s.handleGetProviders(w, r)
		case http.MethodPut:
			s.handleUpdateProvider(w, r)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "Method not allowed",
			})
		}
	}))

	// Provider and Model operations with path parameters
	s.mux.HandleFunc("/providers/", s.combinedMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := strings.TrimPrefix(r.URL.Path, "/providers/")
		parts := strings.Split(path, "/")

		// Expected formats:
		// {provider_name} -> DELETE
		// {provider_name}/models -> GET, POST
		// {provider_name}/available-models -> GET
		// {provider_name}/models/{model_name} -> PUT, DELETE

		if len(parts) == 0 || parts[0] == "" {
			sendJSONError(w, http.StatusBadRequest, "Provider name is required in the path")
			return
		}
		providerName := parts[0]

		switch len(parts) {
		case 1: // Path: /providers/{provider_name}
			if r.Method == http.MethodDelete {
				s.handleDeleteProvider(w, r, providerName)
			} else {
				sendJSONError(w, http.StatusMethodNotAllowed, "Method not allowed for this path")
			}
		case 2: // Path: /providers/{provider_name}/models or /providers/{provider_name}/available-models
			if parts[1] == "models" {
				switch r.Method {
				case http.MethodGet:
					s.handleGetConfiguredModels(w, r, providerName)
				case http.MethodPost:
					s.handleAddModel(w, r, providerName)
				default:
					sendJSONError(w, http.StatusMethodNotAllowed, "Method not allowed for /models path")
				}
			} else if parts[1] == "available-models" {
				if r.Method == http.MethodGet {
					s.handleGetAvailableModels(w, r, providerName)
				} else {
					sendJSONError(w, http.StatusMethodNotAllowed, "Method not allowed for /available-models path")
				}
			} else {
				sendJSONError(w, http.StatusNotFound, "Invalid path")
			}
		case 3: // Path: /providers/{provider_name}/models/{model_name}
			if parts[1] == "models" && parts[2] != "" {
				modelName := parts[2]
				switch r.Method {
				case http.MethodPut:
					s.handleUpdateModel(w, r, providerName, modelName)
				case http.MethodDelete:
					s.handleDeleteModel(w, r, providerName, modelName)
				default:
					sendJSONError(w, http.StatusMethodNotAllowed, "Method not allowed for specific model path")
				}
			} else {
				sendJSONError(w, http.StatusNotFound, "Invalid path")
			}
		default:
			sendJSONError(w, http.StatusNotFound, "Invalid path")
		}
	}))

	// Provider validation - requires auth
	s.mux.HandleFunc("/providers/validate", s.combinedMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		s.handleValidateProvider(w, r)
	}))

	// Environment operations - require auth
	s.mux.HandleFunc("/env/encrypt", s.combinedMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		s.handleEncryptEnv(w, r)
	}))

	s.mux.HandleFunc("/env/decrypt", s.combinedMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		s.handleDecryptEnv(w, r)
	}))

	// YAML operations - require auth
	s.mux.HandleFunc("/yaml/upload", s.combinedMiddleware(s.handleYAMLUpload))
	s.mux.HandleFunc("/yaml/process", s.combinedMiddleware(s.handleYAMLProcess))

	// Process endpoint - requires auth
	s.mux.HandleFunc("/process", s.combinedMiddleware(func(w http.ResponseWriter, r *http.Request) {
		handleProcess(w, r, s.config, s.envConfig)
	}))

	// Generate endpoint - requires auth
	s.mux.HandleFunc("/generate", s.combinedMiddleware(s.handleGenerate))

	// OpenAI compatibility endpoints (if enabled)
	if s.config.OpenAICompat.Enabled {
		prefix := s.config.OpenAICompat.Prefix
		if prefix == "" {
			prefix = "/v1"
		}

		// GET /v1/models - List available models (workflows)
		s.mux.HandleFunc(prefix+"/models", s.combinedMiddleware(s.handleListModels))

		// POST /v1/chat/completions - Chat completion endpoint
		s.mux.HandleFunc(prefix+"/chat/completions", s.combinedMiddleware(s.handleChatCompletions))

		log.Printf("OpenAI compatibility mode enabled at %s\n", prefix)
	}
}

// Run creates and starts the HTTP server with the given configuration
func Run(envConfig *config.EnvConfig) error {
	server, err := New(envConfig)
	if err != nil {
		return err
	}

	serverConfig := envConfig.GetServerConfig()
	if serverConfig == nil {
		return fmt.Errorf("server configuration not found")
	}

	log.Printf("Starting server on port %d...\n", serverConfig.Port)
	log.Printf("Data directory: %s\n", serverConfig.DataDir)
	log.Printf("Runtime directories can be specified with the runtimeDir query parameter\n")

	if serverConfig.Enabled {
		log.Printf("Authentication is enabled. Bearer token required.\n")
		log.Printf("Example usage: curl -H 'Authorization: Bearer %s' 'http://localhost:%d/process?filename=examples/openai-example.yaml'\n",
			maskToken(serverConfig.BearerToken), serverConfig.Port)
	} else {
		log.Printf("Example usage: curl 'http://localhost:%d/process?filename=examples/openai-example.yaml'\n", serverConfig.Port)
	}

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server failed to start: %v", err)
	}

	return nil
}
