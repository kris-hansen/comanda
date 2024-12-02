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

// logger is a custom logger for HTTP requests
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

func checkAuth(serverConfig *config.ServerConfig, w http.ResponseWriter, r *http.Request) bool {
	if !serverConfig.Enabled {
		config.VerboseLog("Authentication disabled")
		config.DebugLog("Auth check skipped: server auth is disabled")
		return true
	}

	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		config.VerboseLog("Missing Authorization header")
		config.DebugLog("Auth failed: no Authorization header present in request")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(ProcessResponse{
			Success: false,
			Error:   "Authorization header required",
		})
		return false
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		config.VerboseLog("Invalid authorization header format")
		config.DebugLog("Auth failed: malformed Authorization header: %s", authHeader)
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(ProcessResponse{
			Success: false,
			Error:   "Invalid authorization header format",
		})
		return false
	}

	if parts[1] != serverConfig.BearerToken {
		config.VerboseLog("Invalid bearer token")
		config.DebugLog("Auth failed: invalid bearer token provided")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(ProcessResponse{
			Success: false,
			Error:   "Invalid bearer token",
		})
		return false
	}

	config.VerboseLog("Authentication successful")
	config.DebugLog("Auth successful: valid bearer token")
	return true
}

// hasStdinInput checks if the first step in the YAML uses STDIN as input
func hasStdinInput(yamlContent []byte) bool {
	config.VerboseLog("Checking YAML for STDIN input requirement")

	// Parse YAML as a map to handle top-level keys as steps
	var yamlMap map[string]map[string]interface{}
	if err := yaml.Unmarshal(yamlContent, &yamlMap); err != nil {
		config.DebugLog("YAML parse error: %v", err)
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
		config.DebugLog("No steps found in YAML content")
		return false
	}

	config.DebugLog("Analyzing first step: %s", firstStepName)

	// Check if the first step's input is STDIN
	if input, ok := firstStep["input"]; ok {
		switch v := input.(type) {
		case string:
			hasStdin := strings.EqualFold(v, "STDIN")
			config.DebugLog("String input found: %s, STDIN=%v", v, hasStdin)
			return hasStdin
		case []interface{}:
			if len(v) > 0 {
				if str, ok := v[0].(string); ok {
					hasStdin := strings.EqualFold(str, "STDIN")
					config.DebugLog("Array input found: %v, STDIN=%v", v, hasStdin)
					return hasStdin
				}
			}
		}
		config.DebugLog("Input field found but not string or array type: %T", input)
	}

	config.DebugLog("No STDIN input requirement found")
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

func handleProcess(w http.ResponseWriter, r *http.Request, serverConfig *config.ServerConfig, envConfig *config.EnvConfig) {
	w.Header().Set("Content-Type", "application/json")

	// Get filename from query parameters
	filename := r.URL.Query().Get("filename")
	if filename == "" {
		config.VerboseLog("Missing filename parameter")
		config.DebugLog("Process request failed: no filename provided")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ProcessResponse{
			Success: false,
			Error:   "filename parameter is required",
		})
		return
	}

	config.VerboseLog("Processing file: %s", filename)
	config.DebugLog("Starting process request for file: %s", filename)

	// If filename doesn't start with data directory, prepend it
	if !strings.HasPrefix(filename, serverConfig.DataDir) {
		filename = filepath.Join(serverConfig.DataDir, filename)
		config.DebugLog("Adjusted filename path: %s", filename)
	}

	// Read YAML file
	yamlContent, err := os.ReadFile(filename)
	if err != nil {
		config.VerboseLog("Error reading file: %v", err)
		config.DebugLog("File read error: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ProcessResponse{
			Success: false,
			Error:   fmt.Sprintf("Error reading YAML file: %v", err),
		})
		return
	}

	// Check if the YAML requires STDIN input
	requiresStdin := hasStdinInput(yamlContent)
	config.DebugLog("YAML STDIN requirement: %v", requiresStdin)

	// If YAML requires STDIN, only allow POST requests
	if requiresStdin && r.Method != http.MethodPost {
		config.VerboseLog("Method not allowed: YAML requires POST")
		config.DebugLog("Method not allowed: got %s, need POST", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(ProcessResponse{
			Success: false,
			Error:   "This YAML file requires STDIN input and can only be accessed via POST",
		})
		return
	}

	// If YAML doesn't require STDIN, only allow GET requests
	if !requiresStdin && r.Method != http.MethodGet {
		config.VerboseLog("Method not allowed: YAML requires GET")
		config.DebugLog("Method not allowed: got %s, need GET", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(ProcessResponse{
			Success: false,
			Error:   "This YAML file does not accept STDIN input and can only be accessed via GET",
		})
		return
	}

	// First unmarshal into a map to preserve step names
	var rawConfig map[string]processor.StepConfig
	if err := yaml.Unmarshal(yamlContent, &rawConfig); err != nil {
		config.VerboseLog("Error parsing YAML: %v", err)
		config.DebugLog("YAML parse error: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ProcessResponse{
			Success: false,
			Error:   fmt.Sprintf("Error parsing YAML file: %v", err),
		})
		return
	}

	// Convert map to ordered Steps slice
	var dslConfig processor.DSLConfig
	for name, config := range rawConfig {
		dslConfig.Steps = append(dslConfig.Steps, processor.Step{
			Name:   name,
			Config: config,
		})
	}

	// Create input handler and processor
	inputHandler := input.NewHandler()
	proc := processor.NewProcessor(&dslConfig, envConfig, verbose)

	// Handle POST input if present
	if r.Method == http.MethodPost {
		config.DebugLog("Processing POST request input")

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
			config.DebugLog("Extracted input from JSON body")
		}

		if stdinInput == "" {
			config.VerboseLog("Missing input for POST request")
			config.DebugLog("POST request failed: no input provided")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(ProcessResponse{
				Success: false,
				Error:   "POST request requires 'input' query parameter or JSON body with 'input' field",
			})
			return
		}

		config.VerboseLog("Processing STDIN input")
		config.DebugLog("Input length: %d bytes", len(stdinInput))

		// Process STDIN input and set it as the processor's last output
		if err := inputHandler.ProcessStdin(stdinInput); err != nil {
			config.VerboseLog("Error processing input: %v", err)
			config.DebugLog("STDIN processing error: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ProcessResponse{
				Success: false,
				Error:   fmt.Sprintf("Error processing input: %v", err),
			})
			return
		}

		// Set the STDIN input as the processor's last output
		proc.SetLastOutput(stdinInput)
	}

	// Create a buffer to capture output
	var buf bytes.Buffer

	// Create a pipe for capturing actual output
	pipeReader, pipeWriter, _ := os.Pipe()

	// Create a custom writer that filters debug/verbose messages
	filterWriter := &filteringWriter{
		output: pipeWriter,
		debug:  os.Stdout,
	}

	// Save the original log output
	originalLogOutput := log.Writer()

	// Redirect log output through our filter
	log.SetOutput(filterWriter)

	config.DebugLog("Starting DSL processing")

	// Run the processor
	err = proc.Process()

	// Create a WaitGroup to ensure we capture all output
	var wg sync.WaitGroup
	wg.Add(1)

	// Copy the output in a separate goroutine
	go func() {
		defer wg.Done()
		io.Copy(&buf, pipeReader)
	}()

	// Restore original log output and close writers
	log.SetOutput(originalLogOutput)
	pipeWriter.Close()

	// Wait for all output to be captured
	wg.Wait()

	// Get the final output from the processor
	finalOutput := proc.LastOutput()

	if err != nil {
		config.VerboseLog("Error processing DSL: %v", err)
		config.DebugLog("DSL processing error: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ProcessResponse{
			Success: false,
			Error:   fmt.Sprintf("Error processing DSL file: %v", err),
			Output:  finalOutput,
		})
		return
	}

	config.VerboseLog("Successfully processed file: %s", filename)
	config.DebugLog("DSL processing complete. Output length: %d bytes", len(finalOutput))

	// Return the response with the final output
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(ProcessResponse{
		Success: true,
		Message: fmt.Sprintf("Successfully processed %s", filename),
		Output:  finalOutput,
	})
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

func init() {
	rootCmd.AddCommand(serveCmd)
}
