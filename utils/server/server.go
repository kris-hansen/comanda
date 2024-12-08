package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/kris-hansen/comanda/utils/config"
)

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

	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("/health", logRequest(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(HealthResponse{
			Status:    "ok",
			Timestamp: time.Now().Format(time.RFC3339),
		})
	}))

	// List files endpoint
	mux.HandleFunc("/list", logRequest(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if !checkAuth(srvConfig, w, r) {
			return
		}

		config.VerboseLog("Listing files in data directory")
		config.DebugLog("Scanning directory: %s", srvConfig.DataDir)

		files, err := filepath.Glob(filepath.Join(srvConfig.DataDir, "*.yaml"))
		if err != nil {
			config.VerboseLog("Error listing files: %v", err)
			config.DebugLog("Glob error: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ListResponse{
				Success: false,
				Error:   fmt.Sprintf("Error listing files: %v", err),
			})
			return
		}

		var fileInfos []YAMLFileInfo
		for _, file := range files {
			relFile, err := filepath.Rel(srvConfig.DataDir, file)
			if err != nil {
				config.DebugLog("Error getting relative path for %s: %v", file, err)
				continue
			}

			config.DebugLog("Processing file: %s", relFile)

			// Read and parse YAML to check if it accepts POST
			yamlContent, err := os.ReadFile(file)
			if err != nil {
				config.DebugLog("Error reading file %s: %v", file, err)
				continue
			}

			methods := "GET"
			if hasStdinInput(yamlContent) {
				methods = "POST"
			}

			fileInfos = append(fileInfos, YAMLFileInfo{
				Name:    relFile,
				Methods: methods,
			})
		}

		config.VerboseLog("Found %d YAML files", len(fileInfos))
		config.DebugLog("File list complete: %v", fileInfos)

		json.NewEncoder(w).Encode(ListResponse{
			Success: true,
			Files:   fileInfos,
		})
	}))

	// Process endpoint
	mux.HandleFunc("/process", logRequest(func(w http.ResponseWriter, r *http.Request) {
		if !checkAuth(srvConfig, w, r) {
			return
		}
		handleProcess(w, r, srvConfig, envConfig)
	}))

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", srvConfig.Port),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	return server, nil
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
