package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/kris-hansen/comanda/utils/config"
	"github.com/kris-hansen/comanda/utils/input"
	"github.com/kris-hansen/comanda/utils/processor"
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
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// logger is a custom logger for HTTP requests
var logger = log.New(os.Stdout, "", log.LstdFlags)

func logRequest(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := &responseWriter{w, http.StatusOK}

		// Build auth info string, masking the token
		var authInfo string
		if auth := r.Header.Get("Authorization"); auth != "" {
			authInfo = strings.Replace(auth, auth[7:], "********", 1)
		}

		// Call the handler
		handler(wrapped, r)

		// Calculate duration
		duration := time.Since(start)

		// Log the request details in a structured format
		logger.Printf("Request: method=%s path=%s query=%s auth=%s status=%d duration=%v",
			r.Method,
			r.URL.Path,
			r.URL.RawQuery,
			authInfo,
			wrapped.statusCode,
			duration)
	}
}

func checkAuth(serverConfig *config.ServerConfig, w http.ResponseWriter, r *http.Request) bool {
	if !serverConfig.Enabled {
		return true
	}

	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(ProcessResponse{
			Success: false,
			Error:   "Authorization header required",
		})
		return false
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(ProcessResponse{
			Success: false,
			Error:   "Invalid authorization header format",
		})
		return false
	}

	if parts[1] != serverConfig.BearerToken {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(ProcessResponse{
			Success: false,
			Error:   "Invalid bearer token",
		})
		return false
	}

	return true
}

// hasStdinInput checks if the first step in the YAML uses STDIN as input
func hasStdinInput(yamlContent []byte) bool {
	// Parse YAML as a map to handle top-level keys as steps
	var yamlMap map[string]map[string]interface{}
	if err := yaml.Unmarshal(yamlContent, &yamlMap); err != nil {
		return false
	}

	// Find the first step and check its input
	var firstStep map[string]interface{}
	var firstStepName string
	for name, step := range yamlMap {
		if firstStep == nil || name < firstStepName { // Use alphabetical order to determine first step
			firstStep = step
			firstStepName = name
		}
	}

	if firstStep == nil {
		return false
	}

	// Check if the first step's input is STDIN
	if input, ok := firstStep["input"]; ok {
		switch v := input.(type) {
		case string:
			return strings.EqualFold(v, "STDIN")
		case []interface{}:
			if len(v) > 0 {
				if str, ok := v[0].(string); ok {
					return strings.EqualFold(str, "STDIN")
				}
			}
		}
	}

	return false
}

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

			files, err := filepath.Glob(filepath.Join(serverConfig.DataDir, "*.yaml"))
			if err != nil {
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
					continue
				}

				// Read and parse YAML to check if it accepts POST
				yamlContent, err := os.ReadFile(file)
				if err != nil {
					continue
				}

				methods := "GET"
				if hasStdinInput(yamlContent) {
					methods = "POST" // Changed from "GET,POST" to "POST" only
				}

				fileInfos = append(fileInfos, YAMLFileInfo{
					Name:    relFile,
					Methods: methods,
				})
			}

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

func handleProcess(w http.ResponseWriter, r *http.Request, serverConfig *config.ServerConfig, envConfig *config.EnvConfig) {
	w.Header().Set("Content-Type", "application/json")

	// Get filename from query parameters
	filename := r.URL.Query().Get("filename")
	if filename == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ProcessResponse{
			Success: false,
			Error:   "filename parameter is required",
		})
		return
	}

	// If filename doesn't start with data directory, prepend it
	if !strings.HasPrefix(filename, serverConfig.DataDir) {
		filename = filepath.Join(serverConfig.DataDir, filename)
	}

	// Read YAML file
	yamlContent, err := os.ReadFile(filename)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ProcessResponse{
			Success: false,
			Error:   fmt.Sprintf("Error reading YAML file: %v", err),
		})
		return
	}

	// Check if the YAML requires STDIN input
	requiresStdin := hasStdinInput(yamlContent)

	// If YAML requires STDIN, only allow POST requests
	if requiresStdin && r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(ProcessResponse{
			Success: false,
			Error:   "This YAML file requires STDIN input and can only be accessed via POST",
		})
		return
	}

	// If YAML doesn't require STDIN, only allow GET requests
	if !requiresStdin && r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(ProcessResponse{
			Success: false,
			Error:   "This YAML file does not accept STDIN input and can only be accessed via GET",
		})
		return
	}

	// Parse YAML into DSL config for processing
	var dslConfig processor.DSLConfig
	if err := yaml.Unmarshal(yamlContent, &dslConfig); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ProcessResponse{
			Success: false,
			Error:   fmt.Sprintf("Error parsing YAML file: %v", err),
		})
		return
	}

	// Create input handler
	inputHandler := input.NewHandler()

	// Handle POST input if present
	if r.Method == http.MethodPost {
		// First check query parameter
		stdinInput := r.URL.Query().Get("input")

		// If not in query, check JSON body
		if stdinInput == "" && r.Body != nil {
			var jsonBody struct {
				Input string `json:"input"`
			}
			if err := json.NewDecoder(r.Body).Decode(&jsonBody); err == nil {
				stdinInput = jsonBody.Input
			}
		}

		if stdinInput == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(ProcessResponse{
				Success: false,
				Error:   "POST request requires 'input' query parameter or JSON body with 'input' field",
			})
			return
		}

		// Process STDIN input
		if err := inputHandler.ProcessStdin(stdinInput); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ProcessResponse{
				Success: false,
				Error:   fmt.Sprintf("Error processing input: %v", err),
			})
			return
		}
	}

	// Create a buffer to capture output
	var buf bytes.Buffer

	// Temporarily replace stdout
	oldStdout := os.Stdout
	pipeReader, pipeWriter, _ := os.Pipe()
	os.Stdout = pipeWriter

	// Create and run processor with input handler
	proc := processor.NewProcessor(&dslConfig, envConfig, verbose)
	err = proc.Process()

	// Create a WaitGroup to ensure we capture all output
	var wg sync.WaitGroup
	wg.Add(1)

	// Copy the output in a separate goroutine
	go func() {
		defer wg.Done()
		io.Copy(&buf, pipeReader)
	}()

	// Restore stdout
	pipeWriter.Close()
	os.Stdout = oldStdout

	// Wait for all output to be captured
	wg.Wait()

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ProcessResponse{
			Success: false,
			Error:   fmt.Sprintf("Error processing DSL file: %v", err),
			Output:  buf.String(),
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(ProcessResponse{
		Success: true,
		Message: fmt.Sprintf("Successfully processed %s", filename),
		Output:  buf.String(),
	})
}

func init() {
	rootCmd.AddCommand(serveCmd)
}
