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

	// Convert config.ServerConfig to our internal ServerConfig
	srvConfig := &ServerConfig{
		Port:        serverConfig.Port,
		DataDir:     serverConfig.DataDir,
		BearerToken: serverConfig.BearerToken,
		Enabled:     serverConfig.Enabled,
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
	// Health check endpoint
	s.mux.HandleFunc("/health", logRequest(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(HealthResponse{
			Status:    "ok",
			Timestamp: time.Now().Format(time.RFC3339),
		})
	}))

	// File operations
	s.mux.HandleFunc("/list", logRequest(s.handleListFiles))
	s.mux.HandleFunc("/files", logRequest(s.handleFileOperation))
	s.mux.HandleFunc("/files/bulk", logRequest(s.handleBulkFileOperation))
	s.mux.HandleFunc("/files/backup", logRequest(s.handleFileBackup))
	s.mux.HandleFunc("/files/restore", logRequest(s.handleFileRestore))

	// Provider operations
	s.mux.HandleFunc("/providers", logRequest(func(w http.ResponseWriter, r *http.Request) {
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

	// Provider validation
	s.mux.HandleFunc("/providers/validate", logRequest(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		s.handleValidateProvider(w, r)
	}))

	// Environment operations
	s.mux.HandleFunc("/env/encrypt", logRequest(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		s.handleEncryptEnv(w, r)
	}))

	s.mux.HandleFunc("/env/decrypt", logRequest(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		s.handleDecryptEnv(w, r)
	}))

	// Process endpoint
	s.mux.HandleFunc("/process", logRequest(func(w http.ResponseWriter, r *http.Request) {
		if !checkAuth(s.config, w, r) {
			return
		}
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
