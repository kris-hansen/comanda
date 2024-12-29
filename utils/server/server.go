package server

import (
	"encoding/json"
	"fmt"
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
	config    *ServerConfig
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
			return "", fmt.Errorf("path attempts to escape data directory")
		}
	}

	// Get absolute path of data directory
	absDataDir, err := filepath.Abs(s.config.DataDir)
	if err != nil {
		return "", fmt.Errorf("invalid data directory path")
	}

	// Join with data directory and clean the path
	fullPath := filepath.Clean(filepath.Join(s.config.DataDir, path))

	// Get absolute path of the target file
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", fmt.Errorf("invalid path")
	}

	// Check if the path is within the data directory
	if !strings.HasPrefix(absPath, absDataDir+string(os.PathSeparator)) {
		return "", fmt.Errorf("path attempts to escape data directory")
	}

	// Check if the path is the data directory itself
	if absPath == absDataDir {
		return "", fmt.Errorf("invalid path")
	}

	// Get relative path and check for traversal attempts
	relPath, err := filepath.Rel(s.config.DataDir, fullPath)
	if err != nil {
		return "", fmt.Errorf("invalid path")
	}

	// Check each path component
	components := strings.Split(filepath.ToSlash(relPath), "/")
	for _, comp := range components {
		if comp == ".." || comp == "." || strings.Contains(comp, "..") {
			return "", fmt.Errorf("path attempts to escape data directory")
		}
	}

	return fullPath, nil
}

// handleCORS adds CORS headers based on configuration
func (s *Server) handleCORS(w http.ResponseWriter) {
	if !s.config.CORS.Enabled {
		return
	}

	// Set allowed origins
	if len(s.config.CORS.AllowedOrigins) > 0 {
		origin := strings.Join(s.config.CORS.AllowedOrigins, ", ")
		w.Header().Set("Access-Control-Allow-Origin", origin)
	} else {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	}

	// Set allowed methods
	if len(s.config.CORS.AllowedMethods) > 0 {
		methods := strings.Join(s.config.CORS.AllowedMethods, ", ")
		w.Header().Set("Access-Control-Allow-Methods", methods)
	} else {
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	}

	// Set allowed headers
	if len(s.config.CORS.AllowedHeaders) > 0 {
		headers := strings.Join(s.config.CORS.AllowedHeaders, ", ")
		w.Header().Set("Access-Control-Allow-Headers", headers)
	} else {
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
	}

	// Set max age
	if s.config.CORS.MaxAge > 0 {
		w.Header().Set("Access-Control-Max-Age", fmt.Sprintf("%d", s.config.CORS.MaxAge))
	} else {
		w.Header().Set("Access-Control-Max-Age", "3600")
	}
}

// combinedMiddleware applies middleware in the correct order based on request method
func (s *Server) combinedMiddleware(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Always set CORS headers first
		s.handleCORS(w)

		// Handle OPTIONS requests immediately
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		// For non-OPTIONS requests, proceed with logging and auth
		logRequest(func(w http.ResponseWriter, r *http.Request) {
			if !checkAuth(s.config, w, r) {
				return
			}
			handler(w, r)
		})(w, r)
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

	// Convert config.ServerConfig to our internal ServerConfig with default CORS settings
	srvConfig := &ServerConfig{
		Port:        serverConfig.Port,
		DataDir:     serverConfig.DataDir,
		BearerToken: serverConfig.BearerToken,
		Enabled:     serverConfig.Enabled,
		CORS: CORSConfig{
			Enabled:        true,
			AllowedOrigins: []string{"*"},
			AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
			AllowedHeaders: []string{"Authorization", "Content-Type"},
			MaxAge:         3600,
		},
	}

	s := &Server{
		mux:       http.NewServeMux(),
		config:    srvConfig,
		envConfig: envConfig,
	}

	// Register routes
	s.routes()

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", srvConfig.Port),
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

	// Provider operations - require auth
	s.mux.HandleFunc("/providers", s.combinedMiddleware(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			s.handleGetProviders(w, r)
		case http.MethodPut:
			s.handleUpdateProvider(w, r)
		case http.MethodDelete:
			s.handleDeleteProvider(w, r)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "Method not allowed",
			})
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

	// Process endpoint - requires auth
	s.mux.HandleFunc("/process", s.combinedMiddleware(func(w http.ResponseWriter, r *http.Request) {
		handleProcess(w, r, s.config, s.envConfig)
	}))
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

	fmt.Printf("Starting server on port %d...\n", serverConfig.Port)
	fmt.Printf("Data directory: %s\n", serverConfig.DataDir)
	if serverConfig.Enabled {
		fmt.Println("Authentication is enabled. Bearer token required.")
		fmt.Printf("Example usage: curl -H 'Authorization: Bearer %s' 'http://localhost:%d/process?filename=examples/openai-example.yaml'\n",
			serverConfig.BearerToken, serverConfig.Port)
	} else {
		fmt.Printf("Example usage: curl 'http://localhost:%d/process?filename=examples/openai-example.yaml'\n", serverConfig.Port)
	}

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server failed to start: %v", err)
	}

	return nil
}
