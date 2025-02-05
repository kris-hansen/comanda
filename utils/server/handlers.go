package server

import (
	"bytes"
	"context"
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

	"github.com/kris-hansen/comanda/utils/config"
	"github.com/kris-hansen/comanda/utils/fileutil"
	"github.com/kris-hansen/comanda/utils/processor"
	"gopkg.in/yaml.v3"
)

func handleProcess(w http.ResponseWriter, r *http.Request, serverConfig *ServerConfig, envConfig *config.EnvConfig) {
	// Determine if streaming is requested
	streaming := r.URL.Query().Get("streaming") == "true" || r.Header.Get("Accept") == "text/event-stream"

	// Set appropriate headers based on streaming mode
	if streaming {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
	} else {
		w.Header().Set("Content-Type", "application/json")
	}

	// Verify this is a POST request first
	if r.Method != http.MethodPost {
		config.VerboseLog("Method not allowed: requires POST")
		config.DebugLog("Method not allowed: got %s, need POST", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		if streaming {
			flusher, ok := w.(http.Flusher)
			if !ok {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(ProcessResponse{
					Success: false,
					Error:   "Streaming is not supported",
				})
				return
			}
			sseWriter := &sseWriter{w: w, f: flusher}
			sseWriter.SendError(fmt.Errorf("YAML processing is only available via POST requests. Please use POST with your YAML content."))
		} else {
			json.NewEncoder(w).Encode(ProcessResponse{
				Success: false,
				Error:   "YAML processing is only available via POST requests. Please use POST with your YAML content.",
			})
		}
		return
	}

	// Get filename from query parameters
	filename := r.URL.Query().Get("filename")
	if filename == "" {
		config.VerboseLog("Missing filename parameter")
		config.DebugLog("Process request failed: no filename provided")
		w.WriteHeader(http.StatusBadRequest)
		if streaming {
			flusher, ok := w.(http.Flusher)
			if !ok {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(ProcessResponse{
					Success: false,
					Error:   "Streaming is not supported",
				})
				return
			}
			sseWriter := &sseWriter{w: w, f: flusher}
			sseWriter.SendError(fmt.Errorf("filename parameter is required"))
		} else {
			json.NewEncoder(w).Encode(ProcessResponse{
				Success: false,
				Error:   "filename parameter is required",
			})
		}
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
		if streaming {
			flusher, ok := w.(http.Flusher)
			if !ok {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(ProcessResponse{
					Success: false,
					Error:   "Streaming is not supported",
				})
				return
			}
			sseWriter := &sseWriter{w: w, f: flusher}
			sseWriter.SendError(fmt.Errorf("Invalid file path"))
		} else {
			json.NewEncoder(w).Encode(ProcessResponse{
				Success: false,
				Error:   "Invalid file path",
			})
		}
		return
	}

	// Check if the relative path tries to escape the data directory
	if strings.HasPrefix(relPath, "..") || strings.Contains(relPath, "/../") {
		config.VerboseLog("Attempted directory traversal detected")
		config.DebugLog("Security violation: attempted path traversal")
		w.WriteHeader(http.StatusForbidden)
		if streaming {
			flusher, ok := w.(http.Flusher)
			if !ok {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(ProcessResponse{
					Success: false,
					Error:   "Streaming is not supported",
				})
				return
			}
			sseWriter := &sseWriter{w: w, f: flusher}
			sseWriter.SendError(fmt.Errorf("Invalid file path: attempted directory traversal"))
		} else {
			json.NewEncoder(w).Encode(ProcessResponse{
				Success: false,
				Error:   "Invalid file path: attempted directory traversal",
			})
		}
		return
	}

	// Verify the final path exists and is within the data directory
	finalPath := filepath.Clean(cleanPath)
	if !strings.HasPrefix(finalPath, filepath.Clean(serverConfig.DataDir)) {
		config.VerboseLog("File path escapes data directory")
		config.DebugLog("Security violation: path escapes data directory")
		w.WriteHeader(http.StatusForbidden)
		if streaming {
			flusher, ok := w.(http.Flusher)
			if !ok {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(ProcessResponse{
					Success: false,
					Error:   "Streaming is not supported",
				})
				return
			}
			sseWriter := &sseWriter{w: w, f: flusher}
			sseWriter.SendError(fmt.Errorf("Invalid file path: access denied"))
		} else {
			json.NewEncoder(w).Encode(ProcessResponse{
				Success: false,
				Error:   "Invalid file path: access denied",
			})
		}
		return
	}

	// Read YAML file with size check
	yamlContent, err := fileutil.SafeReadFile(finalPath)
	if err != nil {
		config.VerboseLog("Error reading file: %v", err)
		config.DebugLog("File read error: %v", err)
		if streaming {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			flusher, ok := w.(http.Flusher)
			if !ok {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(ProcessResponse{
					Success: false,
					Error:   "Streaming is not supported",
				})
				return
			}
			sseWriter := &sseWriter{w: w, f: flusher}
			sseWriter.SendError(fmt.Errorf("Error reading YAML file: %v", err))
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(ProcessResponse{
				Success: false,
				Error:   fmt.Sprintf("Error reading YAML file: %v", err),
			})
		}
		return
	}

	// First unmarshal into a map to preserve step names (same as CLI)
	var rawConfig map[string]processor.StepConfig
	if err := yaml.Unmarshal(yamlContent, &rawConfig); err != nil {
		config.VerboseLog("Error parsing YAML: %v", err)
		config.DebugLog("YAML parse error: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		if streaming {
			flusher, ok := w.(http.Flusher)
			if !ok {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(ProcessResponse{
					Success: false,
					Error:   "Streaming is not supported",
				})
				return
			}
			sseWriter := &sseWriter{w: w, f: flusher}
			sseWriter.SendError(fmt.Errorf("Error parsing YAML file: %v", err))
		} else {
			json.NewEncoder(w).Encode(ProcessResponse{
				Success: false,
				Error:   fmt.Sprintf("Error parsing YAML file: %v", err),
			})
		}
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

	// Handle POST input
	var stdinInput string

	config.DebugLog("Processing POST request input")

	// First check query parameter
	stdinInput = r.URL.Query().Get("input")

	// If not in query, check JSON body
	if stdinInput == "" && r.Body != nil {
		var jsonBody struct {
			Input     string `json:"input"`
			Streaming bool   `json:"streaming"`
		}
		if err := json.NewDecoder(r.Body).Decode(&jsonBody); err == nil {
			stdinInput = jsonBody.Input
			streaming = jsonBody.Streaming
		}
		config.DebugLog("Extracted input from JSON body")
	}

	if stdinInput == "" {
		config.VerboseLog("Missing input for POST request")
		config.DebugLog("POST request failed: no input provided")
		w.WriteHeader(http.StatusBadRequest)
		if streaming {
			flusher, ok := w.(http.Flusher)
			if !ok {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(ProcessResponse{
					Success: false,
					Error:   "Streaming is not supported",
				})
				return
			}
			sseWriter := &sseWriter{w: w, f: flusher}
			sseWriter.SendError(fmt.Errorf("POST request requires 'input' query parameter or JSON body with 'input' field"))
		} else {
			json.NewEncoder(w).Encode(ProcessResponse{
				Success: false,
				Error:   "POST request requires 'input' query parameter or JSON body with 'input' field",
			})
		}
		return
	}

	config.VerboseLog("Processing STDIN input")
	config.DebugLog("Input length: %d bytes", len(stdinInput))

	// Set the STDIN input as the processor's last output
	proc.SetLastOutput(stdinInput)

	// Check Accept header for streaming
	if r.Header.Get("Accept") == "text/event-stream" {
		streaming = true
	}

	if streaming {
		// Set up SSE streaming
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		flusher, ok := w.(http.Flusher)
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ProcessResponse{
				Success: false,
				Error:   "Streaming is not supported",
			})
			return
		}

		// Create SSE writer
		sseWriter := &sseWriter{w: w, f: flusher}

		// Create progress channel and writer
		progressChan := make(chan processor.ProgressUpdate)
		progressWriter := processor.NewChannelProgressWriter(progressChan)

		// Set up processor with progress writer
		proc.SetProgressWriter(progressWriter)

		// Create context with timeout
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
		defer cancel()

		// Run the processor in a goroutine
		processDone := make(chan error)
		go func() {
			processDone <- proc.Process()
		}()

		// Start heartbeat ticker
		heartbeat := time.NewTicker(15 * time.Second)
		defer heartbeat.Stop()

		// Handle events
		for {
			select {
			case <-ctx.Done():
				sseWriter.SendError(fmt.Errorf("processing timed out after 5 minutes"))
				return
			case <-r.Context().Done():
				// Client disconnected
				return
			case err := <-processDone:
				if err != nil {
					sseWriter.SendError(err)
				} else {
					sseWriter.SendComplete("Processing complete")
				}
				return
			case update := <-progressChan:
				switch update.Type {
				case processor.ProgressSpinner:
					sseWriter.SendSpinner(update.Message)
				case processor.ProgressStep:
					// Map specific progress messages to complete events
					if update.Message == "DSL processing completed successfully" {
						sseWriter.SendComplete(update.Message)
					} else {
						sseWriter.SendProgress(update.Message)
					}
				case processor.ProgressComplete:
					sseWriter.SendComplete(update.Message)
				case processor.ProgressError:
					sseWriter.SendError(update.Error)
				}
			case <-heartbeat.C:
				sseWriter.SendHeartbeat()
			}
		}
	}

	// Non-streaming behavior (existing implementation)
	var buf bytes.Buffer
	pipeReader, pipeWriter, _ := os.Pipe()
	filterWriter := &filteringWriter{
		output: pipeWriter,
		debug:  os.Stdout,
	}

	originalLogOutput := log.Writer()
	log.SetOutput(filterWriter)

	config.DebugLog("Starting DSL processing")

	err = proc.Process()

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		io.Copy(&buf, pipeReader)
	}()

	log.SetOutput(originalLogOutput)
	pipeWriter.Close()
	wg.Wait()

	finalOutput := proc.LastOutput()

	if err != nil {
		config.VerboseLog("Error processing DSL: %v", err)
		config.DebugLog("DSL processing error: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		if streaming {
			flusher, ok := w.(http.Flusher)
			if !ok {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(ProcessResponse{
					Success: false,
					Error:   "Streaming is not supported",
				})
				return
			}
			sseWriter := &sseWriter{w: w, f: flusher}
			sseWriter.SendError(fmt.Errorf("Error processing DSL file: %v", err))
		} else {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(ProcessResponse{
				Success: false,
				Error:   fmt.Sprintf("Error processing DSL file: %v", err),
				Output:  finalOutput,
			})
		}
		return
	}

	config.VerboseLog("Successfully processed file: %s", filename)
	config.DebugLog("DSL processing complete. Output length: %d bytes", len(finalOutput))

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(ProcessResponse{
		Success: true,
		Message: fmt.Sprintf("Successfully processed %s", filename),
		Output:  finalOutput,
	})
}
