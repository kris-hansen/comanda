package cmd

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/kris-hansen/comanda/utils/config"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start HTTP server for processing YAML files",
	Long:  `Start an HTTP server that processes YAML DSL configuration files via HTTP requests.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Load environment configuration
		envConfig, err := config.LoadEnvConfigWithPassword(config.GetEnvPath())
		if err != nil {
			log.Fatalf("Error loading environment configuration: %v", err)
		}

		// Get server configuration
		serverConfig := envConfig.GetServerConfig()
		if serverConfig == nil {
			log.Fatal("Server configuration not found. Please run 'comanda configure --server' first")
		}

		config.VerboseLog("Starting server with configuration:")
		config.VerboseLog("- Port: %d", serverConfig.Port)
		config.VerboseLog("- Data Directory: %s", serverConfig.DataDir)
		config.VerboseLog("- Authentication: %v", serverConfig.Enabled)

		config.DebugLog("Detailed server configuration:")
		config.DebugLog("- Port: %d", serverConfig.Port)
		config.DebugLog("- Data Directory: %s", serverConfig.DataDir)
		config.DebugLog("- Authentication Enabled: %v", serverConfig.Enabled)
		config.DebugLog("- Bearer Token Length: %d", len(serverConfig.BearerToken))
		config.DebugLog("- Environment Config Path: %s", config.GetEnvPath())

		// Create data directory if it doesn't exist
		if err := os.MkdirAll(serverConfig.DataDir, 0755); err != nil {
			log.Fatalf("Error creating data directory: %v", err)
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

			if !checkAuth(serverConfig, w, r) {
				return
			}

			config.VerboseLog("Listing files in data directory")
			config.DebugLog("Scanning directory: %s", serverConfig.DataDir)

			files, err := filepath.Glob(filepath.Join(serverConfig.DataDir, "*.yaml"))
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
				relFile, err := filepath.Rel(serverConfig.DataDir, file)
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
			if !checkAuth(serverConfig, w, r) {
				return
			}
			handleProcess(w, r, serverConfig, envConfig)
		}))

		server := &http.Server{
			Addr:         fmt.Sprintf(":%d", serverConfig.Port),
			Handler:      mux,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 120 * time.Second,
			IdleTimeout:  120 * time.Second,
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

		if err := server.ListenAndServe(); err != nil {
			log.Fatalf("Server failed to start: %v", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
}
