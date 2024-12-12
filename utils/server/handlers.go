package server

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

	"github.com/kris-hansen/comanda/utils/config"
	"github.com/kris-hansen/comanda/utils/fileutil"
	"github.com/kris-hansen/comanda/utils/processor"
	"gopkg.in/yaml.v3"
)

func handleProcess(w http.ResponseWriter, r *http.Request, serverConfig *ServerConfig, envConfig *config.EnvConfig) {
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

	// Clean the path to remove any . or .. components
	cleanPath := filepath.Clean(filename)

	// If filename doesn't start with data directory, prepend it
	if !strings.HasPrefix(cleanPath, serverConfig.DataDir) {
		cleanPath = filepath.Join(serverConfig.DataDir, cleanPath)
		config.DebugLog("Adjusted filename path: %s", cleanPath)
	}

	// Get the relative path between the data directory and the target file
	relPath, err := filepath.Rel(serverConfig.DataDir, cleanPath)
	if err != nil {
		config.VerboseLog("Error validating file path: %v", err)
		config.DebugLog("Path validation error: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ProcessResponse{
			Success: false,
			Error:   "Invalid file path",
		})
		return
	}

	// Check if the relative path tries to escape the data directory
	if strings.HasPrefix(relPath, "..") || strings.Contains(relPath, "/../") {
		config.VerboseLog("Attempted directory traversal detected")
		config.DebugLog("Security violation: attempted path traversal")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(ProcessResponse{
			Success: false,
			Error:   "Invalid file path: attempted directory traversal",
		})
		return
	}

	// Verify the final path exists and is within the data directory
	finalPath := filepath.Clean(cleanPath)
	if !strings.HasPrefix(finalPath, filepath.Clean(serverConfig.DataDir)) {
		config.VerboseLog("File path escapes data directory")
		config.DebugLog("Security violation: path escapes data directory")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(ProcessResponse{
			Success: false,
			Error:   "Invalid file path: access denied",
		})
		return
	}

	// Read YAML file with size check
	yamlContent, err := fileutil.SafeReadFile(finalPath)
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

	// Check if the YAML requires STDIN input using the existing function from auth.go
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

	// First unmarshal into a map to preserve step names (same as CLI)
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

	// Convert map to ordered Steps slice (same as CLI)
	var dslConfig processor.DSLConfig
	for name, config := range rawConfig {
		dslConfig.Steps = append(dslConfig.Steps, processor.Step{
			Name:   name,
			Config: config,
		})
	}

	// Create processor instance with validation enabled
	proc := processor.NewProcessor(&dslConfig, envConfig, true)

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

	// Run the processor which includes validation
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
